package recommendation

import "testing"

func TestDecodeEntryEvent(t *testing.T) {
	event, err := DecodeEntryEvent([]byte(`{"event_id":"event-1","event_type":"journey.entered.v1","schema_version":1,"trace_id":"trace-1","journey_id":"journey-1","user_id_hash":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","payload":{"station_id":"R04","line_id":"R"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if event.JourneyID != "journey-1" || event.Payload.StationID != "R04" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestDecodeEntryEventRejectsWrongTopic(t *testing.T) {
	if _, err := DecodeEntryEvent([]byte(`{"event_id":"event-1","event_type":"other.v1","schema_version":1,"trace_id":"trace-1","journey_id":"journey-1","user_id_hash":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","payload":{"station_id":"R04"}}`)); err == nil {
		t.Fatal("expected invalid event")
	}
}
