package journey

import (
	"context"
	"testing"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
	"spacetime-node/internal/mobility"
)

type fakeBeaconResolver struct {
	context mobility.Context
	err     error
}

func (f fakeBeaconResolver) Resolve(context.Context, mobility.Observation) (mobility.Context, error) {
	return f.context, f.err
}

func TestResolveEntryContextUsesBeaconAsAuthority(t *testing.T) {
	service := NewService(nil, fakeBeaconResolver{context: mobility.Context{
		StationID: "R02", LineID: "R", PositionID: "R02-001",
	}})
	stationID, lineID, positionID, err := service.resolveEntryContext(context.Background(), &v1.CreateEntryEventRequest{
		StationId: "R02",
		Beacon:    &v1.BeaconObservation{Uuid: "uuid", Major: 8, Minor: 55},
	})
	if err != nil {
		t.Fatalf("resolve entry context: %v", err)
	}
	if stationID != "R02" || lineID != "R" || positionID != "R02-001" {
		t.Fatalf("unexpected resolved context: %s %s %s", stationID, lineID, positionID)
	}
}

func TestResolveEntryContextRejectsConflictingStation(t *testing.T) {
	service := NewService(nil, fakeBeaconResolver{context: mobility.Context{StationID: "R02", LineID: "R"}})
	_, _, _, err := service.resolveEntryContext(context.Background(), &v1.CreateEntryEventRequest{
		StationId: "BL03",
		Beacon:    &v1.BeaconObservation{Uuid: "uuid", Major: 8, Minor: 55},
	})
	if err == nil {
		t.Fatal("expected a conflicting station_id to be rejected")
	}
}

func TestResolveEntryContextKeepsStationDemoPath(t *testing.T) {
	service := NewService(nil)
	stationID, lineID, positionID, err := service.resolveEntryContext(context.Background(), &v1.CreateEntryEventRequest{
		StationId: "BL03", LineId: "BL", PositionId: "BL03-001",
	})
	if err != nil {
		t.Fatalf("resolve station entry context: %v", err)
	}
	if stationID != "BL03" || lineID != "BL" || positionID != "BL03-001" {
		t.Fatalf("unexpected station context: %s %s %s", stationID, lineID, positionID)
	}
}

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
