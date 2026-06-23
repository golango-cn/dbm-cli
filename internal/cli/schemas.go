package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

func newSchemasCmd() *cobra.Command {
	var like string
	cmd := &cobra.Command{
		Use:   "schemas",
		Short: "列出 schema（user）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			conn, _, err := newConnWithPing(ctx)
			if err != nil {
				return err
			}
			defer conn.Close()

			names, err := conn.Metadata().Schemas(ctx)
			if err != nil {
				return err
			}
			res := &driver.Result{Columns: []string{"schema"}}
			for _, n := range names {
				if like != "" && !matchLike(n, like) {
					continue
				}
				res.Rows = append(res.Rows, []any{n})
			}
			return writeResult(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVarP(&like, "like", "l", "", "名称模糊匹配（SQL LIKE 语法，% 与 _）")
	return cmd
}
