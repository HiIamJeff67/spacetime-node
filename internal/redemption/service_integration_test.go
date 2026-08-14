package redemption

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const demoUserIDHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestCreateRedemption(t *testing.T) {
	db := integrationDB(t)
	resetDatabase(t, db)
	service := NewService(db)
	request := CreateRequest{
		UserIDHash:     demoUserIDHash,
		JourneyID:      "journey-demo-001",
		OfferID:        "offer-coffee-xinyi",
		IdempotencyKey: "redemption-demo-key",
		TraceID:        "trace-redemption-demo",
	}

	created, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != replayed.ID {
		t.Fatalf("replay created a different redemption: %s != %s", created.ID, replayed.ID)
	}
	conflictingRequest := request
	conflictingRequest.OfferID = "offer-lunch-xinyi"
	if _, err := service.Create(context.Background(), conflictingRequest); !errors.Is(err, ErrIdempotencyKeyConflict) {
		t.Fatalf("expected ErrIdempotencyKeyConflict, got %v", err)
	}

	var balance int64
	var quantity int
	var redemptionCount int
	var ledgerCount int
	var eventID string
	var outboxPayload []byte
	if err := db.QueryRow(`SELECT point_balance FROM users WHERE user_id_hash = $1`, demoUserIDHash).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT available_quantity FROM inventory WHERE offer_id = $1`, request.OfferID).Scan(&quantity); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM redemptions`).Scan(&redemptionCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM points_ledger WHERE reason = 'redemption'`).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT event_id, payload FROM outbox_events WHERE topic = 'redemption.succeeded.v1'`).Scan(&eventID, &outboxPayload); err != nil {
		t.Fatal(err)
	}
	var event struct {
		EventID string `json:"event_id"`
		TraceID string `json:"trace_id"`
	}
	if err := json.Unmarshal(outboxPayload, &event); err != nil {
		t.Fatal(err)
	}
	if balance != 1120 || quantity != 49 || redemptionCount != 1 || ledgerCount != 1 || event.EventID != eventID || event.TraceID != request.TraceID {
		t.Fatalf("unexpected state: balance=%d quantity=%d redemptions=%d ledger=%d event=%s trace=%s", balance, quantity, redemptionCount, ledgerCount, event.EventID, event.TraceID)
	}
}

func TestVerifyRedemption(t *testing.T) {
	db := integrationDB(t)
	resetDatabase(t, db)
	service := NewService(db)
	created, err := service.Create(context.Background(), CreateRequest{
		UserIDHash:     demoUserIDHash,
		JourneyID:      "journey-demo-001",
		OfferID:        "offer-coffee-xinyi",
		IdempotencyKey: "verify-redemption-key",
		TraceID:        "trace-redemption-create",
	})
	if err != nil {
		t.Fatal(err)
	}

	verified, err := service.Verify(context.Background(), VerifyRequest{
		RedemptionID:     created.ID,
		MerchantID:       "merchant-coffee-demo",
		VerificationCode: created.MerchantVerificationCode,
		TraceID:          "trace-redemption-verify",
	})
	if err != nil {
		t.Fatal(err)
	}
	if verified.Status != "verified" {
		t.Fatalf("expected verified status, got %q", verified.Status)
	}

	replayed, err := service.Verify(context.Background(), VerifyRequest{
		RedemptionID:     created.ID,
		MerchantID:       "merchant-coffee-demo",
		VerificationCode: created.MerchantVerificationCode,
		TraceID:          "trace-redemption-verify-replay",
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ID != verified.ID || replayed.Status != "verified" {
		t.Fatalf("verification replay changed state: %+v", replayed)
	}

	var status string
	var eventCount int
	if err := db.QueryRow(`SELECT status FROM redemptions WHERE redemption_id = $1`, created.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM outbox_events WHERE topic = 'merchant.verified.v1'`).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if status != "verified" || eventCount != 1 {
		t.Fatalf("unexpected verification state: status=%s events=%d", status, eventCount)
	}
}

func TestCreateRedemptionRollsBack(t *testing.T) {
	db := integrationDB(t)
	resetDatabase(t, db)
	if _, err := db.Exec(`UPDATE users SET point_balance = 0 WHERE user_id_hash = $1`, demoUserIDHash); err != nil {
		t.Fatal(err)
	}

	_, err := NewService(db).Create(context.Background(), CreateRequest{
		UserIDHash:     demoUserIDHash,
		JourneyID:      "journey-demo-001",
		OfferID:        "offer-coffee-xinyi",
		IdempotencyKey: "insufficient-points-key",
		TraceID:        "trace-insufficient-points",
	})
	if !errors.Is(err, ErrInsufficientPoints) {
		t.Fatalf("expected ErrInsufficientPoints, got %v", err)
	}

	var quantity int
	var redemptionCount int
	if err := db.QueryRow(`SELECT available_quantity FROM inventory WHERE offer_id = 'offer-coffee-xinyi'`).Scan(&quantity); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM redemptions`).Scan(&redemptionCount); err != nil {
		t.Fatal(err)
	}
	if quantity != 50 || redemptionCount != 0 {
		t.Fatalf("rollback failed: quantity=%d redemptions=%d", quantity, redemptionCount)
	}
}

func TestCreateRedemptionRollsBackWhenInventoryIsEmpty(t *testing.T) {
	db := integrationDB(t)
	resetDatabase(t, db)
	if _, err := db.Exec(`UPDATE inventory SET available_quantity = 0 WHERE offer_id = 'offer-coffee-xinyi'`); err != nil {
		t.Fatal(err)
	}

	_, err := NewService(db).Create(context.Background(), CreateRequest{
		UserIDHash:     demoUserIDHash,
		JourneyID:      "journey-demo-001",
		OfferID:        "offer-coffee-xinyi",
		IdempotencyKey: "out-of-stock-key",
		TraceID:        "trace-out-of-stock",
	})
	if !errors.Is(err, ErrOfferUnavailable) {
		t.Fatalf("expected ErrOfferUnavailable, got %v", err)
	}

	var balance int64
	var redemptionCount int
	if err := db.QueryRow(`SELECT point_balance FROM users WHERE user_id_hash = $1`, demoUserIDHash).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM redemptions`).Scan(&redemptionCount); err != nil {
		t.Fatal(err)
	}
	if balance != 1200 || redemptionCount != 0 {
		t.Fatalf("rollback failed: balance=%d redemptions=%d", balance, redemptionCount)
	}
}

func integrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to run PostgreSQL integration tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func resetDatabase(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000001_core.sql", "000002_demo_seed.sql"} {
		contents, err := os.ReadFile(filepath.Join("..", "..", "migrations", "postgres", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(contents)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
