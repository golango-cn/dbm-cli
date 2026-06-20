package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/golango-cn/dbm-cli/internal/driver"
	"github.com/golango-cn/dbm-cli/internal/query"
)

// defaultQueryLimit 是 query 命令对结果集的默认行数上限。
// 用于防止 agent/用户误执行 SELECT * 拉爆大表。设为 1000 在「够用」与「防失控」间取平衡。
const defaultQueryLimit = 1000

func newQueryCmd() *cobra.Command {
	var (
		yes   bool
		file  string
		param []string
		limit int
	)
	cmd := &cobra.Command{
		Use:   "query [sql]",
		Short: "执行任意 SQL（只读直接执行；写操作受 allow_write 守卫，危险语句需二次确认）",
		Long: `执行任意 SQL 语句。

SQL 来源（优先级从高到低）：
  1. --file <路径>     从文件读取 SQL
  2. stdin（管道输入） 当标准输入非 tty 时读取，支持 cat a.sql | dbm-cli query 或 heredoc
  3. 命令行参数        query "SELECT ..."

参数绑定（防注入，推荐）：
  SQL 中用 ? 作占位符，用 --param 按顺序传值，自动按引擎转换（MySQL/CH=?、PG=$1、Oracle=:1）：
    dbm-cli query "SELECT * FROM t WHERE id=? AND name=?" --param 100 --param alice

结果集保护：
  --limit 限制返回行数（默认 ` + strconv.Itoa(defaultQueryLimit) + `，<=0 表示不限制）。仅对只读 SELECT 生效。
`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sqlStr, err := resolveSQLSource(cmd.InOrStdin(), file, args)
			if err != nil {
				return err
			}

			ctx := context.Background()
			conn, _, dsCfg, err := newConnWithPingCfg(ctx)
			if err != nil {
				return err
			}
			defer conn.Close()

			// 解析参数（若提供）：--param 按顺序绑定到 ? 占位符。
			// 占位符统一用 ?，按当前驱动风格归一化为引擎原生占位符后下发。
			var sqlArgs []any
			if len(param) > 0 {
				sqlArgs, err = parseParams(param)
				if err != nil {
					return err
				}
				need := query.CountPlaceholders(sqlStr)
				if len(sqlArgs) != need {
					return fmt.Errorf("参数个数 %d 与占位符 ? 个数 %d 不匹配", len(sqlArgs), need)
				}
				style := query.PlaceholderStyleFor(dsCfg.Type)
				sqlStr = query.NormalizePlaceholders(sqlStr, style)
			}

			kind := query.Classify(sqlStr)

			// 只读语句：直接走 Query。
			if kind.IsReadOnly() {
				res, qerr := conn.Query(ctx, sqlStr, sqlArgs...)
				if qerr != nil {
					return qerr
				}
				res = capRows(res, limit)
				return writeResult(cmd.OutOrStdout(), res)
			}

			// 非只读语句：先看数据源是否允许写。
			// allow_write=false 是数据源级硬限制，优先于交互式二次确认——
			// 只读库不应先弹「确认执行吗」误导用户，而应直接拒绝并给出明确提示。
			// 用 WriteDisabledError 包装，保持 errors.Is(err, ErrWriteDisabled) 成立，
			// 从而 main.go 返回 exit code=2（配置/权限类错误）。
			if !dsCfg.AllowWrite {
				return driver.NewWriteDisabledError(fmt.Sprintf(
					"拒绝执行写语句（%s）：数据源 %q 为只读（allow_write=false）。"+
					"若确需写操作，请在配置中设置 allow_write: true，或改用已开启 allow_write 的数据源",
					kind, currentDatasourceName()))
			}

			// allow_write=true 时，再叠加「危险语句二次确认」一层防御。
			if query.IsDestructive(sqlStr) && !yes {
				prompt := fmt.Sprintf("⚠ 该语句被判定为高风险（%s），确认执行？\n  %s",
					kind, truncate(sqlStr, 200))
				if !confirm(prompt) {
					return errors.New("aborted by user")
				}
			}

			res, err := conn.Exec(ctx, sqlStr, sqlArgs...)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "OK (rows affected: %d)\n", res.RowsAffected)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "跳过危险语句的交互式二次确认（AI/脚本场景使用）")
	cmd.Flags().StringVarP(&file, "file", "f", "", "从文件读取 SQL")
	cmd.Flags().StringArrayVar(&param, "param", nil, "绑定到 ? 占位符的参数值（按顺序，可多次指定）")
	cmd.Flags().IntVar(&limit, "limit", defaultQueryLimit, "只读查询返回的最大行数（<=0 表示不限制）")
	return cmd
}

// resolveSQLSource 按优先级解析 SQL 来源：--file > stdin(非tty) > 命令行参数。
func resolveSQLSource(stdin io.Reader, file string, args []string) (string, error) {
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read --file %q: %w", file, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	// stdin 非 tty 时读取（管道 / heredoc）。isTTY 判断避免阻塞等待人类输入。
	if data, ok := readStdinIfPiped(stdin); ok {
		return strings.TrimSpace(data), nil
	}
	if len(args) == 0 {
		return "", errors.New("missing SQL statement: pass as argument, --file, or pipe via stdin")
	}
	return args[0], nil
}

// readStdinIfPiped 当 stdin 不是终端（即被管道/重定向喂入数据）时，读取全部内容。
// 返回 (内容, 是否读取了)。是 tty 时返回 ("", false)，避免阻塞等待人工输入。
func readStdinIfPiped(stdin io.Reader) (string, bool) {
	f, ok := stdin.(*os.File)
	if !ok {
		// 测试注入的非 *os.File reader：有数据则读，无数据则跳过。
		return readNonFileStdin(stdin)
	}
	stat, err := f.Stat()
	if err != nil {
		return "", false
	}
	// 管道/重定向设备文件（mode Device bit 置位）才读取；tty 不读。
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "", false
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// readNonFileStdin 处理测试注入的 reader：尝试读，空则视作无输入。
func readNonFileStdin(stdin io.Reader) (string, bool) {
	data, err := io.ReadAll(stdin)
	if err != nil || len(data) == 0 {
		return "", false
	}
	return string(data), true
}

// parseParams 把 --param 的字符串值解析为 any 切片。
// 尝试把纯数字串解析为 int（数据库驱动通常按值类型匹配），解析失败则保留字符串。
func parseParams(raw []string) ([]any, error) {
	out := make([]any, 0, len(raw))
	for _, s := range raw {
		out = append(out, coerceParam(s))
	}
	return out, nil
}

// coerceParam 尝试把参数字符串转换为更精确的类型：
// 纯整数 → int64；其余 → 原字符串。这样 WHERE id=? 配合 --param 100 时绑定数值而非字符串。
func coerceParam(s string) any {
	// 整数？
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	return s
}

// capRows 按上限截断结果集行数。limit<=0 表示不限制。
func capRows(res *driver.Result, limit int) *driver.Result {
	if limit <= 0 || len(res.Rows) <= limit {
		return res
	}
	res.Rows = res.Rows[:limit]
	return res
}

// currentDatasourceName 返回当前生效的数据源名（-d 指定 或 配置的 default），
// 供错误提示引用。解析失败时回退为 "unknown"。
func currentDatasourceName() string {
	cfg, err := loadConfig()
	if err != nil {
		return "unknown"
	}
	name, err := resolveDatasource(cfg)
	if err != nil {
		return "unknown"
	}
	return name
}

// truncate 截断长字符串用于提示。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
