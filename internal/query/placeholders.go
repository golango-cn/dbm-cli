// Package query 的占位符归一化子模块。
//
// 问题背景：不同数据库驱动对参数占位符的语法要求不一致：
//   - MySQL / ClickHouse：database/sql 标准 "?" 占位符
//   - PostgreSQL (pgx)：  "$1, $2, ..." 位置占位符
//   - Oracle (go-ora)：    ":1, :2, ..." 或 ":name"
//
// 让用户为每种引擎记忆不同占位符体验很差。本模块约定：用户在 SQL 中
// 统一使用 "?" 作为占位符，由 CLI 层按当前驱动的 PlaceholderStyle 转换为
// 引擎原生占位符后再下发。
//
// 注意：本转换只识别「裸 ?」（作为参数占位符），不处理字符串字面量/注释内
// 出现的 ?（例如 WHERE name='a?b'）。对于参数化查询场景这已足够——占位符
// 用于替换字面量，不应出现在字面量内部。若用户确需在字面量中包含 ?，请
// 直接用参数绑定（用占位符传入该值）。
package query

import "strings"

// PlaceholderStyle 描述一种数据库驱动的参数占位符风格。
type PlaceholderStyle int

const (
	// StyleQuestion 使用 "?" 占位符（MySQL、ClickHouse）。无需转换。
	StyleQuestion PlaceholderStyle = iota
	// StyleDollar 使用 "$1, $2, ..."（PostgreSQL）。
	StyleDollar
	// StyleColonNumeric 使用 ":1, :2, ..."（Oracle）。
	StyleColonNumeric
)

// PlaceholderStyleFor 按驱动类型名返回其占位符风格。
// 未知驱动默认 StyleQuestion（database/sql 的事实标准）。
func PlaceholderStyleFor(driverType string) PlaceholderStyle {
	switch strings.ToLower(driverType) {
	case "postgres", "postgresql", "pgx":
		return StyleDollar
	case "oracle":
		return StyleColonNumeric
	default:
		// mysql / mariadb / clickhouse 及未知驱动均用 ?
		return StyleQuestion
	}
}

// NormalizePlaceholders 把 SQL 中的 "?" 占位符按指定风格转换为引擎原生占位符。
// 仅当 style != StyleQuestion 时才发生替换；问号按出现顺序从 1 开始编号。
// 若 SQL 中不含 "?"，原样返回。
func NormalizePlaceholders(sql string, style PlaceholderStyle) string {
	if style == StyleQuestion {
		return sql
	}
	var b strings.Builder
	b.Grow(len(sql) + 16)
	n := 0
	for i := 0; i < len(sql); i++ {
		if sql[i] == '?' {
			n++
			switch style {
			case StyleDollar:
				// PostgreSQL: $1, $2, ...
				b.WriteByte('$')
				b.WriteString(itoa(n))
			case StyleColonNumeric:
				// Oracle: :1, :2, ...
				b.WriteByte(':')
				b.WriteString(itoa(n))
			}
			continue
		}
		b.WriteByte(sql[i])
	}
	return b.String()
}

// CountPlaceholders 统计 SQL 中 "?" 占位符的数量，用于校验参数个数是否匹配。
func CountPlaceholders(sql string) int {
	n := 0
	for i := 0; i < len(sql); i++ {
		if sql[i] == '?' {
			n++
		}
	}
	return n
}

// itoa 是 strconv.Itoa 的轻量内联实现，避免为热路径引入 strconv 导入开销。
// 仅用于占位符序号（正整数），数字位数有限。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
