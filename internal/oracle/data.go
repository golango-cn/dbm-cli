package oracle

import (
	"context"
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// QueryTable 实现 driver.PagedDataProvider，用 ROWNUM 包装做分页。
// 这是 Oracle 特有方言（全版本兼容），因此放在 oracle 包内而非通用层。
//
// 错误处理：schema 为空时返回明确错误，提示用户显式指定 --schema，
// 而非静默用空 schema 拼出非法 SQL。这与 columns/indexes 等命令行为一致。
func (c *conn) QueryTable(ctx context.Context, schema, table string, limit, offset int64, orderCol string) (*driver.Result, error) {
	if table == "" {
		return nil, fmt.Errorf("oracle: QueryTable: table name is required")
	}
	if schema == "" {
		s, err := c.currentSchemaName(ctx)
		if err != nil {
			return nil, fmt.Errorf("oracle: QueryTable: %w; please specify --schema", err)
		}
		schema = s
	}
	if limit <= 0 {
		limit = 100
	}
	q := buildPagedSelectSQL(schema, table, limit, offset, orderCol)
	res, err := c.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	// buildPagedSelectSQL 注入了辅助列 rnum；从结果集中剔除它，只暴露原表列。
	res.Columns, res.Rows = dropColumn(res.Columns, res.Rows, "RNUM")
	return res, nil
}

// currentSchemaName 返回当前会话 schema（大写），供 QueryTable 默认值使用。
func (c *conn) currentSchemaName(ctx context.Context) (string, error) {
	var u string
	if err := c.pool.QueryRowContext(ctx,
		"SELECT sys_context('USERENV','CURRENT_SCHEMA') FROM dual").Scan(&u); err == nil && u != "" {
		return u, nil
	}
	if err := c.pool.QueryRowContext(ctx, "SELECT user FROM dual").Scan(&u); err != nil {
		return "", fmt.Errorf("cannot determine current schema (CURRENT_SCHEMA and user both failed)")
	}
	if u == "" {
		return "", fmt.Errorf("cannot determine current schema (both queries returned empty)")
	}
	return u, nil
}

// dropColumn 从结果集中移除指定列名（大小写不敏感），返回新的列与行。
func dropColumn(cols []string, rows [][]any, name string) ([]string, [][]any) {
	idx := -1
	for i, c := range cols {
		if equalFold(c, name) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return cols, rows
	}
	newCols := append([]string{}, cols[:idx]...)
	newCols = append(newCols, cols[idx+1:]...)
	for ri, row := range rows {
		if idx < len(row) {
			newRow := append([]any{}, row[:idx]...)
			newRow = append(newRow, row[idx+1:]...)
			rows[ri] = newRow
		}
	}
	return newCols, rows
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 32
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
