package mysql

import (
	"context"
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// QueryTable 实现 driver.PagedDataProvider，用 LIMIT/OFFSET 分页。
// 这是 MySQL 标准分页方言（全版本兼容）。
func (c *conn) QueryTable(ctx context.Context, schema, table string, limit, offset int64, orderCol string) (*driver.Result, error) {
	if table == "" {
		return nil, fmt.Errorf("mysql: QueryTable: table name is required")
	}
	if schema == "" {
		schema = c.cfg.Database
	}
	if schema == "" {
		return nil, fmt.Errorf("mysql: QueryTable: no database selected; please specify --schema or set 'database' in config")
	}
	if limit <= 0 {
		limit = 100
	}
	q := buildPagedSelectSQL(schema, table, limit, offset, orderCol)
	return c.Query(ctx, q)
}
