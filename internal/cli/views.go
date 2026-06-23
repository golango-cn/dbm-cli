package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

func newViewsCmd() *cobra.Command {
	var schema string
	cmd := &cobra.Command{
		Use:   "views",
		Short: "列出视图",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			conn, _, err := newConnWithPing(ctx)
			if err != nil {
				return err
			}
			defer conn.Close()

			views, err := conn.Metadata().Views(ctx, schema)
			if err != nil {
				return err
			}
			res := &driver.Result{Columns: []string{"schema", "view"}}
			for _, v := range views {
				res.Rows = append(res.Rows, []any{v.Schema, v.Name})
			}
			return writeResult(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVarP(&schema, "schema", "s", "", "schema 名（默认当前用户）")
	return cmd
}
