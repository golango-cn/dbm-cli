// Package query 提供 SQL 语句的轻量分类工具。
//
// 注意：这是基于语句首关键字（leading keyword）的启发式判定，不依赖完整 SQL
// 解析器——足以支撑「读 vs 写」「是否需要二次确认」这类决策。它不保证对任意
// 复杂 SQL 的精确语义分析。
package query

import (
	"strings"
)

// Kind 描述 SQL 语句的类别。
type Kind int

const (
	KindUnknown Kind = iota
	KindSelect            // SELECT / WITH(CTE)
	KindDML               // INSERT / UPDATE / DELETE / MERGE
	KindDDL               // CREATE / ALTER / DROP / TRUNCATE / RENAME
	KindTransaction       // COMMIT / ROLLBACK / SAVEPOINT / SET TRANSACTION
)

// String 返回类别的人类可读名。
func (k Kind) String() string {
	switch k {
	case KindSelect:
		return "SELECT"
	case KindDML:
		return "DML"
	case KindDDL:
		return "DDL"
	case KindTransaction:
		return "TRANSACTION"
	default:
		return "UNKNOWN"
	}
}

// IsReadOnly 返回该类别是否只读（不会改变数据）。
func (k Kind) IsReadOnly() bool {
	return k == KindSelect
}

// Classify 根据 SQL 首关键字判定类别。
// 会去掉前导空白与 SQL 行注释（-- ...）的简单情况。
func Classify(sql string) Kind {
	kw := firstKeyword(sql)
	switch kw {
	case "SELECT", "WITH":
		return KindSelect
	case "INSERT", "UPDATE", "DELETE", "MERGE":
		return KindDML
	case "CREATE", "ALTER", "DROP", "TRUNCATE", "RENAME":
		return KindDDL
	case "COMMIT", "ROLLBACK", "SAVEPOINT", "SET":
		return KindTransaction
	}
	return KindUnknown
}

// IsDestructive 粗略判断语句是否属于「高风险、可能造成数据丢失」类别，
// 用于触发交互式二次确认（安全守卫）。
// 包含：DROP / TRUNCATE，以及无 WHERE 的 DELETE / UPDATE。
func IsDestructive(sql string) bool {
	kw := firstKeyword(sql)
	switch kw {
	case "DROP", "TRUNCATE":
		return true
	case "DELETE", "UPDATE":
		return !strings.Contains(strings.ToUpper(sql), " WHERE ")
	}
	return false
}

// firstKeyword 提取 SQL 第一个关键字（大写）。
func firstKeyword(sql string) string {
	s := strings.TrimSpace(sql)
	// 跳过前导行注释 "-- ..." 与块注释 "/* */"
	for {
		if strings.HasPrefix(s, "--") {
			if idx := strings.IndexByte(s, '\n'); idx >= 0 {
				s = strings.TrimSpace(s[idx+1:])
				continue
			}
			return ""
		}
		if strings.HasPrefix(s, "/*") {
			if idx := strings.Index(s, "*/"); idx >= 0 {
				s = strings.TrimSpace(s[idx+2:])
				continue
			}
			return ""
		}
		break
	}
	// 取第一个空白前的 token 作为关键字
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToUpper(fields[0])
}
