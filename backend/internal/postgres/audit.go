package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// The audit log is append-only and is the record of every decision a human or
// a job made that the immutable tables cannot express.
//
// Corrections are new rows plus an entry here. A genuine mistake is corrected
// by a compensating entry and a public correction note, never by editing the
// result — that rule will feel obnoxious the first time someone fat-fingers a
// grade, and it is the rule that makes the leaderboard mean anything.

type ActorType string

const (
	ActorSystem ActorType = "system"
	ActorAdmin  ActorType = "admin"
	ActorJob    ActorType = "job"
)

// AuditEntry is one appended record.
type AuditEntry struct {
	ActorType ActorType
	ActorID   *uuid.UUID
	Action    string
	Entity    string
	EntityID  *uuid.UUID
	Before    any
	After     any
	Reason    string
}

// WriteAudit appends an entry. It takes a Querier so that the entry commits in
// the same transaction as the change it describes — an audit row that can be
// lost while the change survives is worse than none, because it looks
// complete.
func (db *DB) WriteAudit(ctx context.Context, q Querier, e AuditEntry) error {
	before, err := encodeAuditPayload(e.Before)
	if err != nil {
		return fmt.Errorf("encode audit before: %w", err)
	}
	after, err := encodeAuditPayload(e.After)
	if err != nil {
		return fmt.Errorf("encode audit after: %w", err)
	}

	var reason *string
	if e.Reason != "" {
		reason = &e.Reason
	}

	_, err = q.Exec(ctx, `
		INSERT INTO audit_log (actor_type, actor_id, action, entity, entity_id, before, after, reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		e.ActorType, e.ActorID, e.Action, e.Entity, e.EntityID, before, after, reason)
	if err != nil {
		return fmt.Errorf("write audit: %w", err)
	}
	return nil
}

func encodeAuditPayload(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// FlagForCorrectionReview records that a finished score changed under an
// already-graded prediction.
//
// prediction_results is immutable, so a score correction does not silently
// re-grade. It flags the affected predictions here and the team publishes a
// correction, which means somebody owns the decision.
func (db *DB) FlagForCorrectionReview(ctx context.Context, q Querier, matchID uuid.UUID, reason string) (int64, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO correction_reviews (match_id, prediction_id, reason)
		SELECT $1, p.id, $2
		FROM predictions p
		WHERE p.match_id = $1`, matchID, reason)
	if err != nil {
		return 0, fmt.Errorf("flag correction review: %w", err)
	}
	return tag.RowsAffected(), nil
}

// OpenCorrectionReview is one unresolved flag, for the admin surface.
type OpenCorrectionReview struct {
	ID           uuid.UUID
	MatchID      uuid.UUID
	PredictionID *uuid.UUID
	Reason       string
}

func (db *DB) OpenCorrectionReviews(ctx context.Context) ([]OpenCorrectionReview, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, match_id, prediction_id, reason
		FROM correction_reviews
		WHERE resolved_at IS NULL
		ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("query correction reviews: %w", err)
	}
	defer rows.Close()

	var out []OpenCorrectionReview
	for rows.Next() {
		var r OpenCorrectionReview
		if err := rows.Scan(&r.ID, &r.MatchID, &r.PredictionID, &r.Reason); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
