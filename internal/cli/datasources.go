package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// newDatasourcesCmd 列出配置文件里的数据源（脱敏，不含密码）。
func newDatasourcesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "datasources",
		Short: "列出已配置的数据源（隐藏密码）",
		Long:  "读取配置文件，列出全部数据源名、类型、地址与是否允许写操作。密码不会被显示。",
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

			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-10s %-30s %-6s %-10s\n",
				"NAME", "TYPE", "HOST", "PORT", "WRITE")
			for _, n := range names {
				ds := cfg.Datasources[n]
				write := "no"
				if ds.AllowWrite {
					write = "yes"
				}
				marker := "  "
				if n == cfg.Default {
					marker = "* "
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%-20s %-10s %-30s %-6d %-10s\n",
					marker, n, ds.Type, fmt.Sprintf("%s:%d", ds.Host, ds.Port), ds.Port, write)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\n(* = 默认数据源)")
			return nil
		},
	}
}
