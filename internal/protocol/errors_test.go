package protocol

import (
	"bytes"
	"errors"
	"testing"
)

func TestProtocolErrorText(t *testing.T) {
	// #given
	err := NewError(ValidationError, "input.week is required")

	// #when
	got := err.Error()

	// #then
	want := "VALIDATION_ERROR: input.week is required"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestProtocolErrorExitCodes(t *testing.T) {
	// #given
	cases := map[Code]int{
		UsageError:      2,
		ValidationError: 1,
		InternalError:   1,
	}

	for code, want := range cases {
		// #when
		got := ExitCode(NewError(code, "x"))

		// #then
		if got != want {
			t.Fatalf("%s exit code = %d, want %d", code, got, want)
		}
	}
}

func TestWriteJSONErrorRendersProtocolEnvelope(t *testing.T) {
	// #given
	var stderr bytes.Buffer
	err := NewError(UsageError, "bad flag")

	// #when
	writeErr := WriteJSONError(&stderr, err)

	// #then
	if writeErr != nil {
		t.Fatalf("WriteJSONError() error = %v", writeErr)
	}
	want := "{\"code\":\"USAGE_ERROR\",\"message\":\"bad flag\",\"ok\":false}\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestWriteTextErrorRendersProtocolError(t *testing.T) {
	// #given
	var stderr bytes.Buffer
	err := errors.New("boom")

	// #when
	writeErr := WriteTextError(&stderr, err)

	// #then
	if writeErr != nil {
		t.Fatalf("WriteTextError() error = %v", writeErr)
	}
	want := "INTERNAL_ERROR: boom\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", stderr.String(), want)
	}
}
