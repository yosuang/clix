package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

func ParseJSONObject(in io.Reader) (map[string]any, []byte, error) {
	raw, err := io.ReadAll(in)
	if err != nil {
		return nil, nil, NewError(ValidationError, "input could not be read")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil, NewError(ValidationError, "input is required")
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, nil, NewError(ValidationError, "input must be valid JSON")
	}
	if decoder.More() {
		return nil, nil, NewError(ValidationError, "input must contain exactly one JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, nil, NewError(ValidationError, "input must contain exactly one JSON object")
	}

	object, ok := value.(map[string]any)
	if !ok {
		return nil, nil, NewError(ValidationError, "input must be a JSON object")
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, nil, NewError(ValidationError, "input could not be canonicalized")
	}
	return object, canonical, nil
}
