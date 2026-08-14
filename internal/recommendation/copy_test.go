package recommendation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func demoCopyFacts() CopyFacts {
	return CopyFacts{
		StationName:         "信義安和",
		OfferName:           "通勤咖啡折抵 50 元",
		PointsCost:          80,
		DiscountDescription: "信義安和站附近咖啡折抵優惠。",
		ReasonLabels:        []string{"near_predicted_destination", "within_budget"},
	}
}

func TestTemplateCopyAndFallback(t *testing.T) {
	facts := demoCopyFacts()
	template, err := TemplateCopy(facts)
	if err != nil || template.Tone != "friendly" {
		t.Fatalf("unexpected template: %+v, %v", template, err)
	}
	invalid := func(context.Context, CopyFacts) (CopyOutput, error) {
		return CopyOutput{Title: "完全不同的優惠", Body: "只要 999 點。", Tone: "friendly"}, nil
	}
	output, source, err := GenerateCopy(context.Background(), facts, invalid, time.Second)
	if err != nil || source != "template" || output != template {
		t.Fatalf("invalid LLM output did not fallback: %+v source=%s err=%v", output, source, err)
	}
	timeoutGenerator := func(ctx context.Context, _ CopyFacts) (CopyOutput, error) {
		<-ctx.Done()
		return CopyOutput{}, ctx.Err()
	}
	output, source, err = GenerateCopy(context.Background(), facts, timeoutGenerator, time.Millisecond)
	if err != nil || source != "template" || output != template {
		t.Fatalf("timeout did not fallback: %+v source=%s err=%v", output, source, err)
	}
}

func TestCopyGeneratorErrorFallsBack(t *testing.T) {
	facts := demoCopyFacts()
	template, err := TemplateCopy(facts)
	if err != nil {
		t.Fatal(err)
	}
	output, source, err := GenerateCopy(context.Background(), facts, func(context.Context, CopyFacts) (CopyOutput, error) {
		return CopyOutput{}, ErrCopyGeneration
	}, time.Second)
	if err != nil || source != "template" || output != template {
		t.Fatalf("generator error did not fallback: %+v source=%s err=%v", output, source, err)
	}
}

func TestHTTPCopyGenerator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected LLM request: %s %s", r.Method, r.URL.Path)
		}
		var request struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Model != "demo-model" {
			t.Fatalf("unexpected LLM request body: %+v err=%v", request, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"title\":\"通勤咖啡折抵 50 元\",\"body\":\"通勤咖啡折抵 50 元：信義安和站附近咖啡折抵優惠。使用 80 點\",\"tone\":\"friendly\"}"}}]}`))
	}))
	defer server.Close()

	output, err := NewHTTPCopyGenerator(server.URL, "demo-model", server.Client())(context.Background(), demoCopyFacts())
	if err != nil || output.Tone != "friendly" {
		t.Fatalf("unexpected HTTP copy: %+v err=%v", output, err)
	}
	if err := ValidateCopyOutput(demoCopyFacts(), output); err != nil {
		t.Fatalf("HTTP output failed facts validation: %v", err)
	}
}

func TestHTTPCopyGeneratorSchemaMismatchFallsBack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"title\":\"通勤咖啡折抵 50 元\",\"body\":\"信義安和站附近咖啡折抵優惠。使用 80 點\",\"tone\":\"friendly\",\"cta\":\"立即購買\"}"}}]}`))
	}))
	defer server.Close()

	facts := demoCopyFacts()
	template, err := TemplateCopy(facts)
	if err != nil {
		t.Fatal(err)
	}
	output, source, err := GenerateCopy(context.Background(), facts, NewHTTPCopyGenerator(server.URL, "demo-model", server.Client()), time.Second)
	if err != nil || source != "template" || output != template {
		t.Fatalf("schema mismatch did not fallback: %+v source=%s err=%v", output, source, err)
	}
}

func TestCopyValidationRejectsNewNumber(t *testing.T) {
	output := CopyOutput{Title: "通勤咖啡折抵 50 元", Body: "信義安和站附近咖啡折抵優惠。使用 999 點", Tone: "friendly"}
	if err := ValidateCopyOutput(demoCopyFacts(), output); err != ErrInvalidCopyOutput {
		t.Fatalf("expected facts violation, got %v", err)
	}
}

func TestTemplateCopyHandlesMaximumFactLengths(t *testing.T) {
	output, err := TemplateCopy(CopyFacts{
		StationName:         strings.Repeat("站", 80),
		OfferName:           strings.Repeat("優惠", 60),
		PointsCost:          999999,
		DiscountDescription: strings.Repeat("折抵", 80),
		ReasonLabels:        []string{strings.Repeat("理由", 20)},
	})
	if err != nil || output.Title == "" || output.Body == "" {
		t.Fatalf("maximum valid facts should produce template: %+v err=%v", output, err)
	}
}
