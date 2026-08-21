package analytics

import (
	"encoding/json"
	"errors"
	"time"
)

var ErrInvalidEvent = errors.New("invalid analytics event")

const (
	ConsumerGroup = "analytics-consumer-v1"
	ConsumerName  = "analytics-consumer"
)

var Topics = []string{
	"journey.entered.v1",
	"recommendation.created.v1",
	"notification.sent.v1",
	"notification.delivered.v1",
	"notification.failed.v1",
	"recommendation.impressed.v1",
	"recommendation.clicked.v1",
	"recommendation.dismissed.v1",
	"redemption.succeeded.v1",
	"merchant.verified.v1",
	"visit.attributed.v1",
}

type Event struct {
	EventID          string          `json:"event_id"`
	EventType        string          `json:"event_type"`
	SchemaVersion    int             `json:"schema_version"`
	OccurredAt       string          `json:"occurred_at"`
	Producer         string          `json:"producer"`
	TraceID          string          `json:"trace_id"`
	JourneyID        string          `json:"journey_id"`
	RecommendationID string          `json:"recommendation_id"`
	UserIDHash       string          `json:"user_id_hash"`
	Payload          json.RawMessage `json:"payload"`
}

type ProductEvent struct {
	EventID          string    `json:"event_id"`
	EventType        string    `json:"event_type"`
	OccurredAt       time.Time `json:"occurred_at"`
	Producer         string    `json:"producer"`
	TraceID          string    `json:"trace_id"`
	JourneyID        string    `json:"journey_id"`
	RecommendationID string    `json:"recommendation_id"`
	UserIDHash       string    `json:"user_id_hash"`
	OfferID          string    `json:"offer_id"`
	RedemptionID     string    `json:"redemption_id"`
	MerchantID       string    `json:"merchant_id"`
	CopySource       string    `json:"copy_source"`
	ExperimentID     string    `json:"experiment_id"`
	Variant          string    `json:"variant"`
	AttributionType  string    `json:"attribution_type"`
	Payload          string    `json:"payload"`
}

func DecodeEvent(data []byte) (ProductEvent, error) {
	var event Event
	if json.Unmarshal(data, &event) != nil || event.EventID == "" || event.SchemaVersion != 1 || event.OccurredAt == "" || event.Producer == "" || event.TraceID == "" || !knownTopic(event.EventType) || !json.Valid(event.Payload) {
		return ProductEvent{}, ErrInvalidEvent
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, event.OccurredAt)
	if err != nil {
		return ProductEvent{}, ErrInvalidEvent
	}
	var payload map[string]any
	if json.Unmarshal(event.Payload, &payload) != nil {
		return ProductEvent{}, ErrInvalidEvent
	}
	if event.JourneyID == "" {
		return ProductEvent{}, ErrInvalidEvent
	}
	attributionType := value(payload, "attribution_type")
	if event.EventType == "merchant.verified.v1" {
		attributionType = "observed_pos_verified"
	}
	if event.EventType == "visit.attributed.v1" && attributionType != "inferred_visit" {
		return ProductEvent{}, ErrInvalidEvent
	}
	return ProductEvent{
		EventID: event.EventID, EventType: event.EventType, OccurredAt: occurredAt.UTC(), Producer: event.Producer,
		TraceID: event.TraceID, JourneyID: event.JourneyID, RecommendationID: event.RecommendationID,
		UserIDHash: event.UserIDHash, OfferID: value(payload, "offer_id"), RedemptionID: value(payload, "redemption_id"),
		MerchantID: value(payload, "merchant_id"), CopySource: value(payload, "copy_source"),
		ExperimentID: value(payload, "experiment_id"), Variant: value(payload, "variant"), AttributionType: attributionType, Payload: string(event.Payload),
	}, nil
}

func knownTopic(topic string) bool {
	for _, candidate := range Topics {
		if topic == candidate {
			return true
		}
	}
	return false
}

func value(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}
