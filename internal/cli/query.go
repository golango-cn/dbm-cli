package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/golango-cn/dbm-cli/internal/driver"
	"github.com/golango-cn/dbm-cli/internal/query"
)

func newQueryCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "query [sql]",
		Short: "执行任意 SQL（只读直接执行；写操作受 allow_write 守卫，危险语句需二次确认）",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("missing SQL statement")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			sql := args[0]
			kind := query.Classify(sql)

			ctx := context.Background()
			conn, _, err := newConnWithPing(ctx)
			if err != nil {
				return err
			}
			defer conn.Close()

			// 只读语句：直接走 Query。
			if kind.IsReadOnly() {
				res, qerr := conn.Query(ctx, sql)
				if qerr != nil {
					return qerr
				}
				return writeResult(cmd.OutOrStdout(), res)
			}

			// 非只读：受 WriteGuard 控制。WriteGuard 已按 allow_write 决定是否放行，
			// 这里再叠加「危险语句二次确认」一层防御。
			if query.IsDestructive(sql) && !yes {
				prompt := fmt.Sprintf("⚠ 该语句被判定为高风险（%s），确认执行？\n  %s",
					kind, truncate(sql, 200))
				if !confirm(prompt) {
					return errors.New("aborted by user")
				}
			}

			res, err := conn.Exec(ctx, sql)
			if err != nil {
				// ErrWriteDisabled 来自 guard，给出更友好的提示。
				if errors.Is(err, driver.ErrWriteDisabled) {
					return fmt.Errorf("%w\n提示：在配置里给该数据源设置 allow_write: true 以启用写操作", err)
				}
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "OK (rows affected: %d)\n", res.RowsAffected)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "跳过危险语句的交互式二次确认（AI/脚本场景使用）")
	return cmd
}

// truncate 截断长字符串用于提示。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
