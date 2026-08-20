package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/api/render"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/auth"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/settle"
)

// Admin routes. All require role = 'admin' and all write audit_log — a
// privileged action with no record of who took it is the one thing the audit
// trail exists to prevent.

type createSlipRequest struct {
	PackageCode string      `json:"packageCode"`
	Title       string      `json:"title"`
	PriceUgx    int64       `json:"priceUgx"`
	TotalOdds   string      `json:"totalOdds"`
	TipCount    int         `json:"tipCount"`
	AnalystIDs  []uuid.UUID `json:"analystIds"`
}

func (s *Server) handleAdminCreateSlip(w http.ResponseWriter, r *http.Request) {
	var req createSlipRequest
	if err := decodeBody(r, &req); err != nil {
		render.Error(w, r, err, s.Log)
		return
	}

	packageCode, err := domain.ParsePackageCode(req.PackageCode)
	if err != nil {
		render.Error(w, r, fmt.Errorf("%w: %s", domain.ErrValidation, err), s.Log)
		return
	}
	if req.Title == "" {
		render.Error(w, r, fmt.Errorf("%w: title is required", domain.ErrValidation), s.Log)
		return
	}
	if req.PriceUgx <= 0 {
		render.Error(w, r, fmt.Errorf("%w: priceUgx must be positive", domain.ErrValidation), s.Log)
		return
	}
	if req.TipCount <= 0 {
		render.Error(w, r, fmt.Errorf("%w: tipCount must be positive", domain.ErrValidation), s.Log)
		return
	}
	// Odds arrive as a string so that a JSON number's float representation
	// never touches them.
	totalOdds, err := decimal.NewFromString(req.TotalOdds)
	if err != nil || totalOdds.LessThan(decimal.NewFromInt(1)) {
		render.Error(w, r, fmt.Errorf(
			"%w: totalOdds must be a decimal string of at least 1", domain.ErrValidation), s.Log)
		return
	}
	if len(req.AnalystIDs) == 0 {
		render.Error(w, r, fmt.Errorf("%w: at least one analyst is required", domain.ErrValidation), s.Log)
		return
	}

	admin, _ := auth.UserFrom(r.Context())

	var slipID uuid.UUID
	err = s.DB.InTx(r.Context(), func(tx pgx.Tx) error {
		var err error
		slipID, err = s.DB.CreateSlip(r.Context(), tx, domain.Slip{
			PackageCode: packageCode,
			Title:       req.Title,
			PriceUGX:    domain.UGX(req.PriceUgx),
			TotalOdds:   totalOdds,
			TipCount:    req.TipCount,
			CreatedBy:   admin.ID,
			AnalystIDs:  req.AnalystIDs,
		})
		if err != nil {
			return err
		}
		return s.DB.WriteAudit(r.Context(), tx, postgres.AuditEntry{
			ActorType: postgres.ActorAdmin,
			ActorID:   &admin.ID,
			Action:    "slip.created",
			Entity:    "slip",
			EntityID:  &slipID,
			After:     map[string]any{"title": req.Title, "price_ugx": req.PriceUgx, "package": packageCode},
		})
	})
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}

	render.Status(w, http.StatusCreated, map[string]any{"id": slipID, "status": domain.SlipDraft})
}

type addTipRequest struct {
	AnalystID      uuid.UUID  `json:"analystId"`
	MatchID        *uuid.UUID `json:"matchId,omitempty"`
	FixtureLabel   string     `json:"fixtureLabel"`
	MarketLabel    string     `json:"marketLabel"`
	SelectionLabel string     `json:"selectionLabel"`
	MarketCode     *string    `json:"marketType,omitempty"`
	SelectionValue *string    `json:"selectionValue,omitempty"`
	Odds           string     `json:"odds"`
	KickoffAt      time.Time  `json:"kickoffAt"`
	Note           *string    `json:"note,omitempty"`
	Position       int        `json:"position"`
}

func (s *Server) handleAdminAddTip(w http.ResponseWriter, r *http.Request) {
	slipID, err := parseUUID(r.PathValue("id"), "slip id")
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	var req addTipRequest
	if err := decodeBody(r, &req); err != nil {
		render.Error(w, r, err, s.Log)
		return
	}

	odds, err := decimal.NewFromString(req.Odds)
	if err != nil || !odds.GreaterThan(decimal.NewFromInt(1)) {
		render.Error(w, r, fmt.Errorf(
			"%w: odds must be a decimal string greater than 1", domain.ErrValidation), s.Log)
		return
	}
	if req.FixtureLabel == "" || req.MarketLabel == "" || req.SelectionLabel == "" {
		render.Error(w, r, fmt.Errorf(
			"%w: fixtureLabel, marketLabel and selectionLabel are required", domain.ErrValidation), s.Log)
		return
	}
	if req.Position <= 0 {
		render.Error(w, r, fmt.Errorf("%w: position must be positive", domain.ErrValidation), s.Log)
		return
	}

	// A tip is either fully structured and auto-gradable, or free text an
	// admin grades by hand. The in-between would silently never settle, so it
	// is rejected here as well as by the schema's CHECK.
	structured := 0
	for _, present := range []bool{req.MatchID != nil, req.MarketCode != nil, req.SelectionValue != nil} {
		if present {
			structured++
		}
	}
	if structured != 0 && structured != 3 {
		render.Error(w, r, fmt.Errorf(
			"%w: a tip needs matchId, marketType and selectionValue together, or none of them",
			domain.ErrValidation), s.Log)
		return
	}

	var marketCode *domain.MarketCode
	if req.MarketCode != nil {
		parsed, err := domain.ParseMarketCode(*req.MarketCode)
		if err != nil {
			render.Error(w, r, fmt.Errorf("%w: %s", domain.ErrValidation, err), s.Log)
			return
		}
		marketCode = &parsed

		if req.SelectionValue != nil && !settle.ValidSelection(parsed, *req.SelectionValue) {
			// A selection the market cannot carry would grade as a loss
			// forever without anyone noticing.
			render.Error(w, r, fmt.Errorf("%w: %q is not a selection %s can settle to",
				domain.ErrValidation, *req.SelectionValue, parsed), s.Log)
			return
		}
	}

	admin, _ := auth.UserFrom(r.Context())

	var tipID uuid.UUID
	err = s.DB.InTx(r.Context(), func(tx pgx.Tx) error {
		var err error
		tipID, err = s.DB.AddTip(r.Context(), tx, domain.Tip{
			SlipID:         slipID,
			AnalystID:      req.AnalystID,
			MatchID:        req.MatchID,
			FixtureLabel:   req.FixtureLabel,
			MarketLabel:    req.MarketLabel,
			SelectionLabel: req.SelectionLabel,
			MarketCode:     marketCode,
			SelectionValue: req.SelectionValue,
			Odds:           odds,
			KickoffAt:      req.KickoffAt,
			Note:           req.Note,
			Position:       req.Position,
		})
		if err != nil {
			return err
		}
		return s.DB.WriteAudit(r.Context(), tx, postgres.AuditEntry{
			ActorType: postgres.ActorAdmin,
			ActorID:   &admin.ID,
			Action:    "tip.added",
			Entity:    "tip",
			EntityID:  &tipID,
			After:     map[string]any{"slip_id": slipID, "position": req.Position},
		})
	})
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.Status(w, http.StatusCreated, map[string]any{"id": tipID})
}

func (s *Server) handleAdminPublishSlip(w http.ResponseWriter, r *http.Request) {
	slipID, err := parseUUID(r.PathValue("id"), "slip id")
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	admin, _ := auth.UserFrom(r.Context())

	err = s.DB.InTx(r.Context(), func(tx pgx.Tx) error {
		// Refuses if any tip's kickoff has passed. Publishing a slip
		// containing a started match is the exact shape of the fraud the whole
		// design prevents, and it is an easy accident at 3pm on a Saturday.
		if err := s.DB.PublishSlip(r.Context(), tx, slipID); err != nil {
			return err
		}
		return s.DB.WriteAudit(r.Context(), tx, postgres.AuditEntry{
			ActorType: postgres.ActorAdmin,
			ActorID:   &admin.ID,
			Action:    "slip.published",
			Entity:    "slip",
			EntityID:  &slipID,
		})
	})
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.Status(w, http.StatusOK, map[string]any{"id": slipID, "status": domain.SlipOpen})
}

func (s *Server) handleAdminDeleteSlip(w http.ResponseWriter, r *http.Request) {
	slipID, err := parseUUID(r.PathValue("id"), "slip id")
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	admin, _ := auth.UserFrom(r.Context())

	err = s.DB.InTx(r.Context(), func(tx pgx.Tx) error {
		// Only while draft. A published slip is never deleted: the history
		// stays, including the losing ones.
		if err := s.DB.DeleteSlip(r.Context(), tx, slipID); err != nil {
			return err
		}
		return s.DB.WriteAudit(r.Context(), tx, postgres.AuditEntry{
			ActorType: postgres.ActorAdmin,
			ActorID:   &admin.ID,
			Action:    "slip.deleted",
			Entity:    "slip",
			EntityID:  &slipID,
		})
	})
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.NoContent(w)
}

type settleTipRequest struct {
	WasCorrect    bool   `json:"wasCorrect"`
	ActualOutcome string `json:"actualOutcome"`
	Reason        string `json:"reason"`
}

func (s *Server) handleAdminSettleTip(w http.ResponseWriter, r *http.Request) {
	tipID, err := parseUUID(r.PathValue("id"), "tip id")
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	var req settleTipRequest
	if err := decodeBody(r, &req); err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	admin, _ := auth.UserFrom(r.Context())

	service := &settle.Service{DB: s.DB, Log: s.Log}
	if err := service.SettleTipByAdmin(r.Context(), settle.AdminSettlement{
		TipID:         tipID,
		WasCorrect:    req.WasCorrect,
		ActualOutcome: req.ActualOutcome,
		Reason:        req.Reason,
		AdminUserID:   admin.ID,
	}); err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.Status(w, http.StatusOK, map[string]any{"id": tipID, "settledBy": domain.SettledByAdmin})
}

type correctMatchRequest struct {
	HomeScore int    `json:"homeScore"`
	AwayScore int    `json:"awayScore"`
	Reason    string `json:"reason"`
}

// handleAdminCorrectMatch is the only path that may change a finished score.
//
// It does **not** re-grade: prediction_results is immutable. It flags the
// affected predictions for review so the team publishes a correction, which
// means somebody owns the decision rather than history changing quietly.
func (s *Server) handleAdminCorrectMatch(w http.ResponseWriter, r *http.Request) {
	matchID, err := parseUUID(r.PathValue("id"), "match id")
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	var req correctMatchRequest
	if err := decodeBody(r, &req); err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	if req.Reason == "" {
		render.Error(w, r, fmt.Errorf("%w: reason is required", domain.ErrValidation), s.Log)
		return
	}
	if req.HomeScore < 0 || req.AwayScore < 0 {
		render.Error(w, r, fmt.Errorf("%w: scores cannot be negative", domain.ErrValidation), s.Log)
		return
	}
	admin, _ := auth.UserFrom(r.Context())

	var flagged int64
	err = s.DB.InTx(r.Context(), func(tx pgx.Tx) error {
		before, err := s.DB.MatchByID(r.Context(), tx, matchID)
		if err != nil {
			return err
		}
		if before.Status != domain.StatusFinished {
			return fmt.Errorf("%w: match %s is not finished", domain.ErrConflict, matchID)
		}

		if _, err := tx.Exec(r.Context(), `
			UPDATE matches SET home_score = $2, away_score = $3 WHERE id = $1`,
			matchID, req.HomeScore, req.AwayScore); err != nil {
			return fmt.Errorf("correct score: %w", err)
		}

		flagged, err = s.DB.FlagForCorrectionReview(r.Context(), tx, matchID, req.Reason)
		if err != nil {
			return err
		}

		return s.DB.WriteAudit(r.Context(), tx, postgres.AuditEntry{
			ActorType: postgres.ActorAdmin,
			ActorID:   &admin.ID,
			Action:    "match.score_corrected",
			Entity:    "match",
			EntityID:  &matchID,
			Reason:    req.Reason,
			Before:    map[string]any{"home_score": before.HomeScore, "away_score": before.AwayScore},
			After:     map[string]any{"home_score": req.HomeScore, "away_score": req.AwayScore},
		})
	})
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}

	render.Status(w, http.StatusOK, map[string]any{
		"id":                 matchID,
		"predictionsFlagged": flagged,
		"note":               "Existing results are unchanged. Flagged predictions need a published correction.",
	})
}

func (s *Server) handleAdminIngestStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.DB.IngestStatus(r.Context())
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	reviews, err := s.DB.OpenCorrectionReviews(r.Context())
	if err != nil {
		render.Error(w, r, err, s.Log)
		return
	}
	render.Status(w, http.StatusOK, map[string]any{
		"providers":             status,
		"openCorrectionReviews": len(reviews),
	})
}
