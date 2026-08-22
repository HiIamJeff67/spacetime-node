package mobility

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
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
		Username string `json:"username"`
		Password string `json:"password"`
		Beacon   struct {
			UUID  string `json:"UUID"`
			Major string `json:"MAJOR"`
			Minor string `json:"MINOR"`
			Power string `json:"POWER"`
		} `json:"beacon"`
	}{
		Username: p.username,
		Password: p.password,
		Beacon: struct {
			UUID  string `json:"UUID"`
			Major string `json:"MAJOR"`
			Minor string `json:"MINOR"`
			Power string `json:"POWER"`
		}{
			UUID: observation.UUID, Major: strconv.FormatInt(observation.Major, 10),
			Minor: strconv.FormatInt(observation.Minor, 10), Power: strconv.FormatInt(int64(observation.Power), 10),
		},
	})
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
	var envelope struct {
		Data *ProviderResponse `json:"d"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return ProviderResponse{}, ErrProviderUnavailable
	}
	if envelope.Data == nil {
		return ProviderResponse{}, nil
	}
	return *envelope.Data, nil
}
