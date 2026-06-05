package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/protocol"
)

func NewRoot(f *cmdutil.Factory) *cobra.Command {
	root := &cobra.Command{
		Use:           "clix",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return protocol.NewError(protocol.UsageError, fmt.Sprintf("unknown command %q", args[0]))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(f.IO.Out)
	root.SetErr(f.IO.ErrOut)
	root.CompletionOptions.DisableDefaultCmd = true
	root.SuggestionsMinimumDistance = -1
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return protocol.NewError(protocol.UsageError, err.Error())
	})
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_, _ = fmt.Fprintln(f.IO.Out, cmd.UsageString())
	})
	root.SetUsageFunc(func(cmd *cobra.Command) error {
		_, _ = fmt.Fprintln(f.IO.ErrOut, cmd.UsageString())
		return nil
	})
	root.AddCommand(NewCheck(f))
	return root
}
