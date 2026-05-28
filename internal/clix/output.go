package clix

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type OutputOptions struct {
	JSONFields  []string
	JSONPresent bool
	JQ          string
	parsed      bool
}

func (o OutputOptions) jsonMode() bool {
	return o.JSONPresent || o.JQ != ""
}

func writeSuccess(stdout io.Writer, value any, options OutputOptions) *AppError {
	if !options.jsonMode() {
		return writeText(stdout, value)
	}
	projected, appErr := projectValue(value, options.JSONFields)
	if appErr != nil {
		return appErr
	}
	if options.JQ != "" {
		filtered, appErr := applyJQ(projected, options.JQ)
		if appErr != nil {
			return appErr
		}
		projected = filtered
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(projected); err != nil {
		return errorf(CodeInternalError, "write JSON output: %v", err)
	}
	return nil
}

func writeFailure(stdout, stderr io.Writer, err *AppError, options OutputOptions) int {
	if options.jsonMode() {
		_ = json.NewEncoder(stdout).Encode(map[string]any{
			"ok":      false,
			"code":    err.Code,
			"message": err.Message,
		})
		return 1
	}
	fmt.Fprintf(stderr, "%s: %s\n", err.Code, err.Message)
	return 1
}

func projectValue(value any, fields []string) (any, *AppError) {
	if len(fields) == 0 {
		return value, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, errorf(CodeInternalError, "project fields: %v", err)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, errorf(CodeInternalError, "project fields: %v", err)
	}
	switch typed := decoded.(type) {
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, newError(CodeUsageError, "--json projection requires object results")
			}
			result = append(result, projectObject(object, fields))
		}
		return result, nil
	case map[string]any:
		return projectObject(typed, fields), nil
	default:
		return nil, newError(CodeUsageError, "--json projection requires object results")
	}
}

func projectObject(object map[string]any, fields []string) map[string]any {
	result := make(map[string]any, len(fields))
	for _, field := range fields {
		if value, ok := object[field]; ok {
			result[field] = value
		}
	}
	return result
}

func applyJQ(value any, expression string) (any, *AppError) {
	jsonValue, appErr := jsonValue(value)
	if appErr != nil {
		return nil, appErr
	}
	query, err := gojq.Parse(expression)
	if err != nil {
		return nil, errorf(CodeJQError, "parse jq expression: %v", err)
	}
	iter := query.Run(jsonValue)
	var results []any
	for {
		item, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := item.(error); ok {
			return nil, errorf(CodeJQError, "run jq expression: %v", err)
		}
		results = append(results, item)
	}
	switch len(results) {
	case 0:
		return []any{}, nil
	case 1:
		return results[0], nil
	default:
		return results, nil
	}
}

func jsonValue(value any) (any, *AppError) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, errorf(CodeInternalError, "prepare JSON value: %v", err)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, errorf(CodeInternalError, "prepare JSON value: %v", err)
	}
	return decoded, nil
}

func writeText(stdout io.Writer, value any) *AppError {
	switch typed := value.(type) {
	case CheckResult:
		fmt.Fprintf(stdout, "manifest ok: %d tools\n", typed.ToolCount)
	case []ToolSummary:
		for _, item := range typed {
			fmt.Fprintf(stdout, "%s %s %s - %s\n", item.Name, item.Effect, item.Adapter, item.Description)
		}
	case ToolDetail:
		fmt.Fprintf(stdout, "%s\n%s\nadapter: %s\neffect: %s\n", typed.Name, typed.Description, typed.Adapter, typed.Effect)
	case RunResult:
		fmt.Fprintf(stdout, "%s %s", typed.ID, typed.Status)
		if typed.ToolName != "" {
			fmt.Fprintf(stdout, " %s", typed.ToolName)
		}
		fmt.Fprintln(stdout)
		if typed.Output != nil {
			writeCompactJSONLine(stdout, typed.Output)
		}
	case RunRecord:
		fmt.Fprintf(stdout, "%s %s %s\n", typed.ID, typed.Status, typed.ToolName)
	case []RunRecord:
		for _, item := range typed {
			fmt.Fprintf(stdout, "%s %s %s\n", item.ID, item.Status, item.ToolName)
		}
	default:
		writeCompactJSONLine(stdout, typed)
	}
	return nil
}

func writeCompactJSONLine(stdout io.Writer, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		fmt.Fprintln(stdout, "<unprintable>")
		return
	}
	fmt.Fprintln(stdout, string(data))
}

func outputOptionsFromFlags(flags *pflag.FlagSet) (OutputOptions, *AppError) {
	var options OutputOptions
	jsonValue, err := flags.GetString("json")
	if err != nil {
		return options, errorf(CodeUsageError, "read --json flag: %v", err)
	}
	if flags.Changed("json") {
		options.JSONPresent = true
		for _, field := range strings.Split(jsonValue, ",") {
			field = strings.TrimSpace(field)
			if field != "" {
				options.JSONFields = append(options.JSONFields, field)
			}
		}
		if len(options.JSONFields) == 0 {
			return options, newError(CodeUsageError, "--json requires a comma-separated field list")
		}
	}

	jqValue, err := flags.GetString("jq")
	if err != nil {
		return options, errorf(CodeUsageError, "read --jq flag: %v", err)
	}
	if flags.Changed("jq") {
		if strings.TrimSpace(jqValue) == "" {
			return options, newError(CodeUsageError, "--jq requires an expression")
		}
		options.JQ = jqValue
	}
	return options, nil
}

func bestEffortOutputOptions(cmd *cobra.Command, fallback OutputOptions) OutputOptions {
	if fallback.jsonMode() {
		return fallback
	}
	if cmd == nil {
		return fallback
	}
	if options, appErr := outputOptionsFromFlags(cmd.Flags()); appErr == nil && options.jsonMode() {
		return options
	}
	if root := cmd.Root(); root != nil {
		if options, appErr := outputOptionsFromFlags(root.PersistentFlags()); appErr == nil && options.jsonMode() {
			return options
		}
	}
	return fallback
}
