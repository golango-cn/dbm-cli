package cli

import (
	"context"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

func newTablesCmd() *cobra.Command {
	var schema, like string
	cmd := &cobra.Command{
		Use:   "tables",
		Short: "列出表",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			conn, _, err := newConnWithPing(ctx)
			if err != nil {
				return err
			}
			defer conn.Close()

			tables, err := conn.Metadata().Tables(ctx, schema)
			if err != nil {
				return err
			}
			res := &driver.Result{Columns: []string{"schema", "name", "type", "comment"}}
			for _, t := range tables {
				if like != "" && !matchLike(t.Name, like) {
					continue
				}
				res.Rows = append(res.Rows, []any{t.Schema, t.Name, t.Type, t.Comment})
			}
			return writeResult(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVarP(&schema, "schema", "s", "", "schema 名（默认当前用户）")
	cmd.Flags().StringVarP(&like, "like", "l", "", "表名模糊匹配（SQL LIKE 语法）")
	return cmd
}

func newColumnsCmd() *cobra.Command {
	var schema, table string
	cmd := &cobra.Command{
		Use:   "columns",
		Short: "查看表结构（列定义）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if table == "" {
				return errFlagRequired("--table/-t")
			}
			ctx := context.Background()
			conn, _, err := newConnWithPing(ctx)
			if err != nil {
				return err
			}
			defer conn.Close()

			cols, err := conn.Metadata().Columns(ctx, schema, table)
			if err != nil {
				return err
			}
			res := &driver.Result{Columns: []string{"pos", "name", "type", "length", "precision", "scale", "nullable", "default", "comment"}}
			for _, c := range cols {
				nullable := "NO"
				if c.Nullable {
					nullable = "YES"
				}
				res.Rows = append(res.Rows, []any{
					c.Position, c.Name, c.DataType, c.Length, c.Precision, c.Scale, nullable, c.DefaultValue, c.Comment,
				})
			}
			return writeResult(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVarP(&schema, "schema", "s", "", "schema 名（默认当前用户）")
	cmd.Flags().StringVarP(&table, "table", "t", "", "表名（必填）")
	return cmd
}

func newIndexesCmd() *cobra.Command {
	var schema, table string
	cmd := &cobra.Command{
		Use:   "indexes",
		Short: "查看表索引",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if table == "" {
				return errFlagRequired("--table/-t")
			}
			ctx := context.Background()
			conn, _, err := newConnWithPing(ctx)
			if err != nil {
				return err
			}
			defer conn.Close()

			idxs, err := conn.Metadata().Indexes(ctx, schema, table)
			if err != nil {
				return err
			}
			res := &driver.Result{Columns: []string{"name", "unique", "primary", "columns"}}
			for _, i := range idxs {
				uniq := "no"
				if i.IsUnique {
					uniq = "yes"
				}
				pk := "no"
				if i.IsPrimary {
					pk = "yes"
				}
				res.Rows = append(res.Rows, []any{i.Name, uniq, pk, strings.Join(i.Columns, ", ")})
			}
			return writeResult(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVarP(&schema, "schema", "s", "", "schema 名（默认当前用户）")
	cmd.Flags().StringVarP(&table, "table", "t", "", "表名（必填）")
	return cmd
}

// matchLike 用 SQL LIKE 风格（% 与 _）做匹配，大小写不敏感。
// 把 LIKE 模式翻译成正则：% → .*，_ → .，其余字符转义。
func matchLike(s, pattern string) bool {
	var b strings.Builder
	b.WriteString("(?i)^")
	for _, r := range pattern {
		switch r {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteByte('.')
		default:
			if strings.ContainsRune(`\.+*?()|[]{}^$`, r) {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('$')
	matched, _ := regexp.MatchString(b.String(), s)
	return matched
}

// errFlagRequired 返回「缺少必填 flag」错误。
func errFlagRequired(flag string) error {
	return &flagRequiredError{flag: flag}
}

type flagRequiredError struct{ flag string }

func (e *flagRequiredError) Error() string { return "required flag " + e.flag + " not set" }
