// Command dbm-cli 是数据库命令行工具的入口。
//
// 这里只做两件事：
//  1. 空导入 internal/oracle，触发其 init() 把 oracle driver 注册进注册表。
//     未来新增数据源时，在这里再加一行 `import _ ".../internal/mysql"` 即可。
//  2. 委托给 cli.New().Execute() 执行 cobra 命令树。
//
// 错误处理原则（面向 AI / 脚本可重试）：
//   - 任何错误都打印到 stderr，格式统一为 `[dbm-cli] error: <message>`
//   - 用 exit code 区分错误大类，便于上游判断是否值得重试：
//       0  成功
//       1  一般错误（SQL 语法、查询失败等，可能可修正后重试）
//       2  配置/连接类错误（数据源未配置、连不上库——通常是环境问题，重试前需改配置）
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/golango-cn/dbm-cli/internal/cli"
	"github.com/golango-cn/dbm-cli/internal/config"
	"github.com/golango-cn/dbm-cli/internal/driver"

	// 空导入：触发各 driver 的 init() 自注册。
	// 每新增一个数据源，在此追加一行 import _ "..."。
	_ "github.com/golango-cn/dbm-cli/internal/clickhouse"
	_ "github.com/golango-cn/dbm-cli/internal/mysql"
	_ "github.com/golango-cn/dbm-cli/internal/oracle"
	_ "github.com/golango-cn/dbm-cli/internal/postgresql"
)

func main() {
	err := cli.New().Execute()
	if err == nil {
		return
	}

	// 统一的、机器友好的错误前缀，便于 AI/脚本 grep 与解析。
	fmt.Fprintf(os.Stderr, "[dbm-cli] error: %v\n", err)

	// 按错误类别选择 exit code，让调用方知道「能否重试 / 重试前要改什么」。
	switch {
	case errors.Is(err, config.ErrNotFound):
		// 配置文件缺失——重试前需创建配置。
		fmt.Fprintln(os.Stderr, "[dbm-cli] hint: 创建配置文件 ./dbm-cli.yaml（参考 examples/config.yaml.example）后重试，或用 -c 指定路径。")
		os.Exit(2)
	case errors.Is(err, driver.ErrWriteDisabled):
		// 写操作被守卫拒绝——重试前需改 allow_write。
		fmt.Fprintln(os.Stderr, "[dbm-cli] hint: 该数据源为只读（allow_write=false）。若确需写操作，请在配置中设置 allow_write: true。")
		os.Exit(2)
	}
	os.Exit(1)
}
