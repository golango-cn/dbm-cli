package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/golango-cn/dbm-cli/internal/cli/output"
	"github.com/golango-cn/dbm-cli/internal/config"
)

// newConfigCmd 展示当前已加载的配置概览（脱敏，不含密码）。
//
// 与 datasources 命令的区别：
//   - datasources 仅列出名称/类型/地址/可写，表格形式
//   - config 额外展示配置文件路径、默认数据源、timeout 等，并支持 JSON 输出
//
// 面向 AI：AI 调用 `dbm-cli config -o json` 可获得结构化的数据源清单，
// 据此决定后续查询用哪个 -d 数据源。
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "查看当前配置概览（数据源清单、默认数据源，脱敏）",
		Long: `展示已加载的配置：配置文件路径、默认数据源、全部数据源的名称/类型/地址/可写/超时。
密码永远不显示。支持 -o json 输出结构化结果，便于 AI/脚本解析后选择数据源。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			// 解析实际加载的配置文件路径（用于展示）
			cfgPath := "<not found>"
			if flagConfig != "" {
				cfgPath = flagConfig
			} else {
				for _, p := range config.DefaultSearchPaths() {
					if info, err := os.Stat(p); err == nil && !info.IsDir() {
						cfgPath = p
						break
					}
				}
			}

			names := make([]string, 0, len(cfg.Datasources))
			for n := range cfg.Datasources {
				names = append(names, n)
			}
			sort.Strings(names)

			// 构造脱敏的数据源信息
			entries := make([]configEntry, 0, len(names))
			for _, n := range names {
				ds := cfg.Datasources[n]
				e := configEntry{
					Name:        n,
					Type:        ds.Type,
					Description: ds.Description,
					Host:        ds.Host,
					Port:        ds.Port,
					Database:    ds.Database,
					AllowWrite:  ds.AllowWrite,
					Timeout:     ds.Timeout.String(),
					IsDefault:   n == cfg.Default,
				}
				// Oracle 特有字段
				if ds.ServiceName != "" {
					e.ServiceName = ds.ServiceName
				}
				if ds.SID != "" {
					e.SID = ds.SID
				}
				entries = append(entries, e)
			}

			// 按全局 -o 决定输出格式
			if output.ParseFormat(flagOutput) == output.FormatJSON {
				return writeConfigJSON(cmd, cfgPath, cfg.Default, entries)
			}
			return writeConfigTable(cmd, cfgPath, cfg.Default, entries)
		},
	}
	return cmd
}

// configEntry 是单个数据源的脱敏视图（无密码）。
type configEntry struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"` // 人类/AI 可读的职责说明
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
	SID         string `json:"sid,omitempty"`
	AllowWrite  bool   `json:"allow_write"`
	Timeout     string `json:"timeout"`
	IsDefault   bool   `json:"is_default"`
}

func writeConfigTable(cmd *cobra.Command, cfgPath, defName string, entries []configEntry) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "config file : %s\n", cfgPath)
	fmt.Fprintf(out, "default     : %s\n", defName)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%-14s %-12s %-22s %-8s %-14s %-8s %-10s\n",
		"DATASOURCE", "TYPE", "HOST", "PORT", "DATABASE/SVC", "WRITE", "TIMEOUT")
	for _, e := range entries {
		host := fmt.Sprintf("%s:%d", e.Host, e.Port)
		dbOrSvc := e.Database
		if e.ServiceName != "" {
			dbOrSvc = "svc:" + e.ServiceName
		}
		if e.SID != "" {
			dbOrSvc = "sid:" + e.SID
		}
		marker := "  "
		if e.IsDefault {
			marker = "* "
		}
		write := "no"
		if e.AllowWrite {
			write = "yes"
		}
		fmt.Fprintf(out, "%s%-14s %-12s %-22s %-8d %-14s %-8s %-10s\n",
			marker, e.Name, e.Type, host, e.Port, dbOrSvc, write, e.Timeout)
		// 若配置了 description，在数据源行下方缩进展示，帮助区分职责
		if e.Description != "" {
			fmt.Fprintf(out, "  ↳ %s\n", e.Description)
		}
	}
	fmt.Fprintln(out, "\n(* = 默认数据源；密码已隐藏)")
	return nil
}

func writeConfigJSON(cmd *cobra.Command, cfgPath, defName string, entries []configEntry) error {
	out := map[string]any{
		"config_file": cfgPath,
		"default":     defName,
		"datasources": entries,
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}
