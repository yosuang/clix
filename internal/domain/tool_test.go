package domain

import "testing"

func TestToolValidateInputWithoutValidatorFails(t *testing.T) {
	// #given
	tool := Tool{Name: "weekly.get_records"}

	// #when
	err := tool.ValidateInput(map[string]any{"week": "current"})

	// #then
	if err == nil {
		t.Fatal("ValidateInput() error = nil, want missing validator error")
	}
}
