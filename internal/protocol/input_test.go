package protocol

import (
	"strings"
	"testing"
)

func TestParseJSONObjectRejectsEmptyInput(t *testing.T) {
	// #given
	input := strings.NewReader("")

	// #when
	_, _, err := ParseJSONObject(input)

	// #then
	if err == nil || err.Error() != "VALIDATION_ERROR: input is required" {
		t.Fatalf("ParseJSONObject() error = %v", err)
	}
}

func TestParseJSONObjectRejectsMultipleValues(t *testing.T) {
	// #given
	input := strings.NewReader(`{"week":"current"} {"extra":true}`)

	// #when
	_, _, err := ParseJSONObject(input)

	// #then
	if err == nil || err.Error() != "VALIDATION_ERROR: input must contain exactly one JSON object" {
		t.Fatalf("ParseJSONObject() error = %v", err)
	}
}

func TestParseJSONObjectRejectsArray(t *testing.T) {
	// #given
	input := strings.NewReader(`[{"week":"current"}]`)

	// #when
	_, _, err := ParseJSONObject(input)

	// #then
	if err == nil || err.Error() != "VALIDATION_ERROR: input must be a JSON object" {
		t.Fatalf("ParseJSONObject() error = %v", err)
	}
}

func TestParseJSONObjectReturnsCanonicalJSON(t *testing.T) {
	// #given
	input := strings.NewReader(`{"b":2,"a":1}`)

	// #when
	value, canonical, err := ParseJSONObject(input)

	// #then
	if err != nil {
		t.Fatalf("ParseJSONObject() error = %v", err)
	}
	if value["a"].(float64) != 1 || value["b"].(float64) != 2 {
		t.Fatalf("value = %#v", value)
	}
	if string(canonical) != `{"a":1,"b":2}` {
		t.Fatalf("canonical = %s", canonical)
	}
}

func TestValidateReservedJQRejectsValue(t *testing.T) {
	// #given
	value := ".id"

	// #when
	err := ValidateReservedJQ(value)

	// #then
	if err == nil || err.Error() != "USAGE_ERROR: --jq is reserved for future use" {
		t.Fatalf("ValidateReservedJQ() error = %v", err)
	}
}
