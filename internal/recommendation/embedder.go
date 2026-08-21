package recommendation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// CanonicalDocument keeps offer and profile/query text in the same semantic
// input shape while leaving hard eligibility rules outside the vector model.
func CanonicalDocument(document OfferDocument) string {
	parts := make([]string, 0, 3)
	for _, value := range []string{document.Title, document.Description, document.Category} {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "\n")
}

// ProfileDocument is the non-sensitive, rebuildable input to a user profile
// embedding. The user hash is intentionally not part of this document.
type ProfileDocument struct {
	PredictedDestination string
	PreferredCategories  []string
	CategoryWeights      map[string]float64
	BudgetMinPoints      int64
	BudgetMaxPoints      int64
}

// CanonicalProfileDocument keeps profile embedding inputs stable across
// processes so the same preferences and feedback produce the same version.
func CanonicalProfileDocument(document ProfileDocument) string {
	categories := append([]string(nil), document.PreferredCategories...)
	sort.Strings(categories)
	weights := make([]string, 0, len(document.CategoryWeights))
	for category, weight := range document.CategoryWeights {
		weights = append(weights, fmt.Sprintf("%s=%.6f", strings.ToLower(strings.TrimSpace(category)), weight))
	}
	sort.Strings(weights)
	return strings.Join([]string{
		"destination=" + strings.TrimSpace(document.PredictedDestination),
		"categories=" + strings.Join(categories, ","),
		"weights=" + strings.Join(weights, ","),
		fmt.Sprintf("budget=%d-%d", document.BudgetMinPoints, document.BudgetMaxPoints),
	}, "\n")
}

type ProfileEmbedding struct {
	Vector         []float32
	EmbeddingModel string
	ContentVersion string
}

func profileContentVersion(document string) string {
	digest := sha256.Sum256([]byte(document))
	return fmt.Sprintf("sha256:%x", digest[:])
}

// NewHTTPEmbedder accepts the common /v1/embeddings request/response shape.
// It is intentionally one-text-at-a-time because indexing and query creation
// are both small demo workloads.
func NewHTTPEmbedder(baseURL, model string, dimension int, client *http.Client) Embedder {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return func(ctx context.Context, document OfferDocument) ([]float32, error) {
		if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(model) == "" || dimension < 1 {
			return nil, ErrEmbeddingUnavailable
		}
		body, err := json.Marshal(struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}{Model: model, Input: CanonicalDocument(document)})
		if err != nil {
			return nil, fmt.Errorf("%w: encode request: %v", ErrEmbeddingUnavailable, err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/v1/embeddings", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("%w: create request: %v", ErrEmbeddingUnavailable, err)
		}
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%w: request: %v", ErrEmbeddingUnavailable, err)
		}
		defer response.Body.Close()
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			return nil, fmt.Errorf("%w: provider status %s", ErrEmbeddingUnavailable, response.Status)
		}
		var result struct {
			Data []struct {
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&result); err != nil {
			return nil, fmt.Errorf("%w: decode response: %v", ErrEmbeddingUnavailable, err)
		}
		if len(result.Data) != 1 || len(result.Data[0].Embedding) != dimension {
			return nil, fmt.Errorf("%w: expected one vector with dimension %d", ErrEmbeddingUnavailable, dimension)
		}
		return result.Data[0].Embedding, nil
	}
}

func NewConfiguredEmbedder(mode, baseURL, model string, dimension int, client *http.Client) (Embedder, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "hash":
		return HashEmbedder(dimension), nil
	case "http":
		if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(model) == "" {
			return nil, ErrEmbeddingUnavailable
		}
		return NewHTTPEmbedder(baseURL, model, dimension, client), nil
	default:
		return nil, fmt.Errorf("%w: unsupported embedding mode %q", ErrEmbeddingUnavailable, mode)
	}
}
