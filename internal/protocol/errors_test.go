package protocol

import "testing"

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
