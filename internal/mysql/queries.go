package mysql

import "fmt"

// 本文件集中放置 MySQL 元数据字典查询。
//
// 多版本兼容策略：MySQL 5.6 / 5.7 / 8.0 的 information_schema 结构高度一致，
// 因此本驱动的元数据查询对各版本通用，无需按版本分支（与 Oracle 不同）。
// 仅在极少数 8.0 特性（如不可见列 invisible columns）上做轻量适配。
//
// 所有涉及用户输入的标识符（schema/table）都用反引号转义（quoteIdent）防注入。

// databasesSQL 列出所有数据库（schema）。
// 从 information_schema.schemata 读取，所有版本可用。
const databasesSQL = `
SELECT schema_name
FROM information_schema.schemata
WHERE schema_name NOT IN ('information_schema','performance_schema','mysql','sys')
ORDER BY schema_name`

// tablesSQL 列出某 schema 下的表（含类型与注释）。
const tablesSQL = `
SELECT table_schema, table_name, table_type, IFNULL(table_comment, '')
FROM information_schema.tables
WHERE table_schema = ?
ORDER BY table_name`

// tablesLikeSQL 支持 LIKE 模糊匹配表名。
const tablesLikeSQL = `
SELECT table_schema, table_name, table_type, IFNULL(table_comment, '')
FROM information_schema.tables
WHERE table_schema = ? AND table_name LIKE ?
ORDER BY table_name`

// columnsSQL 列出某表的列定义。
// extra 字段含 auto_increment 等信息，一并提供。
const columnsSQL = `
SELECT column_name, data_type, character_maximum_length, numeric_precision, numeric_scale,
       is_nullable, IFNULL(column_default, ''), ordinal_position, IFNULL(column_comment, ''), extra
FROM information_schema.columns
WHERE table_schema = ? AND table_name = ?
ORDER BY ordinal_position`

// indexesSQL 列出某表的索引信息（含列与唯一性）。
// 多列索引会有多行，由 metadata.go 按 index_name 聚合。
const indexesSQL = `
SELECT index_name, IF(non_unique=0,1,0), column_name, seq_in_index
FROM information_schema.statistics
WHERE table_schema = ? AND table_name = ?
ORDER BY index_name, seq_in_index`

// primaryIndexNameSQL 取主键索引名。
const primaryIndexNameSQL = `
SELECT index_name
FROM information_schema.statistics
WHERE table_schema = ? AND table_name = ? AND index_name = 'PRIMARY'
LIMIT 1`

// viewsSQL 列出某 schema 下的视图。
const viewsSQL = `
SELECT table_schema, table_name
FROM information_schema.views
WHERE table_schema = ?
ORDER BY table_name`

// rowCountSQL 取近似行数（来自统计值）。
// information_schema.tables.table_rows 在 InnoDB 下是估算值（不精确但快速）。
const rowCountSQL = `
SELECT IFNULL(table_rows, 0)
FROM information_schema.tables
WHERE table_schema = ? AND table_name = ?`

// versionSQL 取版本号。
// VERSION() 在所有 MySQL 版本可用，返回如 "8.0.37"、"5.7.43-log"。
const versionSQL = `SELECT VERSION()`

// buildPagedSelectSQL 用 MySQL 的 LIMIT/OFFSET 实现分页（Builder 模式）。
// LIMIT/OFFSET 从 MySQL 4.1 起即支持，覆盖全部目标版本。
func buildPagedSelectSQL(schema, table string, limit, offset int64, orderCol string) string {
	base := fmt.Sprintf("SELECT * FROM %s.%s", quoteIdent(schema), quoteIdent(table))
	if orderCol != "" {
		base += " ORDER BY " + quoteIdent(orderCol)
	}
	return fmt.Sprintf("%s LIMIT %d OFFSET %d", base, limit, offset)
}

// buildCountSQL 生成精确行数 SQL。
func buildCountSQL(schema, table string) string {
	return fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", quoteIdent(schema), quoteIdent(table))
}

// quoteIdent 用反引号包裹标识符，避免保留字冲突与注入。
// 内部反引号转义为两个反引号。
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
