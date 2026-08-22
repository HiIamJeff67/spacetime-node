package mobility

import (
	"context"
	"errors"
	"testing"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
)

type fakeProvider struct {
	response ProviderResponse
	err      error
}

func TestServiceResolvesSanitizedFixture(t *testing.T) {
	response, err := NewService(DefaultResolver(nil)).ResolveBeacon(context.Background(), &v1.ResolveBeaconRequest{Uuid: "demo-beacon", Major: 1, Minor: 4})
	if err != nil || response.GetContext().GetStationId() != "R04" || response.GetContext().GetSource() != "fixture" {
		t.Fatalf("unexpected fixture response: %+v, %v", response, err)
	}
}

func (f fakeProvider) Resolve(context.Context, Observation) (ProviderResponse, error) {
	return f.response, f.err
}

func TestResolverNormalizesProviderAndCaches(t *testing.T) {
	resolver := NewResolver(fakeProvider{response: ProviderResponse{SID: "R04", LID: "R", POSINO: "exit-3", StationName: "ignored"}}, map[string]StationMapping{
		"R04|R": {StationID: "R04", LineID: "R", StationName: "信義安和"},
	}, nil, Context{})
	first, err := resolver.Resolve(context.Background(), Observation{UUID: "demo", Major: 1, Minor: 4})
	if err != nil || first.StationID != "R04" || first.Source != "provider" || !first.NearExit {
		t.Fatalf("unexpected provider context: %+v, %v", first, err)
	}
	second, err := resolver.Resolve(context.Background(), Observation{UUID: "demo", Major: 1, Minor: 4})
	if err != nil || second.Source != "cache" {
		t.Fatalf("unexpected cached context: %+v, %v", second, err)
	}
}

func TestResolverNormalizesTRTCStationCode(t *testing.T) {
	resolver := NewResolver(fakeProvider{response: ProviderResponse{
		SID: "BL19", LID: "BL", POSINO: "BL19-006", POSITION: "出口 5(富台國宅)", STATIONID: "094", StationName: "永春",
	}}, nil, nil, Context{})

	resolved, err := resolver.Resolve(context.Background(), Observation{UUID: "trtc", Major: 8, Minor: 55})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.StationID != "BL19" || resolved.LineID != "BL" || resolved.StationName != "永春" || resolved.PositionID != "BL19-006" {
		t.Fatalf("unexpected TRTC context: %+v", resolved)
	}
}

func TestDefaultResolverNormalizesMallBeaconAlias(t *testing.T) {
	resolver := DefaultResolver(fakeProvider{response: ProviderResponse{SID: "BL07MALL", LID: "BL", POSINO: "出口 1", StationName: "板橋"}})
	resolved, err := resolver.Resolve(context.Background(), Observation{UUID: "mall", Major: 1, Minor: 1})
	if err != nil || resolved.StationID != "BL07" || resolved.LineID != "BL" {
		t.Fatalf("unexpected mall alias context: %+v, %v", resolved, err)
	}
}

func TestResolverFallsBackWhenProviderFails(t *testing.T) {
	resolver := NewResolver(fakeProvider{err: errors.New("timeout")}, nil, map[string]Context{
		"demo:1:4": {StationID: "R04", LineID: "R", PositionID: "exit-3", StationName: "信義安和"},
	}, Context{})
	resolved, err := resolver.Resolve(context.Background(), Observation{UUID: "demo", Major: 1, Minor: 4})
	if err != nil || resolved.Source != "fixture" || resolved.StationID != "R04" {
		t.Fatalf("unexpected fallback context: %+v, %v", resolved, err)
	}
}
