package recommendation

import (
	"context"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

func TestDemoPreferenceFallback(t *testing.T) {
	store := NewPreferenceStore(nil, time.Minute)
	preference, err := store.Get(context.Background(), DemoUserIDHash)
	if err != nil {
		t.Fatal(err)
	}
	if preference.Source != "fallback" || preference.PredictedDestination != "R04" || preference.BudgetMaxPoints != 300 {
		t.Fatalf("unexpected demo fallback: %+v", preference)
	}
}

func TestMatchesPreferredCategoryUsesStructuredValues(t *testing.T) {
	if !matchesPreferredCategory("Coffee", []string{"coffee", "lunch"}) {
		t.Fatal("expected category match")
	}
	if matchesPreferredCategory("coffee", []string{"咖啡"}) {
		t.Fatal("did not expect translation or substring matching")
	}
}

func TestFeedbackDelta(t *testing.T) {
	if weight, clicks, dismisses, redemptions := feedbackDelta("recommendation.clicked.v1"); weight != 0.5 || clicks != 1 || dismisses != 0 || redemptions != 0 {
		t.Fatalf("unexpected click delta: %v %d %d %d", weight, clicks, dismisses, redemptions)
	}
	if weight, _, dismisses, _ := feedbackDelta(RecommendationDismissedTopic); weight >= 0 || dismisses != 1 {
		t.Fatalf("unexpected dismiss delta: %v %d", weight, dismisses)
	}
	if weight, _, _, redemptions := feedbackDelta(RedemptionSucceededTopic); weight != 1.5 || redemptions != 1 {
		t.Fatalf("unexpected redemption delta: %v %d", weight, redemptions)
	}
}

func TestPreferenceCacheHitAndExpiry(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set TEST_REDIS_ADDR to run Redis integration tests")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.Del(context.Background(), preferenceKey(DemoUserIDHash)).Err(); err != nil {
		t.Fatal(err)
	}

	store := NewPreferenceStore(client, 2*time.Second)
	fallback, err := store.Get(context.Background(), DemoUserIDHash)
	if err != nil || fallback.Source != "fallback" {
		t.Fatalf("cache miss did not use fallback: source=%s err=%v", fallback.Source, err)
	}
	cached, err := store.Get(context.Background(), DemoUserIDHash)
	if err != nil || cached.Source != "redis" || cached.PredictedDestination != "R04" {
		t.Fatalf("cache hit failed: source=%s destination=%s err=%v", cached.Source, cached.PredictedDestination, err)
	}
	if ttl, err := client.TTL(context.Background(), preferenceKey(DemoUserIDHash)).Result(); err != nil || ttl <= 0 {
		t.Fatalf("cache TTL missing: ttl=%s err=%v", ttl, err)
	}
	time.Sleep(2200 * time.Millisecond)
	expired, err := store.Get(context.Background(), DemoUserIDHash)
	if err != nil || expired.Source != "fallback" {
		t.Fatalf("expired cache did not fall back: source=%s err=%v", expired.Source, err)
	}
}

func TestPreferenceLoadsPersistedUserSettings(t *testing.T) {
	db := integrationDB(t)
	resetDatabase(t, db)
	store := NewPreferenceStore(nil, time.Minute).WithDB(db)
	preference, err := store.Get(context.Background(), DemoUserIDHash)
	if err != nil {
		t.Fatal(err)
	}
	if preference.Source != "postgres" || preference.PredictedDestination != "R04" || preference.BudgetMaxPoints != 300 {
		t.Fatalf("unexpected persisted preference: %+v", preference)
	}
}
