package protocol

import (
	"encoding/json"
	"testing"
)

func TestProjectObjectFields(t *testing.T) {
	// #given
	source := map[string]any{"id": "run_1", "status": "succeeded", "input": map[string]any{"week": "current"}}

	// #when
	got, err := Project(source, []string{"id", "status"})

	// #then
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	encoded, _ := json.Marshal(got)
	if string(encoded) != `{"id":"run_1","status":"succeeded"}` {
		t.Fatalf("projected = %s", encoded)
	}
}

func TestProjectListFields(t *testing.T) {
	// #given
	source := []map[string]any{
		{"name": "weekly.get_records", "effect": "read", "adapter": "http"},
		{"name": "weekly.submit_report", "effect": "write", "adapter": "http"},
	}

	// #when
	got, err := ProjectList(source, []string{"name", "effect"})

	// #then
	if err != nil {
		t.Fatalf("ProjectList() error = %v", err)
	}
	encoded, _ := json.Marshal(got)
	want := `[{"effect":"read","name":"weekly.get_records"},{"effect":"write","name":"weekly.submit_report"}]`
	if string(encoded) != want {
		t.Fatalf("projected = %s, want %s", encoded, want)
	}
}

func TestUnknownProjectedFieldFails(t *testing.T) {
	// #given
	source := map[string]any{"id": "run_1"}

	// #when
	_, err := Project(source, []string{"missing"})

	// #then
	if err == nil || err.Error() != "USAGE_ERROR: unknown field \"missing\"" {
		t.Fatalf("Project() error = %v", err)
	}
}
