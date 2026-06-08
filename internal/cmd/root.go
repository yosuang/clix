package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/protocol"
)

type OutputOptions = cmdutil.OutputOptions

func NewRoot(f *cmdutil.Factory) *cobra.Command {
	// Root output flags must run before child persistent hooks on future commands.
	cobra.EnableTraverseRunHooks = true

	var jsonFields string
	var jqExpr string

	var root *cobra.Command
	root = &cobra.Command{
		Use:                "clix",
		SilenceUsage:       true,
		SilenceErrors:      true,
		DisableSuggestions: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return protocol.NewError(protocol.UsageError, fmt.Sprintf("unknown command %q", args[0]))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			jsonFlag := root.PersistentFlags().Lookup("json")
			jqFlag := root.PersistentFlags().Lookup("jq")
			f.Output = OutputOptions{
				JSONSet: jsonFlag != nil && jsonFlag.Changed,
				JQ:      jqExpr,
				JQSet:   jqFlag != nil && jqFlag.Changed,
			}
			if f.Output.JSONSet {
				fields, err := parseJSONFields(jsonFields)
				if err != nil {
					return err
				}
				f.Output.JSONFields = fields
			}
			return protocol.ValidateReservedJQFlag(f.Output.JQSet)
		},
	}
	root.SetIn(f.IO.In)
	root.SetOut(f.IO.Out)
	root.SetErr(f.IO.ErrOut)
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().StringVar(&jsonFields, "json", "", "select top-level JSON fields")
	root.PersistentFlags().StringVar(&jqExpr, "jq", "", "reserved for future use")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return protocol.NewError(protocol.UsageError, err.Error())
	})
	root.AddCommand(NewCheck(f))
	root.AddCommand(NewTools(f))
	return root
}

func parseJSONFields(value string) ([]string, error) {
	fields := strings.Split(value, ",")
	for _, field := range fields {
		if field == "" {
			return nil, protocol.NewError(protocol.UsageError, "empty --json field")
		}
	}
	return fields, nil
}
