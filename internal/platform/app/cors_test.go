package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSFilterAllowsConfiguredOriginAndPreflight(t *testing.T) {
	filter := corsFilter("https://demo.example, http://localhost:5173")
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	request := httptest.NewRequest(http.MethodOptions, "/v1/users/me", nil)
	request.Header.Set("Origin", "https://demo.example")
	request.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	recorder := httptest.NewRecorder()

	filter(next).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://demo.example" {
		t.Fatalf("allow origin = %q", got)
	}
}

func TestCORSFilterRejectsUnknownPreflight(t *testing.T) {
	filter := corsFilter("https://demo.example")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/v1/users/me", nil)
	request.Header.Set("Origin", "https://attacker.example")

	filter(http.NotFoundHandler()).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected allow origin = %q", got)
	}
}

func TestCORSFilterLeavesNonBrowserRequestsUnchanged(t *testing.T) {
	filter := corsFilter("https://demo.example")
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	recorder := httptest.NewRecorder()
	filter(next).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTeapot)
	}
}
