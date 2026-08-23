package recommendation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQdrantSearchReturnsOfferIDsFromPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/collections/offer_embeddings_v1/points/search" {
			t.Fatalf("unexpected qdrant request: %s %s", r.Method, r.URL.Path)
		}
		var request struct {
			Vector []float32 `json:"vector"`
			Limit  int       `json:"limit"`
			Filter struct {
				Must []struct {
					Key   string `json:"key"`
					Match struct {
						Any   []string `json:"any"`
						Value string   `json:"value"`
					} `json:"match"`
				} `json:"must"`
			} `json:"filter"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Vector) != 2 || request.Limit != 3 {
			t.Fatalf("unexpected qdrant request body: %+v", request)
		}
		if len(request.Filter.Must) != 2 || request.Filter.Must[0].Key != "embedding_model" || request.Filter.Must[0].Match.Value != "demo-hash-v1" || request.Filter.Must[1].Key != "station_ids" || len(request.Filter.Must[1].Match.Any) != 1 || request.Filter.Must[1].Match.Any[0] != "R04" {
			t.Fatalf("qdrant request did not isolate model and station: %+v", request.Filter.Must)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[{"id":1,"score":0.91,"payload":{"offer_id":"offer-coffee-xinyi"}},{"id":"2","score":0.72,"payload":{}}]}`))
	}))
	defer server.Close()

	candidates, err := NewQdrantClient(server.URL, server.Client()).Search(context.Background(), "offer_embeddings_v1", []float32{0.1, 0.2}, "R04", "demo-hash-v1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0].OfferID != "offer-coffee-xinyi" || candidates[1].OfferID != "2" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestQdrantPing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/readyz" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := NewQdrantClient(server.URL, server.Client()).Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
