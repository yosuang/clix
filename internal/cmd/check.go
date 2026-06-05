package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yosuang/clix/internal/cmdutil"
)

func NewCheck(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check clix configuration and state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(f.IO.Out, "ok")
			return err
		},
	}
}
