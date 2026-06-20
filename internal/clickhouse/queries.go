package clickhouse

import "fmt"

// 本文件集中放置 ClickHouse 元数据字典查询。
//
// 多版本兼容策略：ClickHouse 的 system.* 表在各版本高度稳定，
// 因此元数据查询对各版本通用。information_schema 在新版也有，
// 但 system.* 兼容性更好，本驱动统一走 system.*。
//
// 概念映射：ClickHouse 的「database」≈ 其它库的 schema/database，
// 「table」包括普通表、视图、物化视图等，由 engine 字段区分类型。

// databasesSQL 列出所有用户库（排除系统库）。
const databasesSQL = `
SELECT name FROM system.databases
WHERE name NOT IN ('INFORMATION_SCHEMA','information_schema','system','default','_temporary_and_external_tables')
ORDER BY name`

// tablesSQL 列出某库下的表（含引擎与注释）。
const tablesSQL = `
SELECT database, name, engine, IFNULL(comment, '')
FROM system.tables
WHERE database = ?
ORDER BY name`

// columnsSQL 列出某表的列定义。
// system.columns 包含类型、默认值、注释、位置等。
const columnsSQL = `
SELECT name, type, position, default_kind, IFNULL(default_expression,''), IFNULL(comment,'')
FROM system.columns
WHERE database = ? AND table = ?
ORDER BY position`

// indexesSQL：ClickHouse 没有传统意义的主键/唯一索引概念。
// 它的「索引」是 MergeTree 的排序键(order by)、主键(primary key)、跳数索引(skip index)。
// 这里返回排序键/主键列，以及跳数索引，用统一结构表示。
const orderByColsSQL = `
SELECT name FROM system.columns
WHERE database = ? AND table = ?
  AND position IN (SELECT col_position FROM system.data_skipping_indices WHERE database=? AND table=?)
ORDER BY position`

// primaryKeySQL 取主键/排序键列（ClickHouse 的 primary key 通常等于 order by）。
const primaryKeySQL = `
SELECT name FROM system.columns
WHERE database = ? AND table = ?
ORDER BY position
LIMIT 1`

// dataSkippingIndicesSQL 取跳数索引（二级索引）。
const dataSkippingIndicesSQL = `
SELECT name, type, expr FROM system.data_skipping_indices
WHERE database = ? AND table = ?`

// viewsSQL 列出某库下的视图（含物化视图，按 engine 判断）。
const viewsSQL = `
SELECT database, name FROM system.tables
WHERE database = ? AND engine LIKE '%View'
ORDER BY name`

// rowCountSQL：ClickHouse 用 system.tables 的 total_rows（实时估算，无需 ANALYZE）。
const rowCountSQL = `
SELECT IFNULL(total_rows, 0) FROM system.tables
WHERE database = ? AND name = ?`

// versionSQL 取版本。
const versionSQL = `SELECT version()`

// buildPagedSelectSQL 用 ClickHouse 的 LIMIT/OFFSET 分页（Builder 模式）。
// ClickHouse 的 LIMIT offset, count 与 LIMIT count OFFSET offset 都支持；
// 这里用后者，与 MySQL 一致更直观。
func buildPagedSelectSQL(db, table string, limit, offset int64, orderCol string) string {
	base := fmt.Sprintf("SELECT * FROM %s.%s", quoteIdent(db), quoteIdent(table))
	if orderCol != "" {
		base += " ORDER BY " + quoteIdent(orderCol)
	}
	return fmt.Sprintf("%s LIMIT %d OFFSET %d", base, limit, offset)
}

// buildCountSQL 生成精确行数 SQL。
func buildCountSQL(db, table string) string {
	return fmt.Sprintf("SELECT count() FROM %s.%s", quoteIdent(db), quoteIdent(table))
}

// quoteIdent 用反引号包裹 ClickHouse 标识符。
// ClickHouse 支持反引号引用标识符（与 MySQL 类似）。
func quoteIdent(name string) string {
	out := make([]byte, 0, len(name)+2)
	out = append(out, '`')
	for i := 0; i < len(name); i++ {
		if name[i] == '`' {
			out = append(out, '`', '`')
		} else {
			out = append(out, name[i])
		}
	}
	out = append(out, '`')
	return string(out)
}
