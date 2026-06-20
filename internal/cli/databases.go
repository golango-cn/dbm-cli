package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// newDatabasesCmd 列出库 / PDB。
func newDatabasesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "databases",
		Short: "列出库 / PDB（Oracle CDB 返回各 PDB）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			conn, _, err := newConnWithPing(ctx)
			if err != nil {
				return err
			}
			defer conn.Close()

			names, err := conn.Metadata().Databases(ctx)
			if err != nil {
				return err
			}
			res := &driver.Result{Columns: []string{"database"}}
			for _, n := range names {
				res.Rows = append(res.Rows, []any{n})
			}
			return writeResult(cmd.OutOrStdout(), res)
		},
	}
}
