package user

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
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
