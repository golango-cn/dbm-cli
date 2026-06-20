// Package oracle 实现 Oracle 数据库驱动（纯 Go，经 go-ora/v2，无 CGO / 无 Instant Client）。
//
// 本包在被 import 时通过 init() 自动注册到 driver 注册表，注册名 "oracle"。
// CLI 只需在 main 里空导入本包（import _ "github.com/golango-cn/dbm-cli/internal/oracle"），
// 即可使配置里 type: oracle 生效——这正是「自注册 + 注册表」模式的收益。
package oracle

import "github.com/golango-cn/dbm-cli/internal/driver"

func init() {
	driver.Register(&Driver{})
}
