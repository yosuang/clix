package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yosuang/clix/internal/catalog"
	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/protocol"
)

func NewTools(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "Inspect available tools",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newToolsList(f))
	cmd.AddCommand(newToolsGet(f))
	return cmd
}

func newToolsList(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available tools",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := loadCatalog(f)
			if err != nil {
				return err
			}
			if f.Output.JSONSet {
				if _, err := protocol.Project(domain.Tool{}.PublicMap(), f.Output.JSONFields); err != nil {
					return err
				}
				items := make([]map[string]any, 0, len(cat.Tools))
				for _, tool := range cat.Tools {
					items = append(items, tool.PublicMap())
				}
				projected, err := protocol.ProjectList(items, f.Output.JSONFields)
				if err != nil {
					return err
				}
				return protocol.WriteJSON(f.IO.Out, projected)
			}
			for _, tool := range cat.Tools {
				if _, err := fmt.Fprintf(f.IO.Out, "%s %s %s - %s\n", tool.Name, tool.Effect, tool.Adapter, tool.Description); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newToolsGet(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "get <tool_name>",
		Short: "Show a tool",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := loadCatalog(f)
			if err != nil {
				return err
			}
			tool, ok := cat.Get(args[0])
			if !ok {
				return protocol.NewError(protocol.ToolNotFound, fmt.Sprintf("tool %q not found", args[0]))
			}
			if f.Output.JSONSet {
				projected, err := protocol.Project(tool.PublicMap(), f.Output.JSONFields)
				if err != nil {
					return err
				}
				return protocol.WriteJSON(f.IO.Out, projected)
			}
			_, err = fmt.Fprintf(f.IO.Out, "%s %s %s\n%s\n", tool.Name, tool.Effect, tool.Adapter, tool.Description)
			return err
		},
	}
}

func loadCatalog(f *cmdutil.Factory) (catalog.Catalog, error) {
	if f.CatalogLoader == nil {
		return catalog.Catalog{Tools: []domain.Tool{}, ByName: map[string]domain.Tool{}}, nil
	}
	return f.CatalogLoader.Load()
}
