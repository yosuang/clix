package runservice

import (
	"strings"
	"testing"
)

func TestNewRunIDHasRunPrefix(t *testing.T) {
	// #given
	generator := NewIDGenerator()

	// #when
	id, err := generator.NewRunID()

	// #then
	if err != nil {
		t.Fatalf("NewRunID() error = %v", err)
	}
	if !strings.HasPrefix(id, "run_") {
		t.Fatalf("id = %q", id)
	}
}
