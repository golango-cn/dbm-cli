// Package impala 实现 Apache Impala 数据库驱动（纯 Go，经 impala-go，无 CGO）。
//
// 兼容性：impala-go 基于 HiveServer2 协议（端口 21050），无 CGO、无原生库依赖，
// 覆盖 Impala 3.x / 4.x（实测 4.5.0）。
//
// 本包在被 import 时通过 init() 自动注册到 driver 注册表，注册名 "impala"。
// CLI 在 main 里空导入本包即生效。
package impala

import (
	// 空导入 impala-go：触发其 init() 把自身注册到 database/sql
	// （注册名 "impala"），使 sql.Open("impala", ...) 可用。
	_ "github.com/sclgo/impala"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

func init() {
	driver.Register(&Driver{})
}
