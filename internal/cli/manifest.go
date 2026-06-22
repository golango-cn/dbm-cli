package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/golango-cn/dbm-cli/internal/manifest"
)

// newManifestCmd 输出自描述 JSON 清单（面向 AI 调用）。
// 这是项目的「AI 友好」关键命令：AI 只需读取本命令输出即可学会如何调用 dbm。
func newManifestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "manifest",
		Short: "输出工具自描述清单（JSON，供 AI 调用）",
		Long: `输出结构化 JSON，描述 dbm 的命令、参数、支持的驱动与示例。
该命令不连接任何数据库，可随时安全调用。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := loadConfig() // 未配置也不算错误，manifest 仍可输出
			m := manifest.Build(cfg, commandInfos())
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			enc.SetEscapeHTML(false)
			return enc.Encode(m)
		},
	}
}

// commandInfos 收集各命令的自描述，注入 manifest。
// 保持与 root.go 注册的命令一致。
func commandInfos() []manifest.CommandInfo {
	return []manifest.CommandInfo{
		{Name: "config", Summary: "查看当前配置概览（数据源清单+默认数据源，脱敏，支持 -o json）", Usage: "dbm-cli config [-o json]",
			Examples: []string{"dbm-cli config", "dbm-cli config -o json"}},
		{Name: "datasources", Summary: "列出已配置数据源", Usage: "dbm-cli datasources",
			Examples: []string{"dbm-cli datasources"}},
		{Name: "version", Summary: "查询目标数据库版本", Usage: "dbm-cli version -d <ds>",
			Flags:    []manifest.Flag{{Name: "datasource", Shorthand: "d", Usage: "数据源名"}},
			Examples: []string{"dbm-cli version -d prod-ro"}},
		{Name: "databases", Summary: "列出库/PDB", Usage: "dbm-cli databases -d <ds>",
			Examples: []string{"dbm-cli databases -d prod-ro"}},
		{Name: "schemas", Summary: "列出 schema", Usage: "dbm-cli schemas -d <ds> [--like PATTERN]",
			Flags:    []manifest.Flag{{Name: "like", Usage: "名称模糊匹配"}},
			Examples: []string{"dbm-cli schemas -d prod-ro --like HR%"}},
		{Name: "tables", Summary: "列出表", Usage: "dbm-cli tables -d <ds> [--schema S] [--like P]",
			Flags: []manifest.Flag{
				{Name: "schema", Usage: "schema 名，默认当前用户"},
				{Name: "like", Usage: "表名模糊匹配"},
			},
			Examples: []string{"dbm-cli tables -d prod-ro --schema HR"}},
		{Name: "columns", Summary: "查看表结构", Usage: "dbm-cli columns -d <ds> --table T [--schema S]",
			Flags: []manifest.Flag{
				{Name: "table", Usage: "表名（必填）"},
				{Name: "schema", Usage: "schema 名"},
			},
			Examples: []string{"dbm-cli columns -d prod-ro --table EMPLOYEES --schema HR"}},
		{Name: "indexes", Summary: "查看表索引", Usage: "dbm-cli indexes -d <ds> --table T [--schema S]",
			Flags: []manifest.Flag{
				{Name: "table", Usage: "表名（必填）"},
				{Name: "schema", Usage: "schema 名"},
			},
			Examples: []string{"dbm-cli indexes -d prod-ro --table EMPLOYEES"}},
		{Name: "views", Summary: "列出视图", Usage: "dbm-cli views -d <ds> [--schema S]",
			Flags:    []manifest.Flag{{Name: "schema", Usage: "schema 名"}},
			Examples: []string{"dbm-cli views -d prod-ro --schema HR"}},
		{Name: "table", Summary: "分页查表数据", Usage: "dbm-cli table -d <ds> --name T [--schema S] [--limit N] [--offset N] [--order COL]",
			Flags: []manifest.Flag{
				{Name: "name", Usage: "表名（必填）"},
				{Name: "schema", Usage: "schema 名"},
				{Name: "limit", Default: "100", Usage: "返回行数"},
				{Name: "offset", Default: "0", Usage: "跳过行数"},
				{Name: "order", Usage: "排序列"},
			},
			Examples: []string{"dbm-cli table -d prod-ro --name EMPLOYEES --schema HR --limit 20 -o json"}},
		{Name: "query", Summary: "执行任意 SQL（只读直执行；写受 allow_write 守卫；支持文件/stdin/参数绑定）",
			Usage: "dbm-cli query -d <ds> \"SQL\" [--file F] [--param V ...] [--limit N] [--yes]",
			Flags: []manifest.Flag{
				{Name: "file", Shorthand: "f", Usage: "从文件读取 SQL（优先级高于 stdin 与命令行参数）"},
				{Name: "param", Usage: "绑定到 ? 占位符的参数值，按顺序，可多次指定；自动按引擎转换为 ?/$1/:1"},
				{Name: "limit", Default: "1000", Usage: "只读查询返回的最大行数（<=0 不限制，防 SELECT * 拉爆大表）"},
				{Name: "yes", Usage: "跳过危险语句二次确认"},
			},
			Examples: []string{
				"dbm-cli query -d prod-ro \"SELECT * FROM HR.EMPLOYEES WHERE ROWNUM<=10\"",
				"dbm-cli query -d prod-ro \"SELECT * FROM users WHERE id=? AND status=?\" --param 100 --param active",
				"dbm-cli query -d prod-ro -f /path/to/report.sql --limit 500",
				"echo \"SELECT COUNT(*) FROM orders\" | dbm-cli query -d prod-ro",
				"dbm-cli query -d dev-rw \"DELETE FROM tmp WHERE id=1\" --yes",
			}},
		{Name: "mcp", Summary: "启动 MCP server（stdio），供 AI 客户端（Claude Desktop/Cursor）调用数据库工具",
			Usage: "dbm-cli mcp",
			Examples: []string{
				"dbm-cli mcp   # 在 stdio 上启动 MCP server，供 MCP 客户端接入",
				"# Claude Desktop 配置：{\"command\":\"dbm-cli\",\"args\":[\"mcp\"]}",
			}},
	}
}
