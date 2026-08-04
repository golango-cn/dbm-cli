package oracle

// 存储过程调用 + DBMS_OUTPUT 捕获（go-ora 驱动）。
//
// 关键约束：DBMS_OUTPUT 的缓冲区是 SESSION 级的 —— ENABLE、过程调用、GET_LINES
// 必须在同一个底层连接（*sql.Conn）上执行，否则读不到输出。因此 CallProc 用
// pool.Conn(ctx) 钉住一条连接，全程不归还连接池。
//
// 不依赖 go-ora 的 dbms 子包（它内部自行钉连接，无法与过程调用共享 session），
// 而是手动在同一 *sql.Conn 上跑 DBMS_OUTPUT.ENABLE / GET_LINES。

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	go_ora "github.com/sijms/go-ora/v2"
	"github.com/golango-cn/dbm-cli/internal/driver"
)

// callOutBinding 是单个 OUT 参数在调用期间的「回收槽」。
// 一次 CallProc 里，每个 OUT 参数对应一个 binding，Exec 后从中读取结果。
type callOutBinding struct {
	name    string             // 参数名（用于回显）
	outType string             // number | string | cursor
	numDest *float64           // number 回收槽
	strDest *sql.NullString    // string 回收槽
	curDest *go_ora.RefCursor  // cursor 回收槽
}

// CallProc 实现 driver.StoredProcCaller。
//
// params 顺序即绑定顺序，调用方负责把 IN 排在前、OUT 排在后（与占位符 :1,:2... 对应）。
// 返回 ProcResult：OutParams（标量）、ResultSet（游标，仅取第一个 cursor）、Output（DBMS_OUTPUT 文本）。
func (c *conn) CallProc(ctx context.Context, proc string, params []driver.ProcParam) (*driver.ProcResult, error) {
	// 1. 钉住一条连接：DBMS_OUTPUT buffer 是 session 级，全程必须同一连接。
	dbConn, err := c.pool.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("oracle: get conn for proc: %w", err)
	}
	defer dbConn.Close()

	// 2. 开启 DBMS_OUTPUT（同一 session）。
	if _, err := dbConn.ExecContext(ctx, "BEGIN DBMS_OUTPUT.ENABLE(NULL); END;"); err != nil {
		return nil, fmt.Errorf("oracle: DBMS_OUTPUT.ENABLE: %w", err)
	}

	// 3. 构造 PL/SQL 与绑定参数。
	//    占位符 :1..:n 与 params 顺序一一对应。
	placeholders := make([]string, len(params))
	args := make([]any, len(params))
	outBindings := make([]*callOutBinding, 0, len(params))
	for i, p := range params {
		placeholders[i] = fmt.Sprintf(":%d", i+1)
		switch p.Direction {
		case driver.ParamIn:
			args[i] = p.Value
		case driver.ParamOut, driver.ParamInOut:
			b := &callOutBinding{name: p.Name, outType: normalizeOutType(p.OutType)}
			switch b.outType {
			case "cursor":
				var rc go_ora.RefCursor
				b.curDest = &rc
				if p.Direction == driver.ParamInOut {
					// INOUT cursor 罕见；IN 部分用 Value，这里仍按纯 OUT 游标处理。
					args[i] = go_ora.Out{Dest: b.curDest}
				} else {
					args[i] = sql.Out{Dest: b.curDest}
				}
			case "number":
				var n float64
				b.numDest = &n
				args[i] = sql.Out{Dest: b.numDest}
			default: // string
				var s sql.NullString
				b.strDest = &s
				size := p.OutSize
				if size <= 0 {
					size = 4000
				}
				args[i] = go_ora.Out{Dest: b.strDest, Size: size}
			}
			outBindings = append(outBindings, b)
		default:
			args[i] = p.Value // 未知方向按 IN 处理
		}
	}
	plsql := fmt.Sprintf("BEGIN %s(%s); END;", proc, strings.Join(placeholders, ", "))

	// 4. 执行存储过程。
	if _, err := dbConn.ExecContext(ctx, plsql, args...); err != nil {
		return nil, fmt.Errorf("oracle: exec proc %s: %w", proc, err)
	}

	// 5. 回收 OUT 值；若有 cursor，先读游标（在 GetOutput 之前完成游标读取，
	//    避免 GET_LINES 与游标 fetch 交叉消耗 session 资源）。
	result := &driver.ProcResult{OutParams: map[string]any{}}
	var cursorBinding *callOutBinding
	for _, b := range outBindings {
		switch b.outType {
		case "cursor":
			if cursorBinding == nil {
				cursorBinding = b // 仅取首个游标
			}
		case "number":
			if b.numDest != nil {
				result.OutParams[b.name] = *b.numDest
			}
		case "string":
			if b.strDest != nil {
				result.OutParams[b.name] = b.strDest.String
			}
		}
	}
	if cursorBinding != nil && cursorBinding.curDest != nil {
		rows, err := go_ora.WrapRefCursor(ctx, dbConn, cursorBinding.curDest)
		if err != nil {
			return nil, fmt.Errorf("oracle: wrap ref cursor: %w", err)
		}
		rs, err := rowsToResult(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		result.ResultSet = rs
	}

	// 6. 读取 DBMS_OUTPUT（同一 session）。手动循环 GET_LINES 直到取空。
	output, err := fetchDBMSOutput(ctx, dbConn)
	if err != nil {
		return nil, fmt.Errorf("oracle: fetch DBMS_OUTPUT: %w", err)
	}
	result.Output = output

	return result, nil
}

// normalizeOutType 归一化 OUT 类型名。
func normalizeOutType(t string) string {
	switch strings.ToLower(t) {
	case "number", "num", "int", "integer", "float":
		return "number"
	case "cursor", "refcursor", "ref_cursor", "sys_refcursor":
		return "cursor"
	case "string", "str", "varchar2", "varchar", "text":
		return "string"
	case "":
		return "string" // 默认按字符串
	}
	return strings.ToLower(t)
}

// rowsToResult 把 *sql.Rows 读成通用 driver.Result（复用 conn.go 的扫描模式）。
func rowsToResult(rows *sql.Rows) (*driver.Result, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("columns: %w", err)
	}
	result := &driver.Result{Columns: cols}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		result.Rows = append(result.Rows, values)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return result, nil
}

// fetchDBMSOutput 在给定连接上循环调用 DBMS_OUTPUT.GET_LINES，返回全部打印文本。
//
// GET_LINES(:lines, :numlines)：
//   - :lines   —— OUT，VARCHAR2 数组，每元素一行
//   - :numlines —— IN OUT，请求取多少行 / 实际取回多少行；为 0 表示缓冲区已空
//
// 用 go_ora.Out 绑定数组（Size 为数组容量），循环直到 numlines == 0。
func fetchDBMSOutput(ctx context.Context, dbConn *sql.Conn) (string, error) {
	var sb strings.Builder
	const batchSize = 20 // 每轮取 20 行
	const lineLen = 400  // 单行缓冲长度
	for {
		lines := make([]string, batchSize)
		num := int64(batchSize)
		// GET_LINES 的 numlines 是 IN OUT：传入想取的行数，返回实际取到的行数。
		_, err := dbConn.ExecContext(ctx,
			"BEGIN DBMS_OUTPUT.GET_LINES(:1, :2); END;",
			go_ora.Out{Dest: &lines, Size: lineLen},
			go_ora.Out{Dest: &num, Size: batchSize, In: true},
		)
		if err != nil {
			// 部分场景（缓冲区未用等）可能报错，已取到的内容仍返回。
			if sb.Len() == 0 {
				return "", err
			}
			break
		}
		if num <= 0 {
			break
		}
		for i := int64(0); i < num; i++ {
			sb.WriteString(lines[i])
			sb.WriteByte('\n')
		}
		if num < int64(batchSize) {
			break // 取回少于请求量，说明缓冲区已空
		}
	}
	return sb.String(), nil
}
