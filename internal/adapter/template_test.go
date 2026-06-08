package adapter

import (
	"encoding/json"
	"testing"
)

func TestRenderTemplateUsesInputAndSecrets(t *testing.T) {
	// #given
	input := json.RawMessage(`{"week":"current"}`)
	secrets := map[string]string{"WORK_API_TOKEN": "secret-token"}

	// #when
	got, err := renderTemplate("Bearer ${secrets.WORK_API_TOKEN} ${input.week}", input, secrets)

	// #then
	if err != nil {
		t.Fatalf("renderTemplate() error = %v", err)
	}
	if got != "Bearer secret-token current" {
		t.Fatalf("got = %q", got)
	}
}

func TestRenderTemplateRejectsMissingSecret(t *testing.T) {
	// #given
	input := json.RawMessage(`{"week":"current"}`)

	// #when
	_, err := renderTemplate("${secrets.WORK_API_TOKEN}", input, map[string]string{})

	// #then
	if err == nil || err.Error() != "MISSING_SECRET: secret WORK_API_TOKEN is required" {
		t.Fatalf("renderTemplate() error = %v", err)
	}
}

func TestRenderTemplateRejectsMissingInputField(t *testing.T) {
	// #given
	input := json.RawMessage(`{"week":"current"}`)

	// #when
	_, err := renderTemplate("${input.content}", input, map[string]string{})

	// #then
	if err == nil || err.Error() != "VALIDATION_ERROR: input.content is required" {
		t.Fatalf("renderTemplate() error = %v", err)
	}
}

func TestRenderTemplateRejectsInputWithTrailingGarbage(t *testing.T) {
	// #given
	input := json.RawMessage(`{"week":"current"} nope`)

	// #when
	_, err := renderTemplate("${input.week}", input, map[string]string{})

	// #then
	if err == nil || err.Error() != "VALIDATION_ERROR: input must be valid JSON" {
		t.Fatalf("renderTemplate() error = %v", err)
	}
}

func TestRenderTemplateRejectsUnsupportedExpression(t *testing.T) {
	// #given
	input := json.RawMessage(`{"week":"current"}`)

	// #when
	_, err := renderTemplate("${input.week | upper}", input, map[string]string{})

	// #then
	if err == nil || err.Error() != "VALIDATION_ERROR: unsupported template expression" {
		t.Fatalf("renderTemplate() error = %v", err)
	}
}

func TestRenderTemplateRejectsUnclosedExpression(t *testing.T) {
	// #given
	input := json.RawMessage(`{"week":"current"}`)

	// #when
	_, err := renderTemplate("Bearer ${input.week", input, map[string]string{})

	// #then
	if err == nil || err.Error() != "VALIDATION_ERROR: unsupported template expression" {
		t.Fatalf("renderTemplate() error = %v", err)
	}
}
