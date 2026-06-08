package protocol

import (
	"encoding/json"
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
	if value["a"].(json.Number).String() != "1" || value["b"].(json.Number).String() != "2" {
		t.Fatalf("value = %#v, want json.Number values", value)
	}
	if string(canonical) != `{"a":1,"b":2}` {
		t.Fatalf("canonical = %s", canonical)
	}
}

func TestParseJSONObjectPreservesJSONNumberPrecision(t *testing.T) {
	// #given
	input := strings.NewReader(`{"id":9007199254740993,"price":1.234567890123456789}`)

	// #when
	value, canonical, err := ParseJSONObject(input)

	// #then
	if err != nil {
		t.Fatalf("ParseJSONObject() error = %v", err)
	}
	id, ok := value["id"].(json.Number)
	if !ok {
		t.Fatalf("id = %T, want json.Number", value["id"])
	}
	if id.String() != "9007199254740993" {
		t.Fatalf("id = %q, want precise integer", id.String())
	}
	if string(canonical) != `{"id":9007199254740993,"price":1.234567890123456789}` {
		t.Fatalf("canonical = %s", canonical)
	}
}

func TestValidateReservedJQFlagRejectsWhenPresent(t *testing.T) {
	// #given
	present := true

	// #when
	err := ValidateReservedJQFlag(present)

	// #then
	if err == nil || err.Error() != "USAGE_ERROR: --jq is reserved for future use" {
		t.Fatalf("ValidateReservedJQFlag() error = %v", err)
	}
}

func TestValidateReservedJQFlagAllowsWhenAbsent(t *testing.T) {
	// #given
	present := false

	// #when
	err := ValidateReservedJQFlag(present)

	// #then
	if err != nil {
		t.Fatalf("ValidateReservedJQFlag() error = %v", err)
	}
}
