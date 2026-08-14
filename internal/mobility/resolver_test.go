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
	resolver := NewResolver(fakeProvider{response: ProviderResponse{SID: "R", LID: "04", POSINO: "exit-3", StationName: "ignored"}}, map[string]StationMapping{
		"R|04": {StationID: "R04", LineID: "R", StationName: "信義安和"},
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

func TestResolverFallsBackWhenProviderFails(t *testing.T) {
	resolver := NewResolver(fakeProvider{err: errors.New("timeout")}, nil, map[string]Context{
		"demo:1:4": {StationID: "R04", LineID: "R", PositionID: "exit-3", StationName: "信義安和"},
	}, Context{})
	resolved, err := resolver.Resolve(context.Background(), Observation{UUID: "demo", Major: 1, Minor: 4})
	if err != nil || resolved.Source != "fixture" || resolved.StationID != "R04" {
		t.Fatalf("unexpected fallback context: %+v, %v", resolved, err)
	}
}
