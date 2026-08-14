package mobility

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

var ErrProviderUnavailable = errors.New("beacon provider unavailable")

type HTTPProvider struct {
	endpoint string
	username string
	password string
	client   *http.Client
}

func NewHTTPProvider(endpoint, username, password string, timeout time.Duration) *HTTPProvider {
	if timeout <= 0 {
		timeout = 800 * time.Millisecond
	}
	return &HTTPProvider{endpoint: strings.TrimSpace(endpoint), username: username, password: password, client: &http.Client{Timeout: timeout}}
}

func (p *HTTPProvider) Resolve(ctx context.Context, observation Observation) (ProviderResponse, error) {
	if p == nil || p.endpoint == "" {
		return ProviderResponse{}, ErrProviderUnavailable
	}
	payload, err := json.Marshal(struct {
		UUID     string `json:"UUID"`
		Major    int64  `json:"Major"`
		Minor    int64  `json:"Minor"`
		Power    int32  `json:"Power"`
		Username string `json:"Username"`
		Password string `json:"Password"`
	}{observation.UUID, observation.Major, observation.Minor, observation.Power, p.username, p.password})
	if err != nil {
		return ProviderResponse{}, ErrProviderUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return ProviderResponse{}, ErrProviderUnavailable
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return ProviderResponse{}, ErrProviderUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return ProviderResponse{}, ErrProviderUnavailable
	}
	var beacon ProviderResponse
	if err := json.NewDecoder(response.Body).Decode(&beacon); err != nil {
		return ProviderResponse{}, ErrProviderUnavailable
	}
	return beacon, nil
}
