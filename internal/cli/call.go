package cli

// call 命令：调用存储过程，回收 OUT 参数并捕获 DBMS_OUTPUT（print 输出）。
//
// 语法：
//   dbm-cli call <proc> --in <v>... --out <name>:<type>...
//
// --in  : IN 参数，按顺序绑定（值自动尝试数值/字符串转换）
// --out : OUT 参数，格式 name:type，type ∈ number|string|cursor
//
// 输出：
//   - DBMS_OUTPUT 文本 → stderr（默认开启，--no-print 关闭）
//   - OUT 标量参数   → stdout（name = value）
//   - REF CURSOR     → stdout（按 -o 全局格式）

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

func newCallCmd() *cobra.Command {
	var (
		inVals  []string
		outSpec []string
		noPrint bool
	)
	cmd := &cobra.Command{
		Use:   "call <procedure> [--in value...] [--out name:type...]",
		Short: "调用存储过程（Oracle），回收 OUT 参数并捕获 DBMS_OUTPUT 打印",
		Long: `调用存储过程，自动回收 OUT 标量参数与 REF CURSOR 结果集，
并捕获过程内 DBMS_OUTPUT.PUT_LINE 的打印输出（默认输出到 stderr）。

  dbm-cli call add_numbers --in 3 --in 5 --out sum:number
  dbm-cli call get_users --in 2 --out users:cursor
  dbm-cli call get_user_info --in 42 --out status:string --out count:number --out rows:cursor

参数绑定顺序：先所有 --in（占位符 :1..:N），再所有 --out（占位符 :N+1..）。
OUT 类型：number（数值）、string（字符串）、cursor（结果集/REF CURSOR）。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			proc := args[0]
			ctx := context.Background()

			conn, _, _, err := newConnWithPingCfg(ctx)
			if err != nil {
				return err
			}
			defer conn.Close()

			// WriteGuard 包裹了 conn；CallProc 在底层 oracle.conn 上，需 Unwrap。
			// 暴露 Unwrap 的可能是 *WriteGuard，也可能是透传的其它装饰器。
			base := unwrapConn(conn)
			sp, ok := base.(driver.StoredProcCaller)
			if !ok {
				return fmt.Errorf("当前数据源驱动不支持存储过程调用（driver.StoredProcCaller 未实现）")
			}

			params, err := buildProcParams(inVals, outSpec)
			if err != nil {
				return err
			}

			res, err := sp.CallProc(ctx, proc, params)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()

			// 1. DBMS_OUTPUT → stderr（默认）
			if !noPrint && strings.TrimSpace(res.Output) != "" {
				fmt.Fprintf(errOut, "%s", res.Output)
				if !strings.HasSuffix(res.Output, "\n") {
					fmt.Fprintln(errOut)
				}
			}

			// 2. OUT 标量 → stdout
			for name, val := range res.OutParams {
				fmt.Fprintf(out, "%s = %v\n", name, formatOutVal(val))
			}

			// 3. REF CURSOR → stdout（按全局格式）
			if res.ResultSet != nil {
				if err := writeResult(out, res.ResultSet); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&inVals, "in", nil, "IN 参数值（按顺序，可多次指定）")
	cmd.Flags().StringArrayVar(&outSpec, "out", nil, "OUT 参数：name:type（type=number|string|cursor，可多次）")
	cmd.Flags().BoolVar(&noPrint, "no-print", false, "不捕获/打印 DBMS_OUTPUT")
	return cmd
}

// unwrapConn 逐层解开 WriteGuard 等装饰器，拿到底层 driver.Conn。
// WriteGuard 有 Unwrap() Conn 方法。
func unwrapConn(c driver.Conn) driver.Conn {
	type unwrap interface{ Unwrap() driver.Conn }
	for {
		u, ok := c.(unwrap)
		if !ok {
			return c
		}
		inner := u.Unwrap()
		if inner == c {
			return c
		}
		c = inner
	}
}

// buildProcParams 把 --in/--out 组装成 ProcParam 切片（先 IN 后 OUT）。
func buildProcParams(inVals, outSpec []string) ([]driver.ProcParam, error) {
	var params []driver.ProcParam
	for _, v := range inVals {
		params = append(params, driver.ProcParam{Value: coerceParam(v), Direction: driver.ParamIn})
	}
	for _, spec := range outSpec {
		// spec 形如 name:type
		idx := strings.Index(spec, ":")
		if idx <= 0 || idx == len(spec)-1 {
			return nil, fmt.Errorf("invalid --out %q：应为 name:type（type=number|string|cursor）", spec)
		}
		name := spec[:idx]
		typ := spec[idx+1:]
		params = append(params, driver.ProcParam{
			Name:      name,
			Direction: driver.ParamOut,
			OutType:   typ,
		})
	}
	if len(params) == 0 {
		return nil, errors.New("至少需要一个 --in 或 --out 参数")
	}
	return params, nil
}

// formatOutVal 美化 OUT 值的显示（float64 整数化等）。
func formatOutVal(v any) any {
	switch x := v.(type) {
	case float64:
		// 整数值去掉小数点
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return x
	}
	return v
}
