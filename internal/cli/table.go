package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// newTableCmd 分页读取某张表的数据。
// 通过类型断言检测驱动是否支持分页能力（driver.PagedDataProvider），
// 不支持时给出清晰提示——这避免把分页方言硬塞进通用 Conn 接口。
func newTableCmd() *cobra.Command {
	var schema, name, order string
	var limit, offset int64
	cmd := &cobra.Command{
		Use:   "table",
		Short: "分页查表数据",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return errFlagRequired("--name/-t")
			}
			ctx := context.Background()
			conn, _, err := newConnWithPing(ctx)
			if err != nil {
				return err
			}
			defer conn.Close()

			// WriteGuard 包裹了真实 Conn；取其底层以做能力断言。
			realConn := conn
			if g, ok := conn.(*driver.WriteGuard); ok {
				realConn = g.Unwrap()
			}
			p, ok := realConn.(driver.PagedDataProvider)
			if !ok {
				return fmt.Errorf("datasource type %q does not support paged table reads; use `dbm query` instead", currentDriverType())
			}

			res, err := p.QueryTable(ctx, schema, name, limit, offset, order)
			if err != nil {
				return err
			}
			return writeResult(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "t", "", "表名（必填）")
	cmd.Flags().StringVarP(&schema, "schema", "s", "", "schema 名（默认当前用户）")
	cmd.Flags().Int64VarP(&limit, "limit", "n", 100, "返回行数")
	cmd.Flags().Int64VarP(&offset, "offset", "", 0, "跳过行数")
	cmd.Flags().StringVarP(&order, "order", "", "", "排序列")
	return cmd
}

// currentDriverType 返回当前数据源的驱动类型名，用于错误提示。
func currentDriverType() string {
	cfg, err := loadConfig()
	if err != nil {
		return "unknown"
	}
	name, err := resolveDatasource(cfg)
	if err != nil {
		return "unknown"
	}
	if ds, ok := cfg.Datasources[name]; ok {
		return ds.Type
	}
	return "unknown"
}
