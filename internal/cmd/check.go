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
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			if f.CatalogLoader != nil {
				if _, err := f.CatalogLoader.Load(); err != nil {
					return err
				}
			}
			if f.RunStore != nil {
				if _, err := f.RunStore.ListRuns(cmd.Context(), nil); err != nil {
					return err
				}
			}
			_, err := fmt.Fprintln(f.IO.Out, "ok")
			return err
		},
	}
}
