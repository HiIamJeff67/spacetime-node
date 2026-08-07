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
	"strings"
	"time"
)

var ErrCandidateRecall = errors.New("candidate recall failed")

type QdrantCandidate struct {
	OfferID     string
	VectorScore float64
}

type QdrantClient struct {
	baseURL string
	client  *http.Client
}

func NewQdrantClient(baseURL string, client *http.Client) *QdrantClient {
	if client == nil {
		client = &http.Client{Timeout: 750 * time.Millisecond}
	}
	return &QdrantClient{baseURL: strings.TrimRight(baseURL, "/"), client: client}
}

func (c *QdrantClient) Search(ctx context.Context, collection string, vector []float32, stationID string, limit int) ([]QdrantCandidate, error) {
	if c == nil || c.client == nil || c.baseURL == "" || collection == "" || len(vector) == 0 || limit < 1 {
		return nil, ErrCandidateRecall
	}
	requestBody := struct {
		Vector      []float32     `json:"vector"`
		Limit       int           `json:"limit"`
		WithPayload bool          `json:"with_payload"`
		Filter      *qdrantFilter `json:"filter,omitempty"`
	}{
		Vector:      vector,
		Limit:       limit,
		WithPayload: true,
	}
	if stationID != "" {
		requestBody.Filter = &qdrantFilter{Must: []qdrantCondition{{
			Key:   "station_ids",
			Match: qdrantMatch{Any: []string{stationID}},
		}}}
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, ErrCandidateRecall
	}
	endpoint := fmt.Sprintf("%s/collections/%s/points/search", c.baseURL, url.PathEscape(collection))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, ErrCandidateRecall
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCandidateRecall, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("%w: qdrant status %s", ErrCandidateRecall, response.Status)
	}
	var result struct {
		Result []struct {
			ID      json.RawMessage `json:"id"`
			Score   float64         `json:"score"`
			Payload struct {
				OfferID string `json:"offer_id"`
			} `json:"payload"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: invalid qdrant response: %v", ErrCandidateRecall, err)
	}
	candidates := make([]QdrantCandidate, 0, len(result.Result))
	for _, item := range result.Result {
		offerID := item.Payload.OfferID
		if offerID == "" {
			offerID = qdrantID(item.ID)
		}
		if offerID != "" {
			candidates = append(candidates, QdrantCandidate{OfferID: offerID, VectorScore: item.Score})
		}
	}
	return candidates, nil
}

type qdrantFilter struct {
	Must []qdrantCondition `json:"must,omitempty"`
}

type qdrantCondition struct {
	Key   string      `json:"key"`
	Match qdrantMatch `json:"match"`
}

type qdrantMatch struct {
	Any []string `json:"any,omitempty"`
}

func qdrantID(raw json.RawMessage) string {
	var stringID string
	if json.Unmarshal(raw, &stringID) == nil {
		return stringID
	}
	var numberID json.Number
	if json.Unmarshal(raw, &numberID) == nil {
		return numberID.String()
	}
	return ""
}
