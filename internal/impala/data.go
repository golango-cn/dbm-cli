package impala

import (
	"context"
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// QueryTable 实现 driver.PagedDataProvider。
// Impala 仅支持 LIMIT，不支持 OFFSET；offset>0 时返回明确错误提示用户改用 query。
func (c *conn) QueryTable(ctx context.Context, schema, table string, limit, offset int64, orderCol string) (*driver.Result, error) {
	if table == "" {
		return nil, fmt.Errorf("impala: QueryTable: table name is required")
	}
	if schema == "" {
		schema = c.cfg.Database
	}
	if schema == "" {
		schema = "default"
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
