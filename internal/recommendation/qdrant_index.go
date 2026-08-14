package recommendation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/google/uuid"
)

var ErrQdrantWrite = errors.New("qdrant write failed")

type qdrantHTTPError struct {
	status int
}

func (e *qdrantHTTPError) Error() string { return fmt.Sprintf("qdrant status %d", e.status) }

type QdrantPoint struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

func qdrantPointID(id string) string {
	if parsed, err := uuid.Parse(id); err == nil {
		return parsed.String()
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(id)).String()
}

func (c *QdrantClient) EnsureCollection(ctx context.Context, collection string, dimension int) error {
	if c == nil || collection == "" || dimension < 1 {
		return ErrQdrantWrite
	}
	err := c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/collections/%s", url.PathEscape(collection)), map[string]any{
		"vectors": map[string]any{"size": dimension, "distance": "Cosine"},
	})
	var statusErr *qdrantHTTPError
	if errors.As(err, &statusErr) && statusErr.status == http.StatusConflict {
		return nil
	}
	return err
}

func (c *QdrantClient) EnsurePayloadIndex(ctx context.Context, collection, field, schema string) error {
	if c == nil || collection == "" || field == "" || schema == "" {
		return ErrQdrantWrite
	}
	err := c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/collections/%s/index", url.PathEscape(collection)), map[string]string{
		"field_name":   field,
		"field_schema": schema,
	})
	var statusErr *qdrantHTTPError
	if errors.As(err, &statusErr) && statusErr.status == http.StatusConflict {
		return nil
	}
	return err
}

func (c *QdrantClient) Upsert(ctx context.Context, collection string, points []QdrantPoint) error {
	if c == nil || collection == "" || len(points) == 0 {
		return ErrQdrantWrite
	}
	for _, point := range points {
		if point.ID == "" || len(point.Vector) == 0 {
			return ErrQdrantWrite
		}
	}
	wirePoints := make([]QdrantPoint, len(points))
	copy(wirePoints, points)
	for index := range wirePoints {
		wirePoints[index].ID = qdrantPointID(wirePoints[index].ID)
	}
	return c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/collections/%s/points?wait=true", url.PathEscape(collection)), map[string]any{"points": wirePoints})
}

func (c *QdrantClient) Delete(ctx context.Context, collection string, offerIDs []string) error {
	if c == nil || collection == "" || len(offerIDs) == 0 {
		return ErrQdrantWrite
	}
	pointIDs := make([]string, len(offerIDs))
	for index, offerID := range offerIDs {
		pointIDs[index] = qdrantPointID(offerID)
	}
	return c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/delete?wait=true", url.PathEscape(collection)), map[string]any{"points": pointIDs})
}

func (c *QdrantClient) doJSON(ctx context.Context, method, path string, value any) error {
	if c == nil || c.client == nil || c.baseURL == "" {
		return ErrQdrantWrite
	}
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%w: encode request: %v", ErrQdrantWrite, err)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: create request: %v", ErrQdrantWrite, err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrQdrantWrite, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("%w: %w", ErrQdrantWrite, &qdrantHTTPError{status: response.StatusCode})
	}
	return nil
}
