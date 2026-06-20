// Package mysql 实现 MySQL 数据库驱动（纯 Go，经 go-sql-driver/mysql，无 CGO）。
//
// 支持 MySQL 5.7 / 8.0.x（含 8.0.12、8.0.37 等）。
// 8.0 默认认证插件 caching_sha2_password 由驱动原生支持（需驱动 v1.5+）。
//
// 同时通过别名支持 MariaDB：MariaDB 是 MySQL 的分支，协议高度兼容，
// go-sql-driver/mysql 驱动可直接连接 MariaDB。注册别名 "mariadb" -> "mysql" 后，
// 配置里写 type: mariadb 即等价于 type: mysql。
//
// 本包在被 import 时通过 init() 自动注册到 driver 注册表，注册名 "mysql"。
// CLI 在 main 里空导入本包（import _ ".../internal/mysql"）即生效。
package mysql

import (
	// 空导入 go-sql-driver/mysql：触发其 init() 把自身注册到 database/sql
	// （注册名 "mysql"），这样我们的 sql.Open("mysql", ...) 才能找到驱动。
	_ "github.com/go-sql-driver/mysql"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

func init() {
	driver.Register(&Driver{})
	// MariaDB 协议兼容 MySQL，复用同一驱动实现。
	// 配置里 type: mariadb 与 type: mysql 等价。
	driver.RegisterAlias("mariadb", "mysql")
}
