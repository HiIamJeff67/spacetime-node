package analytics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDecodeEventProjectsRecommendationFacts(t *testing.T) {
	event, err := DecodeEvent([]byte(`{"event_id":"event-1","event_type":"recommendation.created.v1","schema_version":1,"occurred_at":"2026-08-09T00:00:00Z","producer":"recommendation-service","trace_id":"trace-1","journey_id":"journey-1","recommendation_id":"recommendation-1","user_id_hash":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","payload":{"recommendation_id":"recommendation-1","offer_id":"offer-1","copy_source":"template","experiment_id":"copy-v1","variant":"control"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if event.OfferID != "offer-1" || event.CopySource != "template" || event.Variant != "control" || !event.OccurredAt.Equal(time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestDecodeEventRejectsUnknownTopic(t *testing.T) {
	if _, err := DecodeEvent([]byte(`{"event_id":"event-1","event_type":"unknown.v1","schema_version":1,"occurred_at":"2026-08-09T00:00:00Z","producer":"test","trace_id":"trace-1","payload":{}}`)); err == nil {
		t.Fatal("expected unknown topic rejection")
	}
}

func TestDecodeEventSeparatesInferredVisitFromPOSVerification(t *testing.T) {
	data := []byte(`{"event_id":"event-visit","event_type":"visit.attributed.v1","schema_version":1,"occurred_at":"2026-08-09T00:00:00Z","producer":"attribution-worker","trace_id":"trace-1","journey_id":"journey-1","user_id_hash":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","payload":{"attribution_type":"inferred_visit","confidence":0.8}}`)
	event, err := DecodeEvent(data)
	if err != nil {
		t.Fatal(err)
	}
	if event.AttributionType != "inferred_visit" {
		t.Fatalf("unexpected attribution type: %s", event.AttributionType)
	}

	verified := []byte(`{"event_id":"event-verified","event_type":"merchant.verified.v1","schema_version":1,"occurred_at":"2026-08-09T00:00:00Z","producer":"redemption-service","trace_id":"trace-1","journey_id":"journey-1","payload":{"redemption_id":"redemption-1","merchant_id":"merchant-1","verified_at":"2026-08-09T00:00:00Z"}}`)
	event, err = DecodeEvent(verified)
	if err != nil {
		t.Fatal(err)
	}
	if event.AttributionType != "observed_pos_verified" {
		t.Fatalf("unexpected verification type: %s", event.AttributionType)
	}
}

func TestClickHouseInsertUsesJSONEachRow(t *testing.T) {
	var request *http.Request
	client, err := NewClickHouse("clickhouse://clickhouse:9000/default", &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		request = r
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Insert(context.Background(), ProductEvent{EventID: "event-1"}); err != nil {
		t.Fatal(err)
	}
	if request.URL.Host != "clickhouse:8123" || request.URL.Query().Get("database") != "default" || request.URL.Query().Get("query") != "INSERT INTO product_events FORMAT JSONEachRow" {
		t.Fatalf("unexpected request: %s", request.URL)
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }
