package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/golango-cn/dbm-cli/internal/mcp"
)

// newMcpCmd 启动 MCP（Model Context Protocol）server，通过 stdio 与
// AI 客户端（Claude Desktop / Cursor 等）通信。
//
// 本命令不影响其它 CLI 子命令：它只是 mcp.Run 的薄封装——
// 复用与其它命令相同的配置加载（-c/--config 全局 flag），把已加载的
// *config.File 交给 internal/mcp，由后者负责连接管理与 tool 注册。
//
// 用法：
//   dbm-cli mcp                      # 以 stdio 模式启动，供 MCP 客户端接入
func newMcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "启动 MCP server（stdio），供 AI 客户端调用数据库工具",
		Long: `启动 dbm-cli 的 MCP（Model Context Protocol）server，通过 stdio 与 AI 客户端通信。

AI 客户端（如 Claude Desktop、Cursor）可把本命令配置为 MCP server，从而直接调用
dbm-cli 的数据库能力（查询元数据、采样数据、执行 SQL），无需手动拼命令行。

安全：所有连接沿用配置中的 allow_write 开关——只读数据源在 MCP 层同样拒绝写操作；
危险语句（DROP/TRUNCATE、无 WHERE 的 DELETE/UPDATE）需在 execute tool 显式传
confirm_destructive=true 才会执行。

配置：使用与其它子命令相同的 YAML（--config / 默认查找路径）。

示例（Claude Desktop 配置片段）：
  {
    "mcpServers": {
      "dbm-cli": {
        "command": "/path/to/dbm-cli",
        "args": ["mcp"]
      }
    }
  }`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				// MCP 场景下配置缺失不应硬失败——list_datasources 等 tool 仍可工作，
				// 但大多数 tool 需要数据源。这里给出明确提示让用户先建配置。
				// 仍继续以空配置启动，便于 AI 至少能 discover 到「无配置」状态。
				cfg = nil
				fmt.Fprintf(os.Stderr, "[dbm-cli] warning: %v (MCP server will start but most tools will reject calls until a config exists)\n", err)
			}

			// 捕获中断信号，优雅退出（关闭所有缓存的数据库连接）。
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			return mcp.Run(ctx, cfg)
		},
	}
	return cmd
}
