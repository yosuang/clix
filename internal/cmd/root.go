package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/protocol"
)

type OutputOptions = cmdutil.OutputOptions

const allJSONFieldsFlagValue = "\x00"

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
	root.SetHelpCommand(newHiddenHelpCommand())
	root.PersistentFlags().StringVar(&jsonFields, "json", "", "Output JSON with the specified `fields`")
	root.PersistentFlags().StringVar(&jqExpr, "jq", "", "Filter JSON output using a jq `expression`")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return protocol.NewError(protocol.UsageError, err.Error())
	})
	root.AddCommand(NewApprove(f))
	root.AddCommand(NewCheck(f))
	root.AddCommand(NewReject(f))
	root.AddCommand(NewRun(f))
	root.AddCommand(NewRuns(f))
	root.AddCommand(NewTools(f))
	return root
}

func newHiddenHelpCommand() *cobra.Command {
	return &cobra.Command{
		Use:                "internal-help [command]",
		Aliases:            []string{"help"},
		Hidden:             true,
		DisableSuggestions: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, _, err := cmd.Root().Find(args)
			if target == nil || err != nil {
				return protocol.NewError(protocol.UsageError, fmt.Sprintf("unknown help topic %q", strings.Join(args, " ")))
			}
			target.InitDefaultHelpFlag()
			target.InitDefaultVersionFlag()
			return target.Help()
		},
	}
}

func NormalizeArgs(root *cobra.Command, args []string) []string {
	commandNames := collectCommandNames(root)
	normalized := make([]string, 0, len(args)+1)
	for i, arg := range args {
		normalized = append(normalized, arg)
		if arg == "--json" && jsonFlagOmitsFields(args, i, commandNames) {
			normalized = append(normalized, allJSONFieldsFlagValue)
		}
	}
	return normalized
}

func collectCommandNames(root *cobra.Command) map[string]struct{} {
	names := map[string]struct{}{"help": {}}
	var visit func(*cobra.Command)
	visit = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			if name := child.Name(); name != "" {
				names[name] = struct{}{}
			}
			for _, alias := range child.Aliases {
				names[alias] = struct{}{}
			}
			visit(child)
		}
	}
	visit(root)
	return names
}

func jsonFlagOmitsFields(args []string, index int, commandNames map[string]struct{}) bool {
	if index+1 >= len(args) {
		return true
	}
	next := args[index+1]
	if strings.HasPrefix(next, "-") {
		return true
	}
	_, ok := commandNames[next]
	return ok
}

func parseJSONFields(value string) ([]string, error) {
	if value == "" || value == allJSONFieldsFlagValue {
		return nil, nil
	}
	fields := strings.Split(value, ",")
	for _, field := range fields {
		if field == "" {
			return nil, protocol.NewError(protocol.UsageError, "empty --json field")
		}
	}
	return fields, nil
}
