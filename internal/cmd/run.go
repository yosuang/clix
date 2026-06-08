package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/protocol"
	"github.com/yosuang/clix/internal/runservice"
)

type RunOptions struct {
	InputFlag string
	InputSet  bool
	StdinTTY  bool
}

func NewRun(f *cmdutil.Factory) *cobra.Command {
	opts := RunOptions{}
	cmd := &cobra.Command{
		Use:   "run <tool_name>",
		Short: "Run a tool",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			inputFlag := cmd.Flags().Lookup("input")
			opts.InputSet = inputFlag != nil && inputFlag.Changed
			opts.StdinTTY = f.IO.StdinTTY

			reader, err := opts.InputReader(f.IO.In)
			if err != nil {
				return err
			}
			_, canonical, err := protocol.ParseJSONObject(reader)
			if err != nil {
				return err
			}
			if f.RunService == nil {
				return protocol.NewError(protocol.InternalError, "run service is not configured")
			}
			result, err := f.RunService.Run(cmd.Context(), args[0], json.RawMessage(canonical))
			if err != nil {
				return err
			}
			return renderRunResult(f, result)
		},
	}
	cmd.Flags().StringVar(&opts.InputFlag, "input", "", "JSON object input")
	return cmd
}

func (o RunOptions) InputReader(stdin io.Reader) (io.Reader, error) {
	if o.InputSet {
		if !o.StdinTTY {
			piped, err := io.ReadAll(stdin)
			if err != nil {
				return nil, protocol.NewError(protocol.ValidationError, "stdin could not be read")
			}
			if len(bytes.TrimSpace(piped)) > 0 {
				return nil, protocol.NewError(protocol.UsageError, "--input cannot be combined with non-empty stdin")
			}
		}
		return strings.NewReader(o.InputFlag), nil
	}
	if o.StdinTTY {
		return nil, protocol.NewError(protocol.ValidationError, "input is required")
	}
	return stdin, nil
}

func renderRunResult(f *cmdutil.Factory, result runservice.Result) error {
	if f.Output.JSONSet {
		projected, err := protocol.Project(runResultPublicMap(result), f.Output.JSONFields)
		if err != nil {
			return err
		}
		return protocol.WriteJSON(f.IO.Out, projected)
	}
	return renderRunText(f, result.Run)
}

func renderRunText(f *cmdutil.Factory, run domain.Run) error {
	_, err := fmt.Fprintf(f.IO.Out, "%s %s %s\n", run.ID, run.Status, run.ToolName)
	return err
}

func runResultPublicMap(result runservice.Result) map[string]any {
	fields := runPublicMap(result.Run)
	fields["output"] = result.Output
	return fields
}

func runPublicMap(run domain.Run) map[string]any {
	fields := run.PublicMap()
	if _, ok := fields["error_code"]; !ok {
		fields["error_code"] = nil
	}
	if _, ok := fields["error_message"]; !ok {
		fields["error_message"] = nil
	}
	return fields
}
