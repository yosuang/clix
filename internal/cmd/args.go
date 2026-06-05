package cmd

import (
	"github.com/spf13/cobra"
	"github.com/yosuang/clix/internal/protocol"
)

func usageArgs(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validate(cmd, args); err != nil {
			return protocol.NewError(protocol.UsageError, err.Error())
		}
		return nil
	}
}
