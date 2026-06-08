package cmd

import (
	"github.com/spf13/cobra"
	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/protocol"
)

func NewApprove(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "approve <run_id>",
		Short: "Approve a pending run",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if f.RunService == nil {
				return protocol.NewError(protocol.InternalError, "run service is not configured")
			}
			result, err := f.RunService.Approve(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderRunResult(f, result)
		},
	}
}
