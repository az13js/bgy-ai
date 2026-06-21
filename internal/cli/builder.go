package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"bgy-ai/internal/converter"
	"bgy-ai/internal/provider"
	"bgy-ai/internal/registry"
)

func BuildCommands(root *cobra.Command, reg *registry.Registry) error {
	servers := reg.Servers()
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Config.Name < servers[j].Config.Name
	})

	for _, entry := range servers {
		serverCmd := buildServerCommand(entry)
		root.AddCommand(serverCmd)
	}

	root.AddCommand(newListCmd(reg))
	root.AddCommand(newReloadCmd(reg))

	return nil
}

func buildServerCommand(entry *registry.ServerEntry) *cobra.Command {
	cmd := &cobra.Command{
		Use:   entry.Config.Name,
		Short: entry.Config.Description,
		Long:  entry.Config.Description,
	}

	flagsMap, err := converter.ConvertMany(entry.Tools)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[warn] convert %s tools: %v\n", entry.Config.Name, err)
	}

	for _, tool := range entry.Tools {
		toolCmd := buildToolCommand(entry, tool, flagsMap[tool.Name])
		cmd.AddCommand(toolCmd)
	}

	return cmd
}

func buildToolCommand(entry *registry.ServerEntry, tool provider.ToolDef, flags []converter.Flag) *cobra.Command {
	cmdName := strings.ReplaceAll(tool.Name, "_", "-")
	cmd := &cobra.Command{
		Use:   cmdName,
		Short: tool.Description,
		Long:  tool.Description,
		Annotations: map[string]string{
			"toolName":   tool.Name,
			"serverName": entry.Config.Name,
		},
		RunE: executeServerTool(entry, tool),
	}

	for _, flag := range flags {
		switch flag.Type {
		case "int":
			d := 0
			if flag.Default != "" {
				fmt.Sscanf(flag.Default, "%d", &d)
			}
			cmd.Flags().Int(flag.Long, d, flag.Description)
		case "bool":
			d := flag.Default == "true"
			cmd.Flags().Bool(flag.Long, d, flag.Description)
		case "stringSlice":
			cmd.Flags().StringSlice(flag.Long, nil, flag.Description)
		default:
			cmd.Flags().String(flag.Long, flag.Default, flag.Description)
		}
		if flag.Required {
			cmd.MarkFlagRequired(flag.Long)
		}
	}

	return cmd
}

func newListCmd(reg *registry.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出所有可用 MCP/HTTP 服务及工具",
		RunE: func(cmd *cobra.Command, args []string) error {
			servers := reg.Servers()
			sort.Slice(servers, func(i, j int) bool {
				return servers[i].Config.Name < servers[j].Config.Name
			})

			if len(servers) == 0 {
				fmt.Println("(无已注册服务)")
				return nil
			}

			for _, s := range servers {
				fmt.Printf("%s  (%d tools)\n", s.Config.Name, len(s.Tools))
				for _, t := range s.Tools {
					fmt.Printf("  %-25s %s\n", t.Name, t.Description)
				}
				fmt.Println()
			}
			return nil
		},
	}
}

func newReloadCmd(reg *registry.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "重新加载所有 MCP/HTTP 服务配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := reg.Reload(); err != nil {
				return err
			}
			fmt.Println("配置已重新加载")
			return nil
		},
	}
}
