// Package cli 实现 dbm 的全部 cobra 命令。
//
// 设计原则：命令层保持「薄」。每个命令只做三件事：
//  1. 解析 flag
//  2. 经由 NewConn 拿到 driver.Conn（带写守卫）
//  3. 调用 driver.Conn 的方法 + 用 output 渲染
//
// 业务逻辑（连接、元数据、SQL）都在 driver / oracle 层，便于复用与测试。
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/golango-cn/dbm-cli/internal/buildinfo"
	"github.com/golango-cn/dbm-cli/internal/cli/output"
	"github.com/golango-cn/dbm-cli/internal/config"
	"github.com/golango-cn/dbm-cli/internal/driver"
)

// globals 保存全局 flag 解析结果，供子命令读取。
// 用包级变量是因为 cobra 的命令是分离对象，传参会很繁琐；这是 CLI 工具的常见取舍。
var (
	flagConfig     string
	flagDatasource string
	flagOutput     string
	flagNoHeader   bool
)

// loadedConfig 缓存已加载配置（lazy），供多个命令复用，避免重复读盘。
var loadedConfig *config.File

// New 构建根命令及其所有子命令。
// import 的副作用（driver 自注册）在 main 里通过空导入触发。
func New() *cobra.Command {
	root := &cobra.Command{
		Use:   buildinfo.AppName,
		Short: "dbm-cli — zero-dependency database CLI (Oracle first, extensible)",
		Long: `dbm-cli 是一个零外部依赖的数据库命令行工具：单个静态二进制即可查询数据库元数据与数据。

先支持 Oracle（纯 Go 驱动 go-ora，无需 Instant Client / 无需 CGO），
后续通过 driver 接口扩展 mysql / sqlserver 等。

配置：通过 YAML 文件管理数据源连接串（见 --config）。
面向 AI：运行 ` + "`dbm-cli manifest`" + ` 获取自描述清单。
`,
		SilenceUsage:   true,
		SilenceErrors:  true,
	}

	root.PersistentFlags().StringVarP(&flagConfig, "config", "c", "",
		"配置文件路径（默认按 ./dbm-cli.yaml → ~/.config/dbm-cli/config.yaml → ~/.dbm-cli.yaml 查找）")
	root.PersistentFlags().StringVarP(&flagDatasource, "datasource", "d", "",
		"使用的数据源名（未指定则用配置里的 default）")
	root.PersistentFlags().StringVarP(&flagOutput, "output", "o", "table",
		"输出格式：table|json|csv|yaml|vertical")
	root.PersistentFlags().BoolVar(&flagNoHeader, "no-header", false,
		"table/csv 输出时省略表头")

	root.AddCommand(
		newManifestCmd(),
		newConfigCmd(),
		newDatasourcesCmd(),
		newVersionCmd(),
		newDatabasesCmd(),
		newSchemasCmd(),
		newTablesCmd(),
		newColumnsCmd(),
		newIndexesCmd(),
		newViewsCmd(),
		newTableCmd(),
		newQueryCmd(),
	)

	// 把 cobra 自动生成的 completion 命令描述改为中文。
	// completion 命令由 InitDefaultCompletionCmd 生成，这里触发后改写其文案。
	root.InitDefaultCompletionCmd()
	for _, c := range root.Commands() {
		if c.Name() != "completion" {
			continue
		}
		c.Short = "生成 shell 自动补全脚本"
		c.Long = `为 dbm-cli 生成指定 shell 的自动补全脚本。
安装补全后，在终端输入 dbm-cli 并按 TAB 即可补全命令与参数。
详见各子命令帮助（如 dbm-cli completion bash --help）。`
		for _, sc := range c.Commands() {
			switch sc.Name() {
			case "bash":
				sc.Short = "生成 bash 补全脚本"
			case "zsh":
				sc.Short = "生成 zsh 补全脚本"
			case "fish":
				sc.Short = "生成 fish 补全脚本"
			case "powershell":
				sc.Short = "生成 powershell 补全脚本"
			}
		}
	}

	return root
}

// loadConfig 懒加载配置（首次调用解析，后续复用）。
func loadConfig() (*config.File, error) {
	if loadedConfig != nil {
		return loadedConfig, nil
	}
	f, err := config.Load(flagConfig)
	if err != nil {
		return nil, err
	}
	loadedConfig = f
	return f, nil
}

// resolveDatasource 解析目标数据源名：优先 -d，否则用 default。
func resolveDatasource(cfg *config.File) (string, error) {
	if flagDatasource != "" {
		return flagDatasource, nil
	}
	if cfg.Default != "" {
		return cfg.Default, nil
	}
	return "", errors.New("no datasource specified: use -d/--datasource or set a default in config")
}

// newConn 解析配置并打开到指定数据源的连接（带写守卫）。
// 返回的 driver.Conn 已被 WriteGuard 包裹；调用方负责 Close。
// 注意：本函数不做连通性校验（datasources/manifest 命令不需要连库）。
func newConn(ctx context.Context) (driver.Conn, string, error) {
	conn, name, _, err := newConnWithCfg(ctx)
	return conn, name, err
}

// newConnWithCfg 同 newConn，但额外返回 DatasourceConfig，供调用方读取 timeout 等。
func newConnWithCfg(ctx context.Context) (driver.Conn, string, *driver.DatasourceConfig, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, "", nil, err
	}
	name, err := resolveDatasource(cfg)
	if err != nil {
		return nil, "", nil, err
	}
	dsCfg, ok := cfg.Datasources[name]
	if !ok {
		return nil, "", nil, fmt.Errorf("datasource %q not found in config", name)
	}
	d, err := driver.Get(dsCfg.Type)
	if err != nil {
		return nil, "", nil, err
	}
	c, err := d.Open(dsCfg)
	if err != nil {
		return nil, "", nil, err
	}
	return driver.NewWriteGuard(c, dsCfg.AllowWrite), name, dsCfg, nil
}

// newConnWithPing 打开连接并立即 Ping 验证连通性。
// 供需要真实访问数据库的命令使用，让配置/网络错误尽早、清晰暴露。
// 连接超时由数据源配置的 timeout 决定（默认 30s）；超时则报详细错误，便于定位。
func newConnWithPing(ctx context.Context) (driver.Conn, string, error) {
	conn, name, _, err := newConnWithPingCfg(ctx)
	return conn, name, err
}

// newConnWithPingCfg 同 newConnWithPing，但额外返回 DatasourceConfig。
// 供需要读取驱动类型（如 query 命令做占位符归一化）或 timeout 的调用方使用。
func newConnWithPingCfg(ctx context.Context) (driver.Conn, string, *driver.DatasourceConfig, error) {
	conn, name, dsCfg, err := newConnWithCfg(ctx)
	if err != nil {
		return nil, name, nil, err
	}
	// 取配置的超时（默认 30s），给 Ping 加截止时间。
	timeout := dsCfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := conn.Ping(pingCtx); err != nil {
		conn.Close()
		// 区分超时与其它错误，给出更易读的提示。
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, name, nil, fmt.Errorf("cannot connect to datasource %q: timed out after %s (check host/port=%s:%d reachable, credentials correct, and network/firewall allows access)",
				name, timeout, dsCfg.Host, dsCfg.Port)
		}
		return nil, name, nil, fmt.Errorf("cannot connect to datasource %q: %w", name, err)
	}
	return conn, name, dsCfg, nil
}

// outOpts 根据全局 flag 构造输出选项。
func outOpts() output.Options {
	return output.Options{
		Format:   output.ParseFormat(flagOutput),
		NoHeader: flagNoHeader,
	}
}

// writeResult 把 Result 按全局格式写出。
func writeResult(w io.Writer, r *driver.Result) error {
	return output.Write(w, r, outOpts())
}

// stdinPipe 返回 os.Stdin，便于测试时替换。
var stdinPipe io.Reader = os.Stdin

// confirm 在终端交互式询问 y/n。
func confirm(prompt string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", prompt)
	var resp string
	_, err := fmt.Fscanln(stdinPipe, &resp)
	if err != nil {
		return false
	}
	switch resp {
	case "y", "Y", "yes", "YES":
		return true
	}
	return false
}
