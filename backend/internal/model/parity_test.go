package model_test

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/model/poisson"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/model/strength"
)

// The Go model must produce the same numbers as src/lib/poisson.ts and
// src/lib/model.ts. A silent numerical divergence between them is very hard to
// find later: nothing errors, the site keeps rendering, and the published
// probabilities are simply different from the ones the methodology page
// describes.
//
// testdata/parity.json is generated from the TypeScript itself — see
// tools/parity/gen.ts — so this is a diff against the real implementation
// rather than against a hand-copied expectation.

// Tolerance is not zero because Go's and V8's math.Log/Exp/Pow may differ in
// the last unit in the last place. It is tight enough that any actual
// difference in formula, constant, or order of operations fails.
const tolerance = 1e-12

func closeEnough(got, want float64) bool {
	if got == want {
		return true
	}
	diff := math.Abs(got - want)
	if diff <= tolerance {
		return true
	}
	scale := math.Max(math.Abs(got), math.Abs(want))
	return diff/scale <= tolerance
}

type parityFile struct {
	PMF []struct {
		K      int     `json:"k"`
		Lambda float64 `json:"lambda"`
		P      float64 `json:"p"`
	} `json:"pmf"`

	Matrices []struct {
		XGHome        float64     `json:"xgHome"`
		XGAway        float64     `json:"xgAway"`
		Sum           float64     `json:"sum"`
		Cells         [][]float64 `json:"cells"`
		Distributions []struct {
			Market   domain.MarketCode `json:"market"`
			Outcomes []struct {
				Value       string  `json:"value"`
				Probability float64 `json:"probability"`
			} `json:"outcomes"`
			Best struct {
				Value       string  `json:"value"`
				Probability float64 `json:"probability"`
			} `json:"best"`
		} `json:"distributions"`
		TopScorelines []struct {
			Home        int     `json:"home"`
			Away        int     `json:"away"`
			Probability float64 `json:"probability"`
		} `json:"topScorelines"`
	} `json:"matrices"`

	History []struct {
		HomeTeamID string `json:"homeTeamId"`
		AwayTeamID string `json:"awayTeamId"`
		HomeScore  int    `json:"homeScore"`
		AwayScore  int    `json:"awayScore"`
		KickoffAt  string `json:"kickoffAt"`
	} `json:"history"`

	Baselines tsBaselines `json:"baselines"`

	Strengths []struct {
		Cutoff    int         `json:"cutoff"`
		TeamID    string      `json:"teamId"`
		Baselines tsBaselines `json:"baselines"`
		Strengths struct {
			TeamID     string  `json:"teamId"`
			Home       tsVenue `json:"home"`
			Away       tsVenue `json:"away"`
			TotalGames int     `json:"totalGames"`
		} `json:"strengths"`
		RecentForm []string `json:"recentForm"`
	} `json:"strengths"`

	Fixtures []struct {
		Cutoff int `json:"cutoff"`
		XG     struct {
			XGHome float64 `json:"xgHome"`
			XGAway float64 `json:"xgAway"`
		} `json:"xg"`
	} `json:"fixtures"`
}

type tsBaselines struct {
	HomeGoals float64 `json:"homeGoals"`
	AwayGoals float64 `json:"awayGoals"`
	Matches   int     `json:"matches"`
}

type tsVenue struct {
	Attack       float64 `json:"attack"`
	Defense      float64 `json:"defense"`
	Games        int     `json:"games"`
	GoalsFor     int     `json:"goalsFor"`
	GoalsAgainst int     `json:"goalsAgainst"`
	Wins         int     `json:"wins"`
	Draws        int     `json:"draws"`
	Losses       int     `json:"losses"`
}

func loadParity(t *testing.T) parityFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/parity.json")
	if err != nil {
		t.Fatalf("read parity fixtures: %v", err)
	}
	var f parityFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse parity fixtures: %v", err)
	}
	return f
}

// teamUUID maps the fixture's 't0'…'t7' onto stable UUIDs, since the Go model
// keys on uuid.UUID and the TypeScript on strings.
func teamUUID(name string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name))
}

func (f parityFile) goHistory(t *testing.T, limit int) []strength.PlayedMatch {
	t.Helper()
	out := make([]strength.PlayedMatch, 0, limit)
	for i, m := range f.History {
		if i >= limit {
			break
		}
		kickoff, err := time.Parse(time.RFC3339, m.KickoffAt)
		if err != nil {
			t.Fatalf("parse kickoff %q: %v", m.KickoffAt, err)
		}
		out = append(out, strength.PlayedMatch{
			MatchID:    uuid.NewSHA1(uuid.NameSpaceOID, []byte(m.KickoffAt+m.HomeTeamID)),
			HomeTeamID: teamUUID(m.HomeTeamID),
			AwayTeamID: teamUUID(m.AwayTeamID),
			HomeScore:  m.HomeScore,
			AwayScore:  m.AwayScore,
			KickoffAt:  kickoff,
		})
	}
	return out
}

func TestPoissonPMFMatchesTypeScript(t *testing.T) {
	for _, tc := range loadParity(t).PMF {
		if got := poisson.PMF(tc.K, tc.Lambda); !closeEnough(got, tc.P) {
			t.Errorf("PMF(%d, %g) = %v, TypeScript says %v", tc.K, tc.Lambda, got, tc.P)
		}
	}
}

func TestScorelineMatrixMatchesTypeScript(t *testing.T) {
	for _, tc := range loadParity(t).Matrices {
		matrix := poisson.Build(tc.XGHome, tc.XGAway)

		if len(matrix) != len(tc.Cells) {
			t.Fatalf("xg %g/%g: matrix is %d rows, TypeScript has %d",
				tc.XGHome, tc.XGAway, len(matrix), len(tc.Cells))
		}
		for i := range tc.Cells {
			for j := range tc.Cells[i] {
				if !closeEnough(matrix[i][j], tc.Cells[i][j]) {
					t.Errorf("xg %g/%g cell [%d][%d] = %v, TypeScript says %v",
						tc.XGHome, tc.XGAway, i, j, matrix[i][j], tc.Cells[i][j])
				}
			}
		}

		// Truncation is normalised away, so the matrix must still be a
		// probability distribution.
		sum := 0.0
		for i := range matrix {
			for j := range matrix[i] {
				sum += matrix[i][j]
			}
		}
		if math.Abs(sum-1) > 1e-9 {
			t.Errorf("xg %g/%g: matrix sums to %v, want 1", tc.XGHome, tc.XGAway, sum)
		}

		for _, want := range tc.Distributions {
			got, err := matrix.Distribution(want.Market)
			if err != nil {
				t.Fatalf("Distribution(%s): %v", want.Market, err)
			}
			if len(got) != len(want.Outcomes) {
				t.Fatalf("%s: %d outcomes, TypeScript has %d", want.Market, len(got), len(want.Outcomes))
			}
			for i := range want.Outcomes {
				// Order is display order and is part of the contract — the
				// frontend renders the distribution in the order it arrives.
				if got[i].Value != want.Outcomes[i].Value {
					t.Errorf("%s outcome %d is %q, TypeScript says %q",
						want.Market, i, got[i].Value, want.Outcomes[i].Value)
				}
				if !closeEnough(got[i].Probability, want.Outcomes[i].Probability) {
					t.Errorf("xg %g/%g %s %s = %v, TypeScript says %v",
						tc.XGHome, tc.XGAway, want.Market, got[i].Value,
						got[i].Probability, want.Outcomes[i].Probability)
				}
			}

			best := poisson.PickBest(got)
			if best.Value != want.Best.Value {
				t.Errorf("xg %g/%g %s picked %q, TypeScript picked %q",
					tc.XGHome, tc.XGAway, want.Market, best.Value, want.Best.Value)
			}
		}

		top := matrix.TopScorelines(len(tc.TopScorelines))
		for i, want := range tc.TopScorelines {
			if top[i].Home != want.Home || top[i].Away != want.Away {
				t.Errorf("xg %g/%g scoreline %d is %d-%d, TypeScript says %d-%d",
					tc.XGHome, tc.XGAway, i, top[i].Home, top[i].Away, want.Home, want.Away)
			}
			if !closeEnough(top[i].Probability, want.Probability) {
				t.Errorf("xg %g/%g scoreline %d probability = %v, TypeScript says %v",
					tc.XGHome, tc.XGAway, i, top[i].Probability, want.Probability)
			}
		}
	}
}

func TestTeamStrengthsMatchTypeScript(t *testing.T) {
	f := loadParity(t)

	full := f.goHistory(t, len(f.History))
	base := strength.ComputeBaselines(full)
	if !closeEnough(base.HomeGoals, f.Baselines.HomeGoals) ||
		!closeEnough(base.AwayGoals, f.Baselines.AwayGoals) ||
		base.Matches != f.Baselines.Matches {
		t.Errorf("baselines = %+v, TypeScript says %+v", base, f.Baselines)
	}

	for _, tc := range f.Strengths {
		prefix := f.goHistory(t, tc.Cutoff)
		prefixBase := strength.ComputeBaselines(prefix)
		if !closeEnough(prefixBase.HomeGoals, tc.Baselines.HomeGoals) ||
			!closeEnough(prefixBase.AwayGoals, tc.Baselines.AwayGoals) {
			t.Errorf("cutoff %d: baselines = %+v, TypeScript says %+v",
				tc.Cutoff, prefixBase, tc.Baselines)
		}

		got := strength.ComputeTeamStrengths(teamUUID(tc.TeamID), prefix, prefixBase)
		if got.TotalGames != tc.Strengths.TotalGames {
			t.Errorf("cutoff %d %s: totalGames = %d, TypeScript says %d",
				tc.Cutoff, tc.TeamID, got.TotalGames, tc.Strengths.TotalGames)
		}
		checkVenue(t, tc.Cutoff, tc.TeamID, "home", got.Home, tc.Strengths.Home)
		checkVenue(t, tc.Cutoff, tc.TeamID, "away", got.Away, tc.Strengths.Away)

		form := strength.RecentForm(teamUUID(tc.TeamID), prefix, 5)
		if len(form) != len(tc.RecentForm) {
			t.Fatalf("cutoff %d %s: form has %d entries, TypeScript has %d",
				tc.Cutoff, tc.TeamID, len(form), len(tc.RecentForm))
		}
		for i := range form {
			if string(form[i]) != tc.RecentForm[i] {
				t.Errorf("cutoff %d %s: form[%d] = %s, TypeScript says %s",
					tc.Cutoff, tc.TeamID, i, form[i], tc.RecentForm[i])
			}
		}
	}
}

func checkVenue(t *testing.T, cutoff int, team, venue string, got strength.VenueStrength, want tsVenue) {
	t.Helper()
	if !closeEnough(got.Attack, want.Attack) {
		t.Errorf("cutoff %d %s %s attack = %v, TypeScript says %v", cutoff, team, venue, got.Attack, want.Attack)
	}
	if !closeEnough(got.Defense, want.Defense) {
		t.Errorf("cutoff %d %s %s defense = %v, TypeScript says %v", cutoff, team, venue, got.Defense, want.Defense)
	}
	if got.Games != want.Games || got.GoalsFor != want.GoalsFor || got.GoalsAgainst != want.GoalsAgainst ||
		got.Wins != want.Wins || got.Draws != want.Draws || got.Losses != want.Losses {
		t.Errorf("cutoff %d %s %s record = %+v, TypeScript says %+v", cutoff, team, venue, got, want)
	}
}

func TestExpectedGoalsMatchesTypeScript(t *testing.T) {
	f := loadParity(t)
	for _, tc := range f.Fixtures {
		prefix := f.goHistory(t, tc.Cutoff)
		base := strength.ComputeBaselines(prefix)
		home := strength.ComputeTeamStrengths(teamUUID("t0"), prefix, base)
		away := strength.ComputeTeamStrengths(teamUUID("t3"), prefix, base)
		xg := strength.Expected(home, away, base)

		if !closeEnough(xg.Home, tc.XG.XGHome) || !closeEnough(xg.Away, tc.XG.XGAway) {
			t.Errorf("cutoff %d: xg = %v/%v, TypeScript says %v/%v",
				tc.Cutoff, xg.Home, xg.Away, tc.XG.XGHome, tc.XG.XGAway)
		}
	}
}
