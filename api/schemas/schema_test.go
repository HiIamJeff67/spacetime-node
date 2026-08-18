package schemas

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONSchemasParse(t *testing.T) {
	for _, path := range []string{
		"copy-generator-input.schema.json",
		"copy-generator-output.schema.json",
		filepath.Join("..", "events", "v1", "events.schema.json"),
	} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var schema any
		if err := json.Unmarshal(contents, &schema); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
}

func TestEventSchemaIncludesAttributionEvents(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "events", "v1", "events.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		OneOf []struct {
			Reference string `json:"$ref"`
		} `json:"oneOf"`
	}
	if err := json.Unmarshal(contents, &schema); err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{
		"#/$defs/notificationSent":        false,
		"#/$defs/notificationDelivered":   false,
		"#/$defs/recommendationImpressed": false,
		"#/$defs/recommendationClicked":   false,
	}
	for _, event := range schema.OneOf {
		if _, ok := want[event.Reference]; ok {
			want[event.Reference] = true
		}
	}
	for reference, found := range want {
		if !found {
			t.Errorf("event schema is missing %s", reference)
		}
	}
}

func TestEventSchemaReferencesResolveAndProtectIdentity(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "events", "v1", "events.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		OneOf []struct {
			Reference string `json:"$ref"`
		} `json:"oneOf"`
		Definitions map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(contents, &schema); err != nil {
		t.Fatal(err)
	}
	for _, event := range schema.OneOf {
		name := strings.TrimPrefix(event.Reference, "#/$defs/")
		if name == event.Reference || schema.Definitions[name] == nil {
			t.Errorf("event schema has unresolved reference %q", event.Reference)
		}
	}

	var envelope struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema.Definitions["envelope"], &envelope); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"event_id", "event_type", "schema_version", "occurred_at", "producer", "trace_id", "payload"} {
		if !contains(envelope.Required, field) {
			t.Errorf("event envelope does not require %q", field)
		}
	}
	if envelope.Properties["user_id_hash"] == nil {
		t.Error("event envelope is missing user_id_hash")
	}
	if envelope.Properties["user_id"] != nil {
		t.Error("event envelope must not expose raw user_id")
	}
}

func TestOpenAPIIncludesDemoFlow(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "openapi", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	contract := string(contents)
	for _, fragment := range []string{
		"/v1/entry-events:",
		"operationId: JourneyService_CreateEntryEvent",
		"/v1/recommendations/latest:",
		"operationId: JourneyService_GetLatestRecommendation",
		"/v1/recommendation-events:",
		"operationId: JourneyService_RecordRecommendationEvent",
		"/v1/redemptions:",
		"operationId: RedemptionService_CreateRedemption",
		"/v1/redemptions/{redemption_id}/verify:",
		"operationId: RedemptionService_VerifyRedemption",
		"/v1/users/me:",
		"operationId: UserService_GetUserProfile",
		"/v1/users/me/preferences:",
		"operationId: UserService_UpdateUserPreferences",
	} {
		if !strings.Contains(contract, fragment) {
			t.Errorf("OpenAPI contract is missing %q", fragment)
		}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
