package mobility

import (
	"context"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
)

type Service struct {
	v1.UnimplementedMobilityServiceServer
	resolver *Resolver
}

func NewService(resolver *Resolver) *Service { return &Service{resolver: resolver} }

func (s *Service) ResolveBeacon(ctx context.Context, request *v1.ResolveBeaconRequest) (*v1.ResolveBeaconResponse, error) {
	if s == nil || s.resolver == nil || request == nil {
		return nil, v1.ErrorInvalidRequest("beacon resolver is unavailable")
	}
	resolved, err := s.resolver.Resolve(ctx, Observation{UUID: request.GetUuid(), Major: request.GetMajor(), Minor: request.GetMinor(), Power: request.GetPower()})
	if err != nil {
		return nil, v1.ErrorInvalidRequest("%s", err)
	}
	return &v1.ResolveBeaconResponse{Context: &v1.BeaconContext{
		StationId: resolved.StationID, LineId: resolved.LineID, PositionId: resolved.PositionID,
		StationName: resolved.StationName, Source: resolved.Source, Confidence: resolved.Confidence, NearExit: resolved.NearExit,
	}}, nil
}
