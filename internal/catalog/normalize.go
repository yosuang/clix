package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/yosuang/clix/internal/domain"
)

func normalizeJSONValue(value any) (any, error) {
	switch value := value.(type) {
	case nil, bool, string, json.Number:
		return value, nil
	case float32:
		return json.Number(strconv.FormatFloat(float64(value), 'g', -1, 32)), nil
	case float64:
		return json.Number(strconv.FormatFloat(value, 'g', -1, 64)), nil
	case int:
		return json.Number(strconv.Itoa(value)), nil
	case int8:
		return json.Number(strconv.FormatInt(int64(value), 10)), nil
	case int16:
		return json.Number(strconv.FormatInt(int64(value), 10)), nil
	case int32:
		return json.Number(strconv.FormatInt(int64(value), 10)), nil
	case int64:
		return json.Number(strconv.FormatInt(value, 10)), nil
	case uint:
		return json.Number(strconv.FormatUint(uint64(value), 10)), nil
	case uint8:
		return json.Number(strconv.FormatUint(uint64(value), 10)), nil
	case uint16:
		return json.Number(strconv.FormatUint(uint64(value), 10)), nil
	case uint32:
		return json.Number(strconv.FormatUint(uint64(value), 10)), nil
	case uint64:
		return json.Number(strconv.FormatUint(value, 10)), nil
	case []any:
		out := make([]any, 0, len(value))
		for _, item := range value {
			normalized, err := normalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			out = append(out, normalized)
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, item := range value {
			normalized, err := normalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			out[key] = normalized
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(value))
		for key, item := range value {
			keyText, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("non-string map key")
			}
			normalized, err := normalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			out[keyText] = normalized
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported JSON value %T", value)
	}
}

func fingerprintTool(tool domain.Tool) (string, error) {
	secrets := append([]string(nil), tool.Secrets...)
	sort.Strings(secrets)
	definition := fingerprintDefinition{
		Name:          tool.Name,
		Effect:        tool.Effect,
		InputSchema:   tool.InputSchema,
		OutputSchema:  tool.OutputSchema,
		Adapter:       tool.Adapter,
		AdapterConfig: tool.AdapterConfig,
		Secrets:       secrets,
	}
	payload, err := json.Marshal(definition)
	if err != nil {
		return "", catalogError("could not fingerprint tool %s", tool.Name)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

type fingerprintDefinition struct {
	Name          string         `json:"name"`
	Effect        domain.Effect  `json:"effect"`
	InputSchema   map[string]any `json:"input_schema"`
	OutputSchema  map[string]any `json:"output_schema"`
	Adapter       string         `json:"adapter"`
	AdapterConfig map[string]any `json:"adapter_config"`
	Secrets       []string       `json:"secrets"`
}
