package clix

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func Main() int {
	return Run(os.Args[1:], os.Stdout, os.Stderr)
}

func Run(args []string, stdout, stderr io.Writer) int {
	var result any
	var outputOptions OutputOptions

	root := newRootCommand(&outputOptions, &result)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	if err := root.ExecuteContext(context.Background()); err != nil {
		if !outputOptions.parsed {
			outputOptions = bestEffortOutputOptions(root, outputOptions)
		}
		return writeFailure(stdout, stderr, toAppError(err), outputOptions)
	}
	if appErr := writeSuccess(stdout, result, outputOptions); appErr != nil {
		return writeFailure(stdout, stderr, appErr, outputOptions)
	}
	return 0
}

func newRootCommand(outputOptions *OutputOptions, result *any) *cobra.Command {
	root := &cobra.Command{
		Use:           "clix",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			options, appErr := outputOptionsFromFlags(cmd.Flags())
			if appErr != nil {
				return appErr
			}
			*outputOptions = options
			outputOptions.parsed = true
			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			return newError(CodeUsageError, "command is required")
		},
	}
	root.PersistentFlags().String("json", "", "comma-separated top-level fields to return as JSON")
	root.PersistentFlags().String("jq", "", "jq expression to filter the JSON result")

	root.AddCommand(newCheckCommand(result))
	root.AddCommand(newToolsCommand(result))
	root.AddCommand(newRunCommand(result))
	root.AddCommand(newApproveCommand(result))
	root.AddCommand(newRejectCommand(result))
	root.AddCommand(newRunsCommand(result))
	return root
}

func newCheckCommand(result *any) *cobra.Command {
	return &cobra.Command{
		Use:  "check",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			service, manifestPath, databasePath, appErr := newDefaultService(true)
			if appErr != nil {
				return appErr
			}
			defer service.Close()
			*result = service.check(manifestPath, databasePath)
			return nil
		},
	}
}

func newToolsCommand(result *any) *cobra.Command {
	tools := &cobra.Command{
		Use:  "tools",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return newError(CodeUsageError, "usage: clix tools <list|get>")
		},
	}
	tools.AddCommand(&cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			service, _, _, appErr := newDefaultService(true)
			if appErr != nil {
				return appErr
			}
			defer service.Close()
			*result = service.listTools()
			return nil
		},
	})
	tools.AddCommand(&cobra.Command{
		Use:  "get <tool_name>",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			service, _, _, appErr := newDefaultService(true)
			if appErr != nil {
				return appErr
			}
			defer service.Close()
			tool, appErr := service.getTool(args[0])
			if appErr != nil {
				return appErr
			}
			*result = tool
			return nil
		},
	})
	return tools
}

func newRunCommand(result *any) *cobra.Command {
	var rawInput string
	run := &cobra.Command{
		Use:  "run <tool_name>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("input") || rawInput == "" {
				return newError(CodeUsageError, "usage: clix run <tool_name> --input '<json>'")
			}
			service, _, _, appErr := newDefaultService(true)
			if appErr != nil {
				return appErr
			}
			defer service.Close()
			runResult, appErr := service.runTool(cmd.Context(), args[0], rawInput)
			if appErr != nil {
				return appErr
			}
			*result = runResult
			return nil
		},
	}
	run.Flags().StringVar(&rawInput, "input", "", "JSON object input for the tool")
	return run
}

func newApproveCommand(result *any) *cobra.Command {
	return &cobra.Command{
		Use:  "approve <run_id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			service, _, _, appErr := newDefaultService(true)
			if appErr != nil {
				return appErr
			}
			defer service.Close()
			approved, appErr := service.approveRun(cmd.Context(), args[0])
			if appErr != nil {
				return appErr
			}
			*result = approved
			return nil
		},
	}
}

func newRejectCommand(result *any) *cobra.Command {
	return &cobra.Command{
		Use:  "reject <run_id>",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			service, _, _, appErr := newDefaultService(false)
			if appErr != nil {
				return appErr
			}
			defer service.Close()
			rejected, appErr := service.rejectRun(args[0])
			if appErr != nil {
				return appErr
			}
			*result = rejected
			return nil
		},
	}
}

func newRunsCommand(result *any) *cobra.Command {
	runs := &cobra.Command{
		Use:  "runs",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return newError(CodeUsageError, "usage: clix runs <list|get>")
		},
	}

	var status string
	list := &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			service, _, _, appErr := newDefaultService(false)
			if appErr != nil {
				return appErr
			}
			defer service.Close()
			records, appErr := service.listRuns(status)
			if appErr != nil {
				return appErr
			}
			*result = records
			return nil
		},
	}
	list.Flags().StringVar(&status, "status", "", "filter runs by status")

	runs.AddCommand(list)
	runs.AddCommand(&cobra.Command{
		Use:  "get <run_id>",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			service, _, _, appErr := newDefaultService(false)
			if appErr != nil {
				return appErr
			}
			defer service.Close()
			record, appErr := service.getRun(args[0])
			if appErr != nil {
				return appErr
			}
			*result = record
			return nil
		},
	})
	return runs
}

func toAppError(err error) *AppError {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return newError(CodeUsageError, err.Error())
}
