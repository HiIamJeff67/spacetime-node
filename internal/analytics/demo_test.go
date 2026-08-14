package analytics

import "testing"

func TestDemoEngagementEventsAreAnalyticsEvents(t *testing.T) {
	events, err := DemoEngagementEvents("journey-1", "recommendation-1", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("got %d events", len(events))
	}
	for _, data := range events {
		if _, err := DecodeEvent(data); err != nil {
			t.Fatalf("generated invalid event: %v", err)
		}
	}
}
