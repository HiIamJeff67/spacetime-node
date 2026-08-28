package user

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
)

const demoUserIDHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestGetAndUpdateUserPreferences(t *testing.T) {
	db := integrationDB(t)
	resetDatabase(t, db)
	service := NewService(db)

	profile, err := service.GetUserProfile(context.Background(), &v1.GetUserProfileRequest{UserIdHash: demoUserIDHash})
	if err != nil {
		t.Fatal(err)
	}
	if profile.GetProfile().GetDisplayName() != "Demo Rider" || profile.GetProfile().GetBudgetMaxPoints() != 300 {
		t.Fatalf("unexpected seeded profile: %+v", profile.GetProfile())
	}

	updated, err := service.UpdateUserPreferences(context.Background(), &v1.UpdateUserPreferencesRequest{
		UserIdHash:             demoUserIDHash,
		FavoriteStationIds:     []string{"BL12", "R04"},
		PreferredCategories:    []string{"lunch"},
		BudgetMinPoints:        100,
		BudgetMaxPoints:        500,
		Timezone:               "Asia/Taipei",
		NotificationsEnabled:   true,
		NotificationStartLocal: "07:00",
		NotificationEndLocal:   "10:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := updated.GetProfile()
	if len(got.GetFavoriteStationIds()) != 2 || got.GetBudgetMaxPoints() != 500 || !got.GetNotificationsEnabled() || got.GetNotificationEndLocal() != "10:00" {
		t.Fatalf("preferences were not persisted: %+v", got)
	}
}

func TestAnonymousUserIsProvisioned(t *testing.T) {
	db := integrationDB(t)
	resetDatabase(t, db)
	service := NewService(db)
	userIDHash := "sha256:" + strings.Repeat("b", 64)

	profile, err := service.GetUserProfile(context.Background(), &v1.GetUserProfileRequest{UserIdHash: userIDHash})
	if err != nil {
		t.Fatal(err)
	}
	if profile.GetProfile().GetUserIdHash() != userIDHash || profile.GetProfile().GetDisplayName() != "Demo Rider" {
		t.Fatalf("unexpected anonymous profile: %+v", profile.GetProfile())
	}
	if _, err := service.GetUserProfile(context.Background(), &v1.GetUserProfileRequest{UserIdHash: userIDHash}); err != nil {
		t.Fatal(err)
	}

	var balance int64
	if err := db.QueryRow(`SELECT point_balance FROM users WHERE user_id_hash = $1`, userIDHash).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != anonymousDemoInitialPoints {
		t.Fatalf("unexpected initial balance: %d", balance)
	}
	var ledgerCount int
	if err := db.QueryRow(`SELECT count(*) FROM points_ledger WHERE reason = 'demo_initial_balance' AND user_id = (SELECT user_id FROM users WHERE user_id_hash = $1)`, userIDHash).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 1 {
		t.Fatalf("expected one initial ledger entry, got %d", ledgerCount)
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
	for _, name := range []string{
		"000001_core.sql",
		"000002_demo_seed.sql",
		"000003_recommendation_copy_source.sql",
		"000004_recommendation_candidate_summary.sql",
		"000005_recommendation_latency.sql",
		"000006_user_preferences.sql",
		"000007_notification_subscriptions.sql",
		"000008_offer_category.sql",
		"000009_user_preference_weights.sql",
		"000010_demo_station_catalog.sql",
		"000011_demo_station_catalog_expansion.sql",
		"000012_beacon_station_catalog.sql",
	} {
		contents, err := os.ReadFile(filepath.Join("..", "..", "migrations", "postgres", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(contents)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
