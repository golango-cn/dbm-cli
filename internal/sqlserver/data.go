package sqlserver

import (
	"context"
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// QueryTable 实现 driver.PagedDataProvider，用 OFFSET ... FETCH 分页（SQL Server 2012+）。
func (c *conn) QueryTable(ctx context.Context, schema, table string, limit, offset int64, orderCol string) (*driver.Result, error) {
	if table == "" {
		return nil, fmt.Errorf("sqlserver: QueryTable: table name is required")
	}
	if schema == "" {
		schema = "dbo"
	}
	if limit <= 0 {
		limit = 100
	}
	q, err := buildPagedSelectSQL(schema, table, limit, offset, orderCol)
	if err != nil {
		return nil, err
	}
	return c.Query(ctx, q)
}
