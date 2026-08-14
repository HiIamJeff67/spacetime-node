package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusHandler(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	statusHandler("gateway-service", "dev", "ok").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d", recorder.Code)
	}

	var got status
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Service != "gateway-service" || got.Version != "dev" || got.Status != "ok" {
		t.Fatalf("response = %+v", got)
	}
}

func TestReadinessHandler(t *testing.T) {
	for _, test := range []struct {
		name   string
		check  ReadyCheck
		code   int
		status string
	}{
		{name: "ready", check: func(context.Context) error { return nil }, code: http.StatusOK, status: "ready"},
		{name: "dependency unavailable", check: func(context.Context) error { return errors.New("unavailable") }, code: http.StatusServiceUnavailable, status: "not_ready"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			readinessHandler("test-service", "dev", test.check).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			var got status
			if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != test.code || got.Status != test.status {
				t.Fatalf("code=%d response=%+v", recorder.Code, got)
			}
		})
	}
}
