package mobility

import (
	"context"
	"strings"
	"time"

	kratoshttp "github.com/go-kratos/kratos/v3/transport/http"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
)

type Client interface {
	Resolve(context.Context, Observation) (Context, error)
}

type HTTPClient struct {
	transport *kratoshttp.Client
	service   v1.MobilityServiceHTTPClient
}

func NewHTTPClient(endpoint string, timeout time.Duration) (*HTTPClient, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, ErrBeaconUnavailable
	}
	if timeout <= 0 {
		timeout = 800 * time.Millisecond
	}
	transport, err := kratoshttp.NewClient(context.Background(), kratoshttp.WithEndpoint(strings.TrimSpace(endpoint)), kratoshttp.WithTimeout(timeout))
	if err != nil {
		return nil, err
	}
	return &HTTPClient{transport: transport, service: v1.NewMobilityServiceHTTPClient(transport)}, nil
}

func (c *HTTPClient) Resolve(ctx context.Context, observation Observation) (Context, error) {
	if c == nil || c.service == nil {
		return Context{}, ErrBeaconUnavailable
	}
	response, err := c.service.ResolveBeacon(ctx, &v1.ResolveBeaconRequest{
		Uuid: observation.UUID, Major: observation.Major, Minor: observation.Minor, Power: observation.Power,
	})
	if err != nil || response == nil || response.GetContext() == nil || response.GetContext().GetStationId() == "" {
		return Context{}, ErrBeaconUnavailable
	}
	resolved := response.GetContext()
	return Context{
		StationID: resolved.GetStationId(), LineID: resolved.GetLineId(), PositionID: resolved.GetPositionId(),
		StationName: resolved.GetStationName(), Source: resolved.GetSource(), Confidence: resolved.GetConfidence(),
		NearExit: resolved.GetNearExit(),
	}, nil
}

func (c *HTTPClient) Close() error {
	if c == nil || c.transport == nil {
		return nil
	}
	return c.transport.Close()
}
