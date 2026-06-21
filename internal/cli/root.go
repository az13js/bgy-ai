package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"bgy-ai/internal/provider"
	"bgy-ai/internal/renderer"
	"bgy-ai/internal/registry"
)

var (
	outputFormat string
	configDir    string
	mcpDir       string
	verbose      bool
	reg          *registry.Registry
)

func Execute() {
	reg = registry.New()

	home, _ := os.UserHomeDir()
	mcpDir = home + "/.bgy-ai/tools.d"
	provider.SetSessionDir(home + "/.bgy-ai/sessions")

	rootCmd := newRootCmd()
	// Parse flags early so --verbose takes effect before Load
	rootCmd.ParseFlags(os.Args[1:])
	registry.SetVerbose(verbose)

	if err := reg.Load(mcpDir); err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
	}

	if err := BuildCommands(rootCmd, reg); err != nil {
		fmt.Fprintf(os.Stderr, "build commands: %v\n", err)
		os.Exit(1)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bgy-ai",
		Short: "AI 工具统一命令行",
		Long: `bgy-ai — 统一发现和调用 MCP/HTTP 服务。

所有注册的服务自动生成子命令:
  bgy-ai wiki search-knowledge --query ...     # 调用 Wiki MCP
  bgy-ai jenkins build-status --project ...     # 调用 Jenkins MCP
  bgy-ai weather get-forecast --city Beijing    # 调用 HTTP API

配置文件放在 ~/.bgy-ai/tools.d/*.yaml`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		SilenceUsage: true,
	}

	home, _ := os.UserHomeDir()
	defaultMCPDir := home + "/.bgy-ai/tools.d"

	cmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "输出格式 (text|json)")
	cmd.PersistentFlags().StringVar(&mcpDir, "mcp-dir", defaultMCPDir, "MCP/HTTP 配置文件目录")
	cmd.PersistentFlags().StringVar(&configDir, "config", home+"/.bgy-ai", "配置根目录")
	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "输出详细日志")

	return cmd
}

func getRenderer() renderer.Renderer {
	return renderer.New(renderer.Format(outputFormat))
}
