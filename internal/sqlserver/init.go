// Package sqlserver 实现 Microsoft SQL Server 数据库驱动（纯 Go，经 go-mssqldb，无 CGO）。
//
// 兼容性：go-mssqldb 是微软官方维护的纯 Go 驱动，无需 ODBC Driver，无需 CGO。
// 覆盖 SQL Server 2017 / 2019 / 2022（实测 2017 与 2022）。
//
// 本包在被 import 时通过 init() 自动注册到 driver 注册表，注册名 "sqlserver"。
// CLI 在 main 里空导入本包即生效。
package sqlserver

import (
	// 空导入 go-mssqldb：触发其 init() 把自身注册到 database/sql
	// （注册名 "sqlserver"），使 sql.Open("sqlserver", ...) 可用。
	_ "github.com/microsoft/go-mssqldb"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

func init() {
	driver.Register(&Driver{})
}
