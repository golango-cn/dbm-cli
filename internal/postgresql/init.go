// Package postgresql 实现 PostgreSQL 数据库驱动（纯 Go，经 pgx/v5，无 CGO）。
//
// 兼容性：pgx/v5 原生支持 PostgreSQL 9.6+，实测覆盖 pg 12 / 17。
// 走标准 database/sql 接口，元数据来自 information_schema 与 pg_catalog。
//
// 本包在被 import 时通过 init() 自动注册到 driver 注册表，注册名 "postgresql"。
// CLI 在 main 里空导入本包即生效。
package postgresql

import (
	// 空导入 pgx/v5/stdlib：触发其 init() 把自身注册到 database/sql
	// （注册名 "pgx"），使 sql.Open("pgx", ...) 可用。
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

func init() {
	driver.Register(&Driver{})
}
