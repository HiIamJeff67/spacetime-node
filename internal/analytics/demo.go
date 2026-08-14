package analytics

import (
	"encoding/json"
	"fmt"
	"time"
)

// DemoEngagementEvents creates replay-safe engagement events for the local demo.
// The deterministic event IDs make rerunning the command harmless to the
// ReplacingMergeTree projection.
func DemoEngagementEvents(journeyID, recommendationID, userIDHash, traceID string) ([][]byte, error) {
	if journeyID == "" || recommendationID == "" || userIDHash == "" || traceID == "" {
		return nil, ErrInvalidEvent
	}
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	types := []struct {
		eventType string
		payload   map[string]string
	}{
		{"notification.sent.v1", map[string]string{"notification_id": recommendationID + "-notification", "channel": "demo"}},
		{"notification.delivered.v1", map[string]string{"notification_id": recommendationID + "-notification", "channel": "demo"}},
		{"recommendation.impressed.v1", map[string]string{"surface": "demo"}},
		{"recommendation.clicked.v1", map[string]string{"surface": "demo"}},
	}
	events := make([][]byte, 0, len(types))
	for index, item := range types {
		payload, err := json.Marshal(item.payload)
		if err != nil {
			return nil, err
		}
		event, err := json.Marshal(struct {
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
		}{
			EventID: fmt.Sprintf("demo-%s-%d", recommendationID, index+1), EventType: item.eventType, SchemaVersion: 1,
			OccurredAt: timestamp, Producer: "analytics-demo", TraceID: traceID, JourneyID: journeyID,
			RecommendationID: recommendationID, UserIDHash: userIDHash, Payload: payload,
		})
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}
