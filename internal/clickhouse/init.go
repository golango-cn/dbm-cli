// Package clickhouse 实现 ClickHouse 数据库驱动（纯 Go，经 clickhouse-go/v2，无 CGO）。
//
// 兼容性：clickhouse-go/v2 自 v2.3 起底层用 ch-go（纯 Go）走原生 TCP 协议，
// 无需任何 ClickHouse 客户端库；覆盖 ClickHouse 22.x 及以上（实测 25.x）。
//
// 本包在被 import 时通过 init() 自动注册到 driver 注册表，注册名 "clickhouse"。
// CLI 在 main 里空导入本包即生效。
package clickhouse

import (
	// 空导入 clickhouse-go/v2：触发其 init() 把自身注册到 database/sql
	// （注册名 "clickhouse"），使 sql.Open("clickhouse", ...) 可用。
	_ "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

func init() {
	driver.Register(&Driver{})
}
