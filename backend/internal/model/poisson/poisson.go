// Package poisson is the goal-expectation model.
//
// Every MVP market is derived by summing regions of a single scoreline
// probability matrix, which is what keeps the published markets mutually
// consistent — Over 2.5 can never disagree with the 1X2 numbers, because both
// are read off the same matrix.
//
// This is a port of src/lib/poisson.ts. The two must produce identical output
// for identical input; see poisson_test.go, which carries the TypeScript
// fixtures across.
package poisson

import (
	"fmt"
	"math"
	"sort"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// MaxGoals truncates the matrix. Residual mass above it is normalised away.
const MaxGoals = 10

func logFactorial(n int) float64 {
	acc := 0.0
	for i := 2; i <= n; i++ {
		acc += math.Log(float64(i))
	}
	return acc
}

// PMF is P(X = k) for X ~ Poisson(lambda).
func PMF(k int, lambda float64) float64 {
	if k < 0 || math.IsInf(lambda, 0) || math.IsNaN(lambda) || lambda <= 0 {
		if k == 0 {
			return 1
		}
		return 0
	}
	// Computed in log space: lambda^k overflows well before k = MaxGoals
	// otherwise.
	return math.Exp(-lambda + float64(k)*math.Log(lambda) - logFactorial(k))
}

// Matrix is the joint probability of every scoreline, m[i][j] being
// P(home scores i, away scores j).
type Matrix [][]float64

// BuildMatrix assumes the two teams' goal counts are independent Poisson
// draws. Rows and columns are normalised so the matrix sums to exactly 1
// despite truncation at maxGoals.
func BuildMatrix(xgHome, xgAway float64, maxGoals int) Matrix {
	homeProbs := make([]float64, maxGoals+1)
	awayProbs := make([]float64, maxGoals+1)
	for k := 0; k <= maxGoals; k++ {
		homeProbs[k] = PMF(k, xgHome)
		awayProbs[k] = PMF(k, xgAway)
	}

	matrix := make(Matrix, maxGoals+1)
	total := 0.0
	for i := 0; i <= maxGoals; i++ {
		row := make([]float64, maxGoals+1)
		for j := 0; j <= maxGoals; j++ {
			p := homeProbs[i] * awayProbs[j]
			row[j] = p
			total += p
		}
		matrix[i] = row
	}

	if total > 0 && math.Abs(total-1) > 1e-12 {
		for i := 0; i <= maxGoals; i++ {
			for j := 0; j <= maxGoals; j++ {
				matrix[i][j] /= total
			}
		}
	}
	return matrix
}

// Build uses the default truncation.
func Build(xgHome, xgAway float64) Matrix { return BuildMatrix(xgHome, xgAway, MaxGoals) }

// oneXTwo reads the lower triangle, diagonal and upper triangle of the matrix.
func (m Matrix) oneXTwo() (home, draw, away float64) {
	for i := range m {
		for j := range m[i] {
			p := m[i][j]
			switch {
			case i > j:
				home += p
			case i == j:
				draw += p
			default:
				away += p
			}
		}
	}
	return home, draw, away
}

// bttsYes is every cell where neither side is on zero.
func (m Matrix) bttsYes() float64 {
	p := 0.0
	for i := 1; i < len(m); i++ {
		for j := 1; j < len(m[i]); j++ {
			p += m[i][j]
		}
	}
	return p
}

// overLine is P(total goals > line).
func (m Matrix) overLine(line float64) float64 {
	p := 0.0
	for i := range m {
		for j := range m[i] {
			if float64(i+j) > line {
				p += m[i][j]
			}
		}
	}
	return p
}

// Distribution returns the full probability distribution for one market, in
// the market's display order.
//
// Probabilities are 0..1 and sum to 1 for every market **except** Double
// Chance, whose three outcomes overlap — each 1X2 result is covered by two of
// them — so they sum to 2. Render these as independent bars, never as a
// stacked "share of 100%" chart.
func (m Matrix) Distribution(market domain.MarketCode) ([]domain.OutcomeProbability, error) {
	switch market {
	case domain.MarketOneXTwo:
		home, draw, away := m.oneXTwo()
		return []domain.OutcomeProbability{
			{Value: domain.OutcomeHome, Probability: home},
			{Value: domain.OutcomeDraw, Probability: draw},
			{Value: domain.OutcomeAway, Probability: away},
		}, nil

	case domain.MarketDoubleChance:
		home, draw, away := m.oneXTwo()
		return []domain.OutcomeProbability{
			{Value: domain.SelectionHomeOrDraw, Probability: home + draw},
			{Value: domain.SelectionHomeOrAway, Probability: home + away},
			{Value: domain.SelectionDrawOrAway, Probability: draw + away},
		}, nil

	case domain.MarketBTTS:
		yes := m.bttsYes()
		return []domain.OutcomeProbability{
			{Value: domain.OutcomeYes, Probability: yes},
			{Value: domain.OutcomeNo, Probability: 1 - yes},
		}, nil

	default:
		line, ok := domain.OverUnderLine[market]
		if !ok {
			return nil, fmt.Errorf("poisson: unknown market %q", market)
		}
		over := m.overLine(line)
		return []domain.OutcomeProbability{
			{Value: domain.OutcomeOver, Probability: over},
			{Value: domain.OutcomeUnder, Probability: 1 - over},
		}, nil
	}
}

// PickBest is the outcome the model publishes: simply the most likely one.
// Ties keep the earlier outcome in display order, matching the TypeScript
// reduce this is ported from.
func PickBest(distribution []domain.OutcomeProbability) domain.OutcomeProbability {
	best := distribution[0]
	for _, o := range distribution[1:] {
		if o.Probability > best.Probability {
			best = o
		}
	}
	return best
}

// TopScorelines is the n most likely exact scorelines, for the match detail
// view. The sort is stable so that symmetric matrices (xgHome == xgAway) order
// their tied cells the same way the TypeScript does.
func (m Matrix) TopScorelines(n int) []domain.Scoreline {
	all := make([]domain.Scoreline, 0, len(m)*len(m))
	for i := range m {
		for j := range m[i] {
			all = append(all, domain.Scoreline{Home: i, Away: j, Probability: m[i][j]})
		}
	}
	sort.SliceStable(all, func(a, b int) bool {
		return all[a].Probability > all[b].Probability
	})
	if n > len(all) {
		n = len(all)
	}
	return all[:n]
}
