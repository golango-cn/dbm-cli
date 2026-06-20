package postgresql

import (
	"context"
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// QueryTable 实现 driver.PagedDataProvider，用 LIMIT/OFFSET 分页。
func (c *conn) QueryTable(ctx context.Context, schema, table string, limit, offset int64, orderCol string) (*driver.Result, error) {
	if table == "" {
		return nil, fmt.Errorf("postgresql: QueryTable: table name is required")
	}
	if schema == "" {
		schema = "public"
	}
	if limit <= 0 {
		limit = 100
	}
	q := buildPagedSelectSQL(schema, table, limit, offset, orderCol)
	return c.Query(ctx, q)
}
