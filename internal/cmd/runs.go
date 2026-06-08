package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/protocol"
)

func NewRuns(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "Inspect tool runs",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newRunsList(f))
	cmd.AddCommand(newRunsGet(f))
	return cmd
}

func newRunsList(f *cmdutil.Factory) *cobra.Command {
	var statusText string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List runs",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			if f.RunStore == nil {
				return protocol.NewError(protocol.InternalError, "run store is not configured")
			}
			var statusFilter *domain.Status
			statusFlag := cmd.Flags().Lookup("status")
			if statusFlag != nil && statusFlag.Changed {
				status, err := parseRunStatus(statusText)
				if err != nil {
					return err
				}
				statusFilter = &status
			}
			runs, err := f.RunStore.ListRuns(cmd.Context(), statusFilter)
			if err != nil {
				return err
			}
			if f.Output.JSONSet {
				if _, err := protocol.Project(storedRunPublicMap(domain.Run{}), f.Output.JSONFields); err != nil {
					return err
				}
				items := make([]map[string]any, 0, len(runs))
				for _, run := range runs {
					items = append(items, storedRunPublicMap(run))
				}
				projected, err := protocol.ProjectList(items, f.Output.JSONFields)
				if err != nil {
					return err
				}
				return protocol.WriteJSON(f.IO.Out, projected)
			}
			for _, run := range runs {
				if err := renderRunText(f, run); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&statusText, "status", "", "filter by run status")
	return cmd
}

func newRunsGet(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "get <run_id>",
		Short: "Show a run",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if f.RunStore == nil {
				return protocol.NewError(protocol.InternalError, "run store is not configured")
			}
			run, err := f.RunStore.GetRun(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if f.Output.JSONSet {
				projected, err := protocol.Project(storedRunPublicMap(run), f.Output.JSONFields)
				if err != nil {
					return err
				}
				return protocol.WriteJSON(f.IO.Out, projected)
			}
			return renderRunText(f, run)
		},
	}
}

func parseRunStatus(value string) (domain.Status, error) {
	status := domain.Status(value)
	switch status {
	case domain.StatusCreated,
		domain.StatusPendingApproval,
		domain.StatusRunning,
		domain.StatusSucceeded,
		domain.StatusFailed,
		domain.StatusRejected:
		return status, nil
	default:
		return "", protocol.NewError(protocol.UsageError, fmt.Sprintf("unknown status %q", value))
	}
}

func storedRunPublicMap(run domain.Run) map[string]any {
	fields := runPublicMap(run)
	fields["approved_at"] = optionalRunTime(run.ApprovedAt)
	fields["started_at"] = optionalRunTime(run.StartedAt)
	fields["finished_at"] = optionalRunTime(run.FinishedAt)
	fields["exit_code"] = optionalRunInt(run.ExitCode)
	if len(run.InputJSON) > 0 {
		fields["input"] = json.RawMessage(run.InputJSON)
	} else {
		fields["input"] = nil
	}
	return fields
}

func optionalRunTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.Format(time.RFC3339Nano)
}

func optionalRunInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
