package protocol

import "fmt"

func Project(source map[string]any, fields []string) (map[string]any, error) {
	out := make(map[string]any, len(fields))
	for _, field := range fields {
		value, ok := source[field]
		if !ok {
			return nil, NewError(UsageError, fmt.Sprintf("unknown field %q", field))
		}
		out[field] = value
	}
	return out, nil
}

func ProjectList(source []map[string]any, fields []string) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(source))
	for _, item := range source {
		projected, err := Project(item, fields)
		if err != nil {
			return nil, err
		}
		out = append(out, projected)
	}
	return out, nil
}
