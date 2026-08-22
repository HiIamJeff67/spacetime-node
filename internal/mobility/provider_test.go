package mobility

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHTTPProviderUsesTRTCContract(t *testing.T) {
	provider := NewHTTPProvider("http://trtc.test/GetBeaconInfo", "user", "pass", time.Second)
	provider.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.Header.Get("Content-Type"))
		}
		var payload struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Beacon   struct {
				UUID  string `json:"UUID"`
				Major string `json:"MAJOR"`
				Minor string `json:"MINOR"`
				Power string `json:"POWER"`
			} `json:"beacon"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			return nil, err
		}
		if payload.Username != "user" || payload.Password != "pass" || payload.Beacon.UUID != "uuid" || payload.Beacon.Major != "8" || payload.Beacon.Minor != "55" || payload.Beacon.Power != "2222" {
			t.Errorf("unexpected TRTC payload: %+v", payload)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"d":{"BID":"1079","SID":"BL19","LID":"BL","POSINO":"BL19-006","POSITION":"出口 5(富台國宅)","STATIONID":"094","STATION_NAME":"永春"}}`)), Header: make(http.Header)}, nil
	})

	response, err := provider.Resolve(context.Background(), Observation{
		UUID: "uuid", Major: 8, Minor: 55, Power: 2222,
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if response.SID != "BL19" || response.LID != "BL" || response.StationName != "永春" {
		t.Fatalf("unexpected provider response: %+v", response)
	}
}

func TestHTTPProviderAcceptsEmptyTRTCResult(t *testing.T) {
	provider := NewHTTPProvider("http://trtc.test/GetBeaconInfo", "user", "pass", time.Second)
	provider.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"d":null}`)), Header: make(http.Header)}, nil
	})

	response, err := provider.Resolve(context.Background(), Observation{UUID: "uuid"})
	if err != nil || response.SID != "" {
		t.Fatalf("expected an empty successful result, got %+v, %v", response, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type unexpectedPayloadError struct{ payload any }

func (e *unexpectedPayloadError) Error() string { return "unexpected payload" }
