package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// newVersionCmd 查询目标数据库版本。
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "查询目标数据库版本",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			conn, name, err := newConnWithPing(ctx)
			if err != nil {
				return err
			}
			defer conn.Close()

			v, err := conn.Version(ctx)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "datasource : %s\n", name)
			fmt.Fprintf(out, "product    : %s\n", v.Product)
			fmt.Fprintf(out, "version    : %s\n", v.Version)
			fmt.Fprintf(out, "major.minor: %d.%d\n", v.Major, v.Minor)
			return nil
		},
	}
}
