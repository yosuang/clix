package cmd

import (
	"io"
	"strings"
	"testing"
)

func TestInputSourceUsesInputFlagOverTTYStdin(t *testing.T) {
	// #given
	opts := RunOptions{InputFlag: `{"week":"current"}`, StdinTTY: true}

	// #when
	got, err := opts.InputReader(strings.NewReader(""))

	// #then
	if err != nil {
		t.Fatalf("InputReader() error = %v", err)
	}
	body, _ := io.ReadAll(got)
	if string(body) != `{"week":"current"}` {
		t.Fatalf("body = %q", body)
	}
}

func TestInputSourceRejectsInputFlagWithNonEmptyPipe(t *testing.T) {
	// #given
	opts := RunOptions{InputFlag: `{"week":"current"}`, StdinTTY: false}

	// #when
	_, err := opts.InputReader(strings.NewReader(`{"other":true}`))

	// #then
	if err == nil || err.Error() != "USAGE_ERROR: --input cannot be combined with non-empty stdin" {
		t.Fatalf("InputReader() error = %v", err)
	}
}

func TestInputSourceRejectsMissingTTYInput(t *testing.T) {
	// #given
	opts := RunOptions{InputFlag: "", StdinTTY: true}

	// #when
	_, err := opts.InputReader(strings.NewReader(""))

	// #then
	if err == nil || err.Error() != "VALIDATION_ERROR: input is required" {
		t.Fatalf("InputReader() error = %v", err)
	}
}
