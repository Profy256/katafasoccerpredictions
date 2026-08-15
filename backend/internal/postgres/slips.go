package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
)

// SlipQuery narrows the slip list.
type SlipQuery struct {
	PackageCode domain.PackageCode
	Status      string // "open" or "settled"; drafts are never listed
	AnalystID   *uuid.UUID
	Limit       int
	Cursor      *SlipCursor
}

// SlipCursor paginates on (published_at, id).
type SlipCursor struct {
	PublishedAt time.Time
	ID          uuid.UUID
}

// Slips lists published slips. Drafts never appear: they are not a product
// yet, and their ids must not be discoverable.
func (db *DB) Slips(ctx context.Context, q SlipQuery) ([]domain.APISlip, *SlipCursor, error) {
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	args := []any{}
	where := []string{"s.status <> 'draft'", "s.published_at IS NOT NULL"}

	if q.PackageCode != "" {
		args = append(args, q.PackageCode)
		where = append(where, fmt.Sprintf("s.package_code = $%d", len(args)))
	}
	if q.Status != "" {
		args = append(args, q.Status)
		where = append(where, fmt.Sprintf("s.status = $%d", len(args)))
	}
	if q.AnalystID != nil {
		args = append(args, *q.AnalystID)
		where = append(where, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM slip_analysts sa WHERE sa.slip_id = s.id AND sa.analyst_id = $%d)", len(args)))
	}
	if q.Cursor != nil {
		args = append(args, q.Cursor.PublishedAt, q.Cursor.ID)
		where = append(where, fmt.Sprintf("(s.published_at, s.id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit+1)

	rows, err := db.Pool.Query(ctx, `
		SELECT s.id, s.package_code, s.title, s.price_ugx, s.total_odds::text,
		       s.tip_count, s.status, s.published_at, s.won_tips, s.settled_odds::text,
		       COALESCE(array_agg(sa.analyst_id) FILTER (WHERE sa.analyst_id IS NOT NULL), '{}')
		FROM slips s
		LEFT JOIN slip_analysts sa ON sa.slip_id = s.id
		WHERE `+strings.Join(where, " AND ")+`
		GROUP BY s.id
		ORDER BY s.published_at DESC, s.id DESC
		LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, nil, fmt.Errorf("query slips: %w", err)
	}
	defer rows.Close()

	out := make([]domain.APISlip, 0, limit)
	var hasMore bool
	for rows.Next() {
		if len(out) == limit {
			hasMore = true
			break
		}
		slip, err := scanSlip(rows)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, slip)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var next *SlipCursor
	if hasMore && len(out) > 0 {
		last := out[len(out)-1]
		next = &SlipCursor{PublishedAt: last.PublishedAt, ID: last.ID}
	}
	return out, next, nil
}

func scanSlip(rows pgx.Row) (domain.APISlip, error) {
	var s domain.APISlip
	var price int64
	var totalOdds string
	var settledOdds *string
	var publishedAt *time.Time
	var analystIDs []uuid.UUID

	if err := rows.Scan(&s.ID, &s.PackageCode, &s.Title, &price, &totalOdds,
		&s.TipCount, &s.Status, &publishedAt, &s.WonTips, &settledOdds, &analystIDs); err != nil {
		return domain.APISlip{}, fmt.Errorf("scan slip: %w", err)
	}

	s.PriceUGX = domain.UGX(price)
	s.AnalystIDs = analystIDs
	if s.AnalystIDs == nil {
		s.AnalystIDs = []uuid.UUID{}
	}
	if publishedAt != nil {
		s.PublishedAt = publishedAt.UTC()
	}

	odds, err := decimal.NewFromString(totalOdds)
	if err != nil {
		return domain.APISlip{}, fmt.Errorf("parse total odds %q: %w", totalOdds, err)
	}
	s.TotalOdds, _ = odds.Float64()

	if settledOdds != nil {
		d, err := decimal.NewFromString(*settledOdds)
		if err != nil {
			return domain.APISlip{}, fmt.Errorf("parse settled odds %q: %w", *settledOdds, err)
		}
		f, _ := d.Float64()
		s.SettledOdds = &f
	}

	// The frontend's Slip status is 'open' | 'settled'. A void slip — every
	// leg called off — presents as settled with zero winning tips, which is
	// what it is: a finished product that returned nothing, and refunded.
	if s.Status == string(domain.SlipVoid) {
		s.Status = string(domain.SlipSettled)
		zero := 0
		if s.WonTips == nil {
			s.WonTips = &zero
		}
	}
	return s, nil
}

// Slip returns one slip with its tips, if and only if the viewer is entitled
// to them.
//
// viewerID may be uuid.Nil for an anonymous viewer. The entitlement is folded
// into the tips query's WHERE clause: an unpaid viewer gets zero rows *from
// the database*. There is no filtering step in Go that could be forgotten, and
// no serialisation path where the tips exist in memory next to a boolean.
func (db *DB) Slip(ctx context.Context, slipID uuid.UUID, viewerID uuid.UUID, isAdmin bool) (domain.SlipWithTips, error) {
	// A draft slip is a 404 rather than a 403, so unpublished slip ids are not
	// discoverable by probing.
	visibility := "s.status <> 'draft'"
	if isAdmin {
		visibility = "TRUE"
	}

	row := db.Pool.QueryRow(ctx, `
		SELECT s.id, s.package_code, s.title, s.price_ugx, s.total_odds::text,
		       s.tip_count, s.status, s.published_at, s.won_tips, s.settled_odds::text,
		       COALESCE(array_agg(sa.analyst_id) FILTER (WHERE sa.analyst_id IS NOT NULL), '{}')
		FROM slips s
		LEFT JOIN slip_analysts sa ON sa.slip_id = s.id
		WHERE s.id = $1 AND `+visibility+`
		GROUP BY s.id`, slipID)

	slip, err := scanSlip(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SlipWithTips{}, domain.ErrNotFound
	}
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return domain.SlipWithTips{}, domain.ErrNotFound
		}
		return domain.SlipWithTips{}, err
	}

	tipRows, err := db.tipsForSlip(ctx, slipID, viewerID, isAdmin)
	if err != nil {
		return domain.SlipWithTips{}, err
	}
	results, err := db.tipResultsForSlip(ctx, slipID)
	if err != nil {
		return domain.SlipWithTips{}, err
	}

	analysts, err := db.Analysts(ctx)
	if err != nil {
		return domain.SlipWithTips{}, err
	}
	byID := make(map[uuid.UUID]domain.Analyst, len(analysts))
	for _, a := range analysts {
		byID[a.ID] = a
	}
	slipAnalysts := make([]domain.Analyst, 0, len(slip.AnalystIDs))
	for _, id := range slip.AnalystIDs {
		if a, ok := byID[id]; ok {
			slipAnalysts = append(slipAnalysts, a)
		}
	}

	packages, err := db.Packages(ctx)
	if err != nil {
		return domain.SlipWithTips{}, err
	}
	var pkg domain.Package
	for _, p := range packages {
		if p.Code == slip.PackageCode {
			pkg = p
		}
	}

	return domain.SlipWithTips{
		APISlip:  slip,
		Tips:     tipRows,
		Results:  results,
		Analysts: slipAnalysts,
		Package:  pkg,
		// Unlocked describes what happened, rather than deciding it: the query
		// above already returned tips or did not.
		Unlocked: len(tipRows) > 0 || slip.TipCount == 0,
	}, nil
}

// tipsForSlip is the paywall.
//
// Settled slips are deliberately public — that is what turns an analyst's
// record into evidence rather than a claim, and it means their losing slips
// are visible too. Only slips still open are paywalled.
func (db *DB) tipsForSlip(ctx context.Context, slipID, viewerID uuid.UUID, isAdmin bool) ([]domain.APITip, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT t.id, t.slip_id, t.analyst_id, t.match_id, t.fixture_label,
		       t.market_label, t.selection_label, t.market_code, t.selection_value,
		       t.odds::text, t.kickoff_at, t.note
		FROM tips t
		JOIN slips s ON s.id = t.slip_id
		WHERE t.slip_id = $1
		  AND (
		        s.status IN ('settled','void')        -- settled slips are public
		     OR $3                                    -- admins author them
		     OR EXISTS (SELECT 1 FROM purchases p
		                WHERE p.slip_id = s.id
		                  AND p.user_id = $2
		                  AND p.status  = 'paid')     -- a refund revokes access
		      )
		ORDER BY t.position`, slipID, viewerID, isAdmin)
	if err != nil {
		return nil, fmt.Errorf("query tips: %w", err)
	}
	defer rows.Close()

	out := []domain.APITip{}
	for rows.Next() {
		var t domain.APITip
		var odds string
		if err := rows.Scan(&t.ID, &t.SlipID, &t.AnalystID, &t.MatchID, &t.FixtureLabel,
			&t.MarketLabel, &t.SelectionLabel, &t.MarketType, &t.SelectionValue,
			&odds, &t.KickoffAt, &t.Note); err != nil {
			return nil, fmt.Errorf("scan tip: %w", err)
		}
		d, err := decimal.NewFromString(odds)
		if err != nil {
			return nil, fmt.Errorf("parse tip odds %q: %w", odds, err)
		}
		t.Odds, _ = d.Float64()
		t.KickoffAt = t.KickoffAt.UTC()
		out = append(out, t)
	}
	return out, rows.Err()
}

// tipResultsForSlip is not paywalled. Results only exist for settled tips, and
// a settled slip is public.
func (db *DB) tipResultsForSlip(ctx context.Context, slipID uuid.UUID) ([]domain.APITipResult, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT r.tip_id, r.was_correct, r.actual_outcome, r.settled_at, r.settled_by
		FROM tip_results r
		JOIN tips t ON t.id = r.tip_id
		WHERE t.slip_id = $1
		ORDER BY t.position`, slipID)
	if err != nil {
		return nil, fmt.Errorf("query tip results: %w", err)
	}
	defer rows.Close()

	var out []domain.APITipResult
	for rows.Next() {
		var r domain.APITipResult
		if err := rows.Scan(&r.TipID, &r.WasCorrect, &r.ActualOutcome, &r.SettledAt, &r.SettledBy); err != nil {
			return nil, err
		}
		r.SettledAt = r.SettledAt.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateSlip inserts a draft.
func (db *DB) CreateSlip(ctx context.Context, q Querier, s domain.Slip) (uuid.UUID, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx, `
		INSERT INTO slips (package_code, title, price_ugx, total_odds, tip_count, created_by)
		VALUES ($1,$2,$3,$4::numeric,$5,$6)
		RETURNING id`,
		s.PackageCode, s.Title, int64(s.PriceUGX), s.TotalOdds.String(), s.TipCount, s.CreatedBy).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert slip: %w", err)
	}
	for _, analystID := range s.AnalystIDs {
		if _, err := q.Exec(ctx, `
			INSERT INTO slip_analysts (slip_id, analyst_id) VALUES ($1,$2)
			ON CONFLICT DO NOTHING`, id, analystID); err != nil {
			return uuid.Nil, fmt.Errorf("attach analyst: %w", err)
		}
	}
	return id, nil
}

// AddTip appends a tip to a draft slip.
func (db *DB) AddTip(ctx context.Context, q Querier, t domain.Tip) (uuid.UUID, error) {
	var status string
	if err := q.QueryRow(ctx, `SELECT status FROM slips WHERE id = $1`, t.SlipID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, domain.ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("read slip status: %w", err)
	}
	if status != string(domain.SlipDraft) {
		// tips is immutable and a published slip's tip_count is frozen, so
		// adding a leg after publication would make the slip describe itself
		// incorrectly.
		return uuid.Nil, fmt.Errorf("%w: slip %s is not a draft", domain.ErrConflict, t.SlipID)
	}

	var id uuid.UUID
	err := q.QueryRow(ctx, `
		INSERT INTO tips (slip_id, analyst_id, match_id, fixture_label, market_label,
		                  selection_label, market_code, selection_value, odds,
		                  kickoff_at, note, position)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::numeric,$10,$11,$12)
		RETURNING id`,
		t.SlipID, t.AnalystID, t.MatchID, t.FixtureLabel, t.MarketLabel,
		t.SelectionLabel, t.MarketCode, t.SelectionValue, t.Odds.String(),
		t.KickoffAt, t.Note, t.Position).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert tip: %w", err)
	}
	return id, nil
}

// PublishSlip moves a draft to open.
//
// It refuses if any tip's kickoff has passed. Publishing a slip containing a
// started match is the exact shape of the fraud this whole design exists to
// prevent, and it is an easy accident at 3pm on a Saturday.
func (db *DB) PublishSlip(ctx context.Context, q Querier, slipID uuid.UUID) error {
	var status string
	var tipCount, actualTips int
	err := q.QueryRow(ctx, `
		SELECT s.status, s.tip_count, (SELECT count(*) FROM tips t WHERE t.slip_id = s.id)
		FROM slips s WHERE s.id = $1`, slipID).Scan(&status, &tipCount, &actualTips)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read slip: %w", err)
	}
	if status != string(domain.SlipDraft) {
		return fmt.Errorf("%w: slip %s is already published", domain.ErrConflict, slipID)
	}
	if actualTips == 0 {
		return fmt.Errorf("%w: slip %s has no tips", domain.ErrConflict, slipID)
	}
	if actualTips != tipCount {
		return fmt.Errorf("%w: slip %s claims %d tips but carries %d",
			domain.ErrConflict, slipID, tipCount, actualTips)
	}

	var started int
	if err := q.QueryRow(ctx,
		`SELECT count(*) FROM tips WHERE slip_id = $1 AND kickoff_at <= now()`, slipID).
		Scan(&started); err != nil {
		return fmt.Errorf("check kickoffs: %w", err)
	}
	if started > 0 {
		return fmt.Errorf("%w: slip %s contains %d selection(s) whose match has already kicked off",
			domain.ErrConflict, slipID, started)
	}

	if _, err := q.Exec(ctx,
		`UPDATE slips SET status = 'open', published_at = now() WHERE id = $1 AND status = 'draft'`,
		slipID); err != nil {
		return fmt.Errorf("publish slip: %w", err)
	}
	return nil
}

// DeleteSlip removes a draft. Published slips are never deleted: the history
// stays, including the losing ones.
func (db *DB) DeleteSlip(ctx context.Context, q Querier, slipID uuid.UUID) error {
	tag, err := q.Exec(ctx, `DELETE FROM slips WHERE id = $1 AND status = 'draft'`, slipID)
	if err != nil {
		return fmt.Errorf("delete slip: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: slip %s is not a deletable draft", domain.ErrConflict, slipID)
	}
	return nil
}
