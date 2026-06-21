package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"bgy-ai/internal/converter"
	"bgy-ai/internal/provider"
	"bgy-ai/internal/registry"
)

func executeServerTool(entry *registry.ServerEntry, tool provider.ToolDef) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		flags, err := converter.Convert(tool)
		if err != nil {
			return fmt.Errorf("convert flags: %w", err)
		}

		params := make(map[string]any)
		seen := make(map[string]bool)

		for _, flag := range flags {
			seen[flag.Long] = true
			switch flag.Type {
			case "int":
				val, _ := cmd.Flags().GetInt(flag.Long)
				if val != 0 || flag.Default != "" {
					params[flagKey(flag.Long)] = val
				}
			case "bool":
				val, _ := cmd.Flags().GetBool(flag.Long)
				params[flagKey(flag.Long)] = val
			case "stringSlice":
				val, _ := cmd.Flags().GetStringSlice(flag.Long)
				if len(val) > 0 {
					params[flagKey(flag.Long)] = val
				}
			default:
				val, _ := cmd.Flags().GetString(flag.Long)
				if val != "" {
					params[flagKey(flag.Long)] = val
				}
			}
		}

		result, err := entry.Provider.CallTool(context.Background(), tool.Name, params)
		if err != nil {
			return fmt.Errorf("call %s.%s: %w", entry.Config.Name, tool.Name, err)
		}

		r := getRenderer()
		out, err := r.Render(result)
		if err != nil {
			return fmt.Errorf("render: %w", err)
		}
		fmt.Print(out)
		return nil
	}
}

func flagKey(long string) string {
	return strings.ReplaceAll(long, "-", "_")
}
