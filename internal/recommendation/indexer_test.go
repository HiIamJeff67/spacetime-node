package recommendation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOfferIndexerBootstrapsAndUpserts(t *testing.T) {
	var upsert QdrantPoint
	var deleted bool
	var collectionCreated bool
	indexes := make(map[string]string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/collections/offer_embeddings_v1" {
			var body struct {
				Vectors struct {
					Size     int    `json:"size"`
					Distance string `json:"distance"`
				} `json:"vectors"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			collectionCreated = body.Vectors.Size == 2 && body.Vectors.Distance == "Cosine"
		}
		if r.Method == http.MethodPut && r.URL.Path == "/collections/offer_embeddings_v1/index" {
			var body struct {
				FieldName   string `json:"field_name"`
				FieldSchema string `json:"field_schema"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			indexes[body.FieldName] = body.FieldSchema
		}
		if r.Method == http.MethodPut && r.URL.Path == "/collections/offer_embeddings_v1/points" {
			var body struct {
				Points []QdrantPoint `json:"points"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			upsert = body.Points[0]
		}
		if r.Method == http.MethodPost && r.URL.Path == "/collections/offer_embeddings_v1/points/delete" {
			deleted = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	indexer := NewOfferIndexer(nil, NewQdrantClient(server.URL, server.Client()), func(context.Context, OfferDocument) ([]float32, error) {
		return []float32{0.1, 0.2}, nil
	}, "demo-embedding-v1")
	if err := indexer.Bootstrap(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	if !collectionCreated || indexes["station_ids"] != "keyword" || indexes["category"] != "keyword" || indexes["content_version"] != "integer" {
		t.Fatalf("unexpected qdrant bootstrap: collection=%v indexes=%v", collectionCreated, indexes)
	}
	event := []byte(`{"event_type":"offer.changed.v1","schema_version":1,"payload":{"offer_id":"offer-coffee-xinyi","content_version":2,"change_type":"UPSERT","title":"咖啡","description":"站內咖啡","station_id":"R04","category":"coffee"}}`)
	if err := indexer.HandleOfferChanged(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if upsert.ID != "offer-coffee-xinyi" || len(upsert.Vector) != 2 || upsert.Payload["embedding_model"] != "demo-embedding-v1" {
		t.Fatalf("unexpected upsert: %+v", upsert)
	}
	if err := indexer.HandleOfferChanged(context.Background(), []byte(`{"event_type":"offer.changed.v1","schema_version":1,"payload":{"offer_id":"offer-coffee-xinyi","content_version":3,"change_type":"DELETE"}}`)); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("delete was not sent to qdrant")
	}
}

func TestOfferIndexerRejectsInvalidEvent(t *testing.T) {
	indexer := NewOfferIndexer(nil, nil, nil, "demo")
	if err := indexer.HandleOfferChanged(context.Background(), []byte(`{"event_type":"other.v1"}`)); err != ErrInvalidOfferChangedEvent {
		t.Fatalf("expected invalid event, got %v", err)
	}
}
