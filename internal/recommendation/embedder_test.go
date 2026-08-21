package recommendation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPEmbedderUsesCanonicalDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var request struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "demo-semantic" || request.Input != "Coffee\nNear R04\ncoffee" {
			t.Fatalf("unexpected embedding input: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer server.Close()

	embedder := NewHTTPEmbedder(server.URL, "demo-semantic", 3, server.Client())
	vector, err := embedder(context.Background(), OfferDocument{Title: "Coffee", Description: "Near R04", Category: "coffee"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vector) != 3 || vector[0] != 0.1 || vector[2] != 0.3 {
		t.Fatalf("unexpected vector: %v", vector)
	}
}

func TestProfileEmbeddingIsVersionedAndIncludesFeedbackWeights(t *testing.T) {
	var captured OfferDocument
	service := NewRecommendationService(nil, nil, nil).
		WithEmbeddingModel("semantic-demo-v2").
		WithQueryEmbedder(func(_ context.Context, document OfferDocument) ([]float32, error) {
			captured = document
			return []float32{0.1, 0.2}, nil
		})
	event := EntryEvent{UserIDHash: DemoUserIDHash}
	event.Payload.StationID = "R04"
	event.Payload.LineID = "R"
	event.Payload.PositionID = "R04-1"

	embedding, err := service.ProfileEmbedding(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if embedding.EmbeddingModel != "semantic-demo-v2" || embedding.ContentVersion == "" || len(embedding.Vector) != 2 {
		t.Fatalf("unexpected profile embedding metadata: %+v", embedding)
	}
	if captured.Title != "user profile" || !strings.Contains(captured.Description, "categories=coffee,lunch") {
		t.Fatalf("profile document omitted preferences: %+v", captured)
	}
}
