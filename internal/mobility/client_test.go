package mobility

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	kratoshttp "github.com/go-kratos/kratos/v3/transport/http"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
)

func TestHTTPClientResolvesMobilityEndpoint(t *testing.T) {
	transport, err := kratoshttp.NewClient(context.Background(), kratoshttp.WithEndpoint("http://mobility.test"), kratoshttp.WithTransport(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/mobility/beacon/resolve" {
			return nil, &unexpectedPayloadError{payload: request.URL.Path}
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"context":{"station_id":"R04","line_id":"R","position_id":"R04-003","station_name":"信義安和","source":"provider","confidence":0.95,"near_exit":true}}`)), Header: http.Header{"Content-Type": []string{"application/protojson"}}}, nil
	})))
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	client := &HTTPClient{transport: transport, service: v1.NewMobilityServiceHTTPClient(transport)}
	defer client.Close()

	resolved, err := client.Resolve(t.Context(), Observation{UUID: "uuid", Major: 8, Minor: 55, Power: 2222})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.StationID != "R04" || resolved.PositionID != "R04-003" || !resolved.NearExit {
		t.Fatalf("unexpected resolved context: %+v", resolved)
	}
}
