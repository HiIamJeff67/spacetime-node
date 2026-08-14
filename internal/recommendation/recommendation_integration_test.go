package recommendation

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestRecommendPersistsValidatedOfferAndFallsBackFromInvalidCopy(t *testing.T) {
	db := integrationDB(t)
	resetDatabase(t, db)
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[{"id":"stale","score":0.99,"payload":{"offer_id":"offer-dessert-101"}},{"id":"coffee","score":0.80,"payload":{"offer_id":"offer-coffee-xinyi"}}]}`))
	}))
	defer qdrant.Close()

	copyGeneratorCalled := false
	service := NewRecommendationService(db, NewQdrantClient(qdrant.URL, qdrant.Client()), NewPreferenceStore(nil, time.Minute)).WithCopyGenerator(func(context.Context, CopyFacts) (CopyOutput, error) {
		copyGeneratorCalled = true
		return CopyOutput{Title: "完全不同的優惠", Body: "只要 999 點。", Tone: "friendly"}, nil
	}, time.Second)
	recommendation, err := service.Recommend(context.Background(), RecommendationRequest{
		UserIDHash: DemoUserIDHash,
		JourneyID:  "journey-demo-001",
		StationID:  "R04",
		TraceID:    "trace-recommendation-demo",
		Vector:     []float32{0.1, 0.2},
		Limit:      5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if recommendation.OfferID != "offer-coffee-xinyi" || recommendation.CopySource != "template" || len(recommendation.Candidates) != 2 {
		t.Fatalf("unexpected recommendation: %+v", recommendation)
	}
	if !copyGeneratorCalled {
		t.Fatal("copy generator was not called")
	}
	var staleFound bool
	for _, candidate := range recommendation.Candidates {
		if candidate.OfferID == "offer-dessert-101" {
			staleFound = true
			if candidate.Eligible {
				t.Fatal("stale cross-station candidate was not rejected")
			}
		}
	}
	if !staleFound {
		t.Fatal("stale candidate summary was not persisted")
	}

	var topic string
	var eventPayload []byte
	var candidateSummary []byte
	if err := db.QueryRow(`SELECT o.topic, o.payload, r.candidate_summary FROM outbox_events o JOIN recommendations r ON r.recommendation_id = $1 WHERE o.topic = 'recommendation.created.v1'`, recommendation.ID).Scan(&topic, &eventPayload, &candidateSummary); err != nil {
		t.Fatal(err)
	}
	if topic != "recommendation.created.v1" {
		t.Fatalf("unexpected outbox topic: %s", topic)
	}
	var event struct {
		EventID          string `json:"event_id"`
		RecommendationID string `json:"recommendation_id"`
		TraceID          string `json:"trace_id"`
	}
	if err := json.Unmarshal(eventPayload, &event); err != nil {
		t.Fatal(err)
	}
	if event.RecommendationID != recommendation.ID || event.TraceID != "trace-recommendation-demo" || event.EventID == "" {
		t.Fatalf("unexpected recommendation event: %+v", event)
	}
	var summaries []CandidateSummary
	if err := json.Unmarshal(candidateSummary, &summaries); err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("candidate summary was not persisted: %+v", summaries)
	}

	latest, err := service.GetLatest(context.Background(), "journey-demo-001")
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != recommendation.ID || len(latest.Candidates) != 2 || latest.CopySource != "template" || latest.DecisionLatencyMS != recommendation.DecisionLatencyMS {
		t.Fatalf("unexpected latest recommendation: %+v", latest)
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
