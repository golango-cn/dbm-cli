package cli

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/golango-cn/dbm-cli/internal/cli/output"
)

// newDatasourcesCmd 列出配置文件里的数据源（脱敏，不含密码）。
//
// 与 config 命令的区别：本命令更轻量，只列数据源基本字段，
// 不展示配置文件路径/timeout/Oracle 服务名等。但 description 字段会带上，
// 因为它是 AI 区分同类型数据源（如多个 MySQL）的关键信息。
// 支持 -o json 输出结构化结果，便于 AI/脚本解析。
func newDatasourcesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "datasources",
		Short: "列出已配置的数据源（隐藏密码）",
		Long:  "读取配置文件，列出全部数据源名、类型、描述、地址与是否允许写操作。密码不会被显示。",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			names := make([]string, 0, len(cfg.Datasources))
			for n := range cfg.Datasources {
				names = append(names, n)
			}
			sort.Strings(names)

			// 构造脱敏的数据源信息（与 config 命令的 configEntry 同语义）
			entries := make([]datasourceEntry, 0, len(names))
			for _, n := range names {
				ds := cfg.Datasources[n]
				entries = append(entries, datasourceEntry{
					Name:        n,
					Type:        ds.Type,
					Description: ds.Description,
					Host:        ds.Host,
					Port:        ds.Port,
					AllowWrite:  ds.AllowWrite,
					IsDefault:   n == cfg.Default,
				})
			}

			if output.ParseFormat(flagOutput) == output.FormatJSON {
				return writeDatasourcesJSON(cmd, entries)
			}
			return writeDatasourcesTable(cmd, entries)
		},
	}
}

// datasourceEntry 是单个数据源的脱敏视图（无密码），用于 datasources 命令的表格与 JSON 输出。
type datasourceEntry struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"` // 人类/AI 可读的职责说明
	Host        string `json:"host"`
	Port        int    `json:"port"`
	AllowWrite  bool   `json:"allow_write"`
	IsDefault   bool   `json:"is_default"`
}

func writeDatasourcesTable(cmd *cobra.Command, entries []datasourceEntry) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-34s %-12s %-26s %-6s %-6s\n",
		"NAME", "TYPE", "HOST", "PORT", "WRITE")
	for _, e := range entries {
		write := "no"
		if e.AllowWrite {
			write = "yes"
		}
		marker := "  "
		if e.IsDefault {
			marker = "* "
		}
		fmt.Fprintf(out, "%s%-34s %-12s %-26s %-6d %-6s\n",
			marker, e.Name, e.Type, fmt.Sprintf("%s:%d", e.Host, e.Port), e.Port, write)
		// 若配置了 description，在数据源行下方缩进展示，帮助区分职责
		if e.Description != "" {
			fmt.Fprintf(out, "  ↳ %s\n", e.Description)
		}
	}
	fmt.Fprintln(out, "\n(* = 默认数据源)")
	return nil
}

func writeDatasourcesJSON(cmd *cobra.Command, entries []datasourceEntry) error {
	out := map[string]any{"datasources": entries}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}
