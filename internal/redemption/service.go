package redemption

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"spacetime-node/internal/platform/outbox"
)

var (
	ErrInvalidRequest         = errors.New("invalid redemption request")
	ErrUserNotFound           = errors.New("user not found")
	ErrJourneyNotFound        = errors.New("journey not found")
	ErrOfferUnavailable       = errors.New("offer unavailable")
	ErrInsufficientPoints     = errors.New("insufficient points")
	ErrIdempotencyKeyConflict = errors.New("idempotency key conflict")
)

type Service struct {
	db *sql.DB
}

type CreateRequest struct {
	UserIDHash     string
	JourneyID      string
	OfferID        string
	IdempotencyKey string
	TraceID        string
}

type Redemption struct {
	ID                       string
	JourneyID                string
	OfferID                  string
	Status                   string
	PointsCost               int64
	MerchantVerificationCode string
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, request CreateRequest) (Redemption, error) {
	if s == nil || s.db == nil || request.UserIDHash == "" || request.JourneyID == "" || request.OfferID == "" || request.IdempotencyKey == "" || request.TraceID == "" {
		return Redemption{}, ErrInvalidRequest
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Redemption{}, err
	}
	defer tx.Rollback()

	var userID string
	var balance int64
	if err := tx.QueryRowContext(ctx, `
		SELECT user_id, point_balance
		FROM users
		WHERE user_id_hash = $1
		FOR UPDATE`, request.UserIDHash).Scan(&userID, &balance); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Redemption{}, ErrUserNotFound
		}
		return Redemption{}, err
	}

	existing, found, err := existingRedemption(ctx, tx, userID, request.IdempotencyKey)
	if err != nil {
		return Redemption{}, err
	}
	if found {
		if existing.JourneyID != request.JourneyID || existing.OfferID != request.OfferID {
			return Redemption{}, ErrIdempotencyKeyConflict
		}
		if err := tx.Commit(); err != nil {
			return Redemption{}, err
		}
		return existing, nil
	}

	var journeyExists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM journeys WHERE journey_id = $1 AND user_id = $2
		)`, request.JourneyID, userID).Scan(&journeyExists); err != nil {
		return Redemption{}, err
	}
	if !journeyExists {
		return Redemption{}, ErrJourneyNotFound
	}

	var availableQuantity int
	var pointsCost int64
	if err := tx.QueryRowContext(ctx, `
		SELECT inventory.available_quantity, offers.points_cost
		FROM offers
		JOIN inventory ON inventory.offer_id = offers.offer_id
		WHERE offers.offer_id = $1
		  AND offers.is_active
		  AND offers.starts_at <= current_timestamp
		  AND offers.ends_at > current_timestamp
		FOR UPDATE OF inventory`, request.OfferID).Scan(&availableQuantity, &pointsCost); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Redemption{}, ErrOfferUnavailable
		}
		return Redemption{}, err
	}
	if availableQuantity == 0 {
		return Redemption{}, ErrOfferUnavailable
	}
	if balance < pointsCost {
		return Redemption{}, ErrInsufficientPoints
	}

	redemption := Redemption{
		ID:                       uuid.NewString(),
		JourneyID:                request.JourneyID,
		OfferID:                  request.OfferID,
		Status:                   "succeeded",
		PointsCost:               pointsCost,
		MerchantVerificationCode: uuid.NewString(),
	}
	newBalance := balance - pointsCost

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO redemptions (
			redemption_id, user_id, journey_id, offer_id, idempotency_key,
			status, points_cost, merchant_verification_code
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		redemption.ID, userID, redemption.JourneyID, redemption.OfferID,
		request.IdempotencyKey, redemption.Status, redemption.PointsCost,
		redemption.MerchantVerificationCode); err != nil {
		return Redemption{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE users SET point_balance = $1, updated_at = current_timestamp WHERE user_id = $2`, newBalance, userID); err != nil {
		return Redemption{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE inventory
		SET available_quantity = available_quantity - 1, version = version + 1, updated_at = current_timestamp
		WHERE offer_id = $1`, redemption.OfferID); err != nil {
		return Redemption{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO points_ledger (user_id, redemption_id, delta, balance_after, reason)
		VALUES ($1, $2, $3, $4, $5)`,
		userID, redemption.ID, -redemption.PointsCost, newBalance, "redemption"); err != nil {
		return Redemption{}, err
	}
	if err := enqueueSucceededEvent(ctx, tx, request, redemption); err != nil {
		return Redemption{}, err
	}
	if err := tx.Commit(); err != nil {
		return Redemption{}, err
	}

	return redemption, nil
}

func enqueueSucceededEvent(ctx context.Context, tx *sql.Tx, request CreateRequest, redemption Redemption) error {
	type payload struct {
		RedemptionID string `json:"redemption_id"`
		OfferID      string `json:"offer_id"`
		PointsCost   int64  `json:"points_cost"`
	}

	eventID := uuid.NewString()
	message, err := json.Marshal(struct {
		EventID       string  `json:"event_id"`
		EventType     string  `json:"event_type"`
		SchemaVersion int     `json:"schema_version"`
		OccurredAt    string  `json:"occurred_at"`
		Producer      string  `json:"producer"`
		TraceID       string  `json:"trace_id"`
		JourneyID     string  `json:"journey_id"`
		UserIDHash    string  `json:"user_id_hash"`
		Payload       payload `json:"payload"`
	}{
		EventID:       eventID,
		EventType:     "redemption.succeeded.v1",
		SchemaVersion: 1,
		OccurredAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Producer:      "redemption-service",
		TraceID:       request.TraceID,
		JourneyID:     redemption.JourneyID,
		UserIDHash:    request.UserIDHash,
		Payload: payload{
			RedemptionID: redemption.ID,
			OfferID:      redemption.OfferID,
			PointsCost:   redemption.PointsCost,
		},
	})
	if err != nil {
		return err
	}
	return outbox.Enqueue(ctx, tx, outbox.Event{
		ID:      eventID,
		Topic:   "redemption.succeeded.v1",
		Key:     request.UserIDHash,
		Payload: message,
	})
}

func existingRedemption(ctx context.Context, tx *sql.Tx, userID, idempotencyKey string) (Redemption, bool, error) {
	var redemption Redemption
	err := tx.QueryRowContext(ctx, `
		SELECT redemption_id, journey_id, offer_id, status, points_cost, merchant_verification_code
		FROM redemptions
		WHERE user_id = $1 AND idempotency_key = $2`, userID, idempotencyKey).Scan(
		&redemption.ID,
		&redemption.JourneyID,
		&redemption.OfferID,
		&redemption.Status,
		&redemption.PointsCost,
		&redemption.MerchantVerificationCode,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Redemption{}, false, nil
	}
	if err != nil {
		return Redemption{}, false, err
	}
	return redemption, true, nil
}
