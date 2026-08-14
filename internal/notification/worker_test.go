package notification

import (
	"testing"
	"time"
)

func TestWithinNotificationWindow(t *testing.T) {
	location, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatal(err)
	}
	inside := time.Date(2026, 8, 13, 9, 30, 0, 0, location)
	outside := time.Date(2026, 8, 13, 21, 0, 0, 0, location)
	if !WithinNotificationWindow(inside, "Asia/Taipei", "08:00", "20:00") {
		t.Fatal("expected daytime window to include 09:30")
	}
	if WithinNotificationWindow(outside, "Asia/Taipei", "08:00", "20:00") {
		t.Fatal("expected daytime window to exclude 21:00")
	}
	if !WithinNotificationWindow(outside, "Asia/Taipei", "20:00", "08:00") {
		t.Fatal("expected overnight window to include 21:00")
	}
	if !WithinNotificationWindow(inside, "Asia/Taipei", "09:30", "09:30") {
		t.Fatal("expected equal bounds to represent all day")
	}
}

func TestDecodeRecommendationCreatedEvent(t *testing.T) {
	event, err := DecodeRecommendationCreatedEvent([]byte(`{"event_id":"evt-1","event_type":"recommendation.created.v1","schema_version":1,"trace_id":"trace-1","journey_id":"journey-1","recommendation_id":"rec-1","user_id_hash":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","payload":{"recommendation_id":"rec-1","offer_id":"offer-1"}}`))
	if err != nil || event.RecommendationID != "rec-1" {
		t.Fatalf("decode failed: %+v %v", event, err)
	}
}
