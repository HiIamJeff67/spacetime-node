package recommendation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
)

var (
	ErrInvalidOfferChangedEvent = errors.New("invalid offer changed event")
	ErrEmbeddingUnavailable     = errors.New("embedding unavailable")
)

const OfferEmbeddingCollection = "offer_embeddings_v1"

type OfferDocument struct {
	OfferID        string
	Title          string
	Description    string
	StationID      string
	Category       string
	ContentVersion int64
}

type Embedder func(context.Context, OfferDocument) ([]float32, error)

func HashEmbedder(dimension int) Embedder {
	return func(_ context.Context, document OfferDocument) ([]float32, error) {
		if dimension < 1 {
			return nil, ErrEmbeddingUnavailable
		}
		// ponytail: deterministic local vector keeps the demo self-contained; replace with a model adapter when semantic quality is measured.
		digest := sha256.Sum256([]byte(document.Title + "\n" + document.Description))
		vector := make([]float32, dimension)
		for index := range vector {
			vector[index] = float32(digest[index%len(digest)])/127.5 - 1
		}
		return vector, nil
	}
}

type OfferIndexer struct {
	db             *sql.DB
	qdrant         *QdrantClient
	embedder       Embedder
	collection     string
	embeddingModel string
}

func NewOfferIndexer(db *sql.DB, qdrant *QdrantClient, embedder Embedder, embeddingModel string) *OfferIndexer {
	return &OfferIndexer{
		db:             db,
		qdrant:         qdrant,
		embedder:       embedder,
		collection:     OfferEmbeddingCollection,
		embeddingModel: embeddingModel,
	}
}

func (i *OfferIndexer) Bootstrap(ctx context.Context, dimension int) error {
	if i == nil || i.qdrant == nil || i.collection == "" || i.embeddingModel == "" || dimension < 1 {
		return ErrInvalidOfferChangedEvent
	}
	if err := i.qdrant.EnsureCollection(ctx, i.collection, dimension); err != nil {
		return err
	}
	for _, field := range []struct{ name, schema string }{
		{name: "station_ids", schema: "keyword"},
		{name: "category", schema: "keyword"},
		{name: "content_version", schema: "integer"},
	} {
		if err := i.qdrant.EnsurePayloadIndex(ctx, i.collection, field.name, field.schema); err != nil {
			return err
		}
	}
	return i.reindexActiveOffers(ctx)
}

func (i *OfferIndexer) reindexActiveOffers(ctx context.Context) error {
	if i == nil || i.db == nil {
		return nil
	}
	if i.embedder == nil {
		return ErrEmbeddingUnavailable
	}
	rows, err := i.db.QueryContext(ctx, `
		SELECT offer_id, title, description, station_id, category, content_version
		FROM offers
		WHERE is_active = true`)
	if err != nil {
		return err
	}
	defer rows.Close()

	points := make([]QdrantPoint, 0)
	for rows.Next() {
		var document OfferDocument
		if err := rows.Scan(&document.OfferID, &document.Title, &document.Description, &document.StationID, &document.Category, &document.ContentVersion); err != nil {
			return err
		}
		vector, err := i.embedder(ctx, document)
		if err != nil {
			return err
		}
		points = append(points, QdrantPoint{
			ID:     document.OfferID,
			Vector: vector,
			Payload: map[string]any{
				"offer_id":        document.OfferID,
				"station_ids":     []string{document.StationID},
				"category":        document.Category,
				"content_version": document.ContentVersion,
				"embedding_model": i.embeddingModel,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(points) == 0 {
		return nil
	}
	return i.qdrant.Upsert(ctx, i.collection, points)
}

func (i *OfferIndexer) HandleOfferChanged(ctx context.Context, data []byte) error {
	if i == nil || i.qdrant == nil {
		return ErrInvalidOfferChangedEvent
	}
	var event struct {
		EventType     string `json:"event_type"`
		SchemaVersion int    `json:"schema_version"`
		Payload       struct {
			OfferID        string `json:"offer_id"`
			ContentVersion int64  `json:"content_version"`
			ChangeType     string `json:"change_type"`
			Title          string `json:"title"`
			Description    string `json:"description"`
			StationID      string `json:"station_id"`
			Category       string `json:"category"`
		} `json:"payload"`
	}
	if json.Unmarshal(data, &event) != nil || event.EventType != "offer.changed.v1" || event.SchemaVersion != 1 || event.Payload.OfferID == "" || event.Payload.ContentVersion < 1 {
		return ErrInvalidOfferChangedEvent
	}
	if event.Payload.ChangeType == "DELETE" {
		return i.qdrant.Delete(ctx, i.collection, []string{event.Payload.OfferID})
	}
	if event.Payload.ChangeType != "UPSERT" {
		return ErrInvalidOfferChangedEvent
	}
	document := OfferDocument{
		OfferID:        event.Payload.OfferID,
		Title:          event.Payload.Title,
		Description:    event.Payload.Description,
		StationID:      event.Payload.StationID,
		Category:       event.Payload.Category,
		ContentVersion: event.Payload.ContentVersion,
	}
	if document.Title == "" || document.Description == "" || document.StationID == "" {
		if err := i.loadOffer(ctx, &document); err != nil {
			return err
		}
	}
	if document.ContentVersion < event.Payload.ContentVersion {
		document.ContentVersion = event.Payload.ContentVersion
	}
	if i == nil || i.embedder == nil || i.qdrant == nil || i.embeddingModel == "" {
		return ErrEmbeddingUnavailable
	}
	vector, err := i.embedder(ctx, document)
	if err != nil || len(vector) == 0 {
		if err != nil {
			return err
		}
		return ErrEmbeddingUnavailable
	}
	return i.qdrant.Upsert(ctx, i.collection, []QdrantPoint{{
		ID:     document.OfferID,
		Vector: vector,
		Payload: map[string]any{
			"offer_id":        document.OfferID,
			"station_ids":     []string{document.StationID},
			"category":        document.Category,
			"content_version": document.ContentVersion,
			"embedding_model": i.embeddingModel,
		},
	}})
}

func (i *OfferIndexer) loadOffer(ctx context.Context, document *OfferDocument) error {
	if i == nil || i.db == nil || document == nil {
		return ErrInvalidOfferChangedEvent
	}
	return i.db.QueryRowContext(ctx, `
		SELECT offer_id, title, description, station_id, category, content_version
		FROM offers WHERE offer_id = $1`, document.OfferID).Scan(
		&document.OfferID, &document.Title, &document.Description, &document.StationID, &document.Category, &document.ContentVersion)
}
