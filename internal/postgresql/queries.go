package postgresql

import (
	"fmt"
	"strconv"
)

// 本文件集中放置 PostgreSQL 元数据字典查询。
//
// 多版本兼容：information_schema 与 pg_catalog 在 pg 9.6+ 高度稳定，
// 本驱动统一走这些视图，覆盖 pg 12 / 17。
//
// 概念映射：PostgreSQL 中「database」是独立概念，「schema」是库内的命名空间
// （默认 public）。因此 Databases() 列库，Schemas() 列库内 schema，
// Tables() 等需要 schema（默认 public）。

// databasesSQL 列出当前实例下所有库（pg_database）。
const databasesSQL = `
SELECT datname FROM pg_database
WHERE datistemplate = false AND datname NOT IN ('postgres')
ORDER BY datname`

// schemasSQL 列出当前库下的 schema（information_schema.schemata）。
const schemasSQL = `
SELECT schema_name FROM information_schema.schemata
WHERE schema_name NOT IN ('information_schema','pg_catalog','pg_toast')
  AND schema_name NOT LIKE 'pg_temp_%' AND schema_name NOT LIKE 'pg_toast_temp_%'
ORDER BY schema_name`

// tablesSQL 列出某 schema 的表（含类型与注释）。
// pg_obj_description 取表注释。
const tablesSQL = `
SELECT
  c.relname,
  CASE c.relkind WHEN 'r' THEN 'TABLE' WHEN 'v' THEN 'VIEW'
                 WHEN 'm' THEN 'MATERIALIZED VIEW' WHEN 'p' THEN 'PARTITIONED TABLE'
                 ELSE c.relkind::text END,
  COALESCE(obj_description(c.oid), '')
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1 AND c.relkind IN ('r','v','m','p')
ORDER BY c.relname`

// columnsSQL 列出某表的列。information_schema.columns 跨版本稳定。
const columnsSQL = `
SELECT column_name, data_type, character_maximum_length, numeric_precision, numeric_scale,
       is_nullable, COALESCE(column_default, ''), ordinal_position, COALESCE(col_description((table_schema||'.'||table_name)::regclass, ordinal_position), '')
FROM information_schema.columns
WHERE table_schema = $1 AND table_name = $2
ORDER BY ordinal_position`

// indexesSQL 列出某表的索引（聚合列）。
// pg_indexes.indexdef 包含完整定义，但为统一结构这里从 pg_index 拆解。
const indexesSQL = `
SELECT i.relname AS index_name,
       ix.indisunique, ix.indisprimary,
       array_agg(a.attname ORDER BY array_position(ix.indkey, a.attnum)) AS columns
FROM pg_index ix
JOIN pg_class t ON t.oid = ix.indrelid
JOIN pg_class i ON i.oid = ix.indexrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
WHERE n.nspname = $1 AND t.relname = $2
GROUP BY i.relname, ix.indisunique, ix.indisprimary, i.oid
ORDER BY ix.indisprimary DESC, i.relname`

// viewsSQL 列出某 schema 的视图。
const viewsSQL = `
SELECT c.relname
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1 AND c.relkind = 'v'
ORDER BY c.relname`

// rowCountSQL 精确行数（PostgreSQL 无统计估算，用 count）。
func buildCountSQL(schema, table string) string {
	return fmt.Sprintf("SELECT count(*) FROM %s.%s", quoteIdent(schema), quoteIdent(table))
}

// buildPagedSelectSQL 用 PostgreSQL 的 LIMIT/OFFSET 分页（Builder 模式）。
func buildPagedSelectSQL(schema, table string, limit, offset int64, orderCol string) string {
	base := fmt.Sprintf("SELECT * FROM %s.%s", quoteIdent(schema), quoteIdent(table))
	if orderCol != "" {
		base += " ORDER BY " + quoteIdent(orderCol)
	}
	return base + " LIMIT " + strconv.FormatInt(limit, 10) + " OFFSET " + strconv.FormatInt(offset, 10)
}

// quoteIdent 用双引号包裹标识符（PostgreSQL 标准引用方式）。
func quoteIdent(name string) string {
	out := make([]byte, 0, len(name)+2)
	out = append(out, '"')
	for i := 0; i < len(name); i++ {
		if name[i] == '"' {
			out = append(out, '"', '"')
		} else {
			out = append(out, name[i])
		}
	}
	out = append(out, '"')
	return string(out)
}
