package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrInvalidClickHouseURL = errors.New("invalid ClickHouse URL")

type Writer interface {
	Insert(context.Context, ProductEvent) error
}

type ClickHouse struct {
	baseURL string
	client  *http.Client
}

func NewClickHouse(rawURL string, client *http.Client) (*ClickHouse, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return nil, ErrInvalidClickHouseURL
	}
	database := strings.Trim(parsed.Path, "/")
	query := parsed.Query()
	if database != "" && query.Get("database") == "" {
		query.Set("database", database)
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = query.Encode()
	if parsed.Scheme == "clickhouse" {
		parsed.Scheme = "http"
		if parsed.Port() == "9000" {
			parsed.Host = parsed.Hostname() + ":8123"
		}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, ErrInvalidClickHouseURL
	}
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	return &ClickHouse{baseURL: strings.TrimRight(parsed.String(), "/"), client: client}, nil
}

func (c *ClickHouse) Insert(ctx context.Context, event ProductEvent) error {
	if c == nil || c.client == nil || c.baseURL == "" || event.EventID == "" {
		return ErrInvalidClickHouseURL
	}
	body, err := jsonLine(event)
	if err != nil {
		return err
	}
	endpoint, err := url.Parse(c.baseURL)
	if err != nil {
		return err
	}
	query := endpoint.Query()
	query.Set("query", "INSERT INTO product_events FORMAT JSONEachRow")
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("ClickHouse insert returned %s", response.Status)
	}
	return nil
}

func jsonLine(event ProductEvent) ([]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
