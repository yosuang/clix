package cmd

import (
	"github.com/spf13/cobra"
	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/protocol"
)

func NewReject(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "reject <run_id>",
		Short: "Reject a pending run",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if f.RunService == nil {
				return protocol.NewError(protocol.InternalError, "run service is not configured")
			}
			run, err := f.RunService.Reject(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if f.Output.JSONSet {
				projected, err := protocol.Project(runPublicMap(run), f.Output.JSONFields)
				if err != nil {
					return err
				}
				return protocol.WriteJSON(f.IO.Out, projected)
			}
			return renderRunText(f, run)
		},
	}
}
