package schemas

import (
	"encoding/json"
	"os"
	"path/filepath"
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
