// Package impala 实现 Apache Impala 数据库驱动（纯 Go，无 CGO）。
//
// 兼容性：底层驱动（vendored 在 thirdparty/impala/）基于 HiveServer2 协议（端口 21050），
// 无 CGO、无原生库依赖，覆盖 Impala 3.x / 4.x（实测 4.5.0）。
// 支持 SASL PLAIN（用户名/密码 LDAP）与 SASL GSSAPI（Kerberos）两种认证。
//
// 本包在被 import 时通过 init() 自动注册到 driver 注册表，注册名 "impala"。
// CLI 在 main 里空导入本包即生效。
package impala

import (
	// 空导入 vendored 驱动：触发其 init() 把自身注册到 database/sql
	// （注册名 "impala"），使 sql.Open("impala", ...) 可用。
	_ "github.com/golango-cn/dbm-cli/internal/impala/thirdparty/impala"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

func init() {
	driver.Register(&Driver{})
}
