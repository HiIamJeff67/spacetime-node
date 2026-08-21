package journey

import "testing"

func TestValidUserIDHash(t *testing.T) {
	valid := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if !validUserIDHash(valid) {
		t.Fatal("expected valid user hash")
	}
	for _, invalid := range []string{"", "sha256:short", "sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		if validUserIDHash(invalid) {
			t.Fatalf("expected invalid user hash: %q", invalid)
		}
	}
}

func TestValidRecommendationEventType(t *testing.T) {
	for _, eventType := range []string{recommendationImpressedTopic, recommendationClickedTopic, recommendationDismissedTopic} {
		if !validRecommendationEventType(eventType) {
			t.Fatalf("expected valid recommendation event type: %q", eventType)
		}
	}
	for _, eventType := range []string{"", "journey.entered.v1", "recommendation.impressed.v2"} {
		if validRecommendationEventType(eventType) {
			t.Fatalf("expected invalid recommendation event type: %q", eventType)
		}
	}
}

func TestRecommendationFeedbackEventIDIsDeterministic(t *testing.T) {
	first := recommendationFeedbackEventID("sha256:user", "journey-1", "recommendation-1", "offer-1", recommendationClickedTopic)
	second := recommendationFeedbackEventID("sha256:user", "journey-1", "recommendation-1", "offer-1", recommendationClickedTopic)
	otherJourney := recommendationFeedbackEventID("sha256:user", "journey-2", "recommendation-1", "offer-1", recommendationClickedTopic)
	if first != second {
		t.Fatalf("expected the same event identity for a replay: %q != %q", first, second)
	}
	if first == otherJourney {
		t.Fatal("expected separate journeys to produce separate event identities")
	}
}
