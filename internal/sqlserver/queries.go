package sqlserver

import (
	"fmt"
	"strconv"
)

// 本文件集中放置 SQL Server 元数据字典查询。
//
// 多版本兼容：sys.* 系统目录视图在 SQL Server 2012+ 高度稳定，
// 覆盖 2017 / 2022。分页用 OFFSET ... FETCH（2012+ 支持）。
//
// 概念映射：SQL Server 中「database」是独立概念，「schema」是库内的命名空间
// （默认 dbo）。Tables 等需要 schema（默认 dbo）。

// databasesSQL 列出实例下所有用户库。
const databasesSQL = `SELECT name FROM sys.databases
WHERE name NOT IN ('master','tempdb','model','msdb') ORDER BY name`

// schemasSQL 列出当前库的 schema。
const schemasSQL = `SELECT name FROM sys.schemas
WHERE name NOT IN ('guest','INFORMATION_SCHEMA','sys') ORDER BY name`

// tablesSQL 列出某 schema 的表。
const tablesSQL = `SELECT t.name,
  CASE t.type WHEN 'U' THEN 'TABLE' WHEN 'V' THEN 'VIEW' ELSE t.type_desc END,
  ISNULL(ep.value,'')
FROM sys.tables t
LEFT JOIN sys.extended_properties ep ON ep.major_id = t.object_id AND ep.minor_id = 0 AND ep.name = 'MS_Description'
INNER JOIN sys.schemas s ON s.schema_id = t.schema_id
WHERE s.name = @p1 ORDER BY t.name`

// columnsSQL 列出某表的列。
const columnsSQL = `SELECT c.name, tp.name, c.max_length, c.precision, c.scale,
  CASE c.is_nullable WHEN 1 THEN 'YES' ELSE 'NO' END,
  ISNULL(OBJECT_DEFINITION(c.default_object_id),''), c.column_id,
  ISNULL(ep.value,'')
FROM sys.columns c
JOIN sys.types tp ON tp.user_type_id = c.user_type_id
LEFT JOIN sys.extended_properties ep ON ep.major_id = c.object_id AND ep.minor_id = c.column_id AND ep.name = 'MS_Description'
WHERE c.object_id = OBJECT_ID(@p1) ORDER BY c.column_id`

// indexesSQL 列出某表的索引（聚合列）。
const indexesSQL = `SELECT i.name,
  CASE i.is_unique WHEN 1 THEN 1 ELSE 0 END,
  CASE i.is_primary_key WHEN 1 THEN 1 ELSE 0 END,
  STUFF((SELECT ',' + col_name(ic.object_id, ic.column_id)
         FROM sys.index_columns ic WHERE ic.object_id = i.object_id AND ic.index_id = i.index_id
         ORDER BY ic.key_ordinal FOR XML PATH('')), 1, 1, '') AS cols
FROM sys.indexes i
WHERE i.object_id = OBJECT_ID(@p1) AND i.name IS NOT NULL
ORDER BY i.is_primary_key DESC, i.name`

// viewsSQL 列出某 schema 的视图。
const viewsSQL = `SELECT v.name FROM sys.views v
INNER JOIN sys.schemas s ON s.schema_id = v.schema_id
WHERE s.name = @p1 ORDER BY v.name`

// rowCountSQL 取近似行数（sys.partitions 的 rows 统计值，无需全表扫描）。
const rowCountSQL = `SELECT SUM(p.rows) FROM sys.partitions p
WHERE p.object_id = OBJECT_ID(@p1) AND p.index_id IN (0,1)`

// buildPagedSelectSQL 用 OFFSET ... FETCH 分页（SQL Server 2012+）。
func buildPagedSelectSQL(schema, table string, limit, offset int64, orderCol string) (string, error) {
	base := fmt.Sprintf("SELECT * FROM %s.%s", quoteIdent(schema), quoteIdent(table))
	if orderCol != "" {
		base += " ORDER BY " + quoteIdent(orderCol)
	} else {
		// OFFSET/FETCH 要求 ORDER BY；无指定列时用 (SELECT 0) 占位以允许 OFFSET。
		base += " ORDER BY (SELECT 0)"
	}
	return base + " OFFSET " + strconv.FormatInt(offset, 10) + " ROWS FETCH NEXT " + strconv.FormatInt(limit, 10) + " ROWS ONLY", nil
}

// buildCountSQL 生成精确行数 SQL。
func buildCountSQL(schema, table string) string {
	return fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", quoteIdent(schema), quoteIdent(table))
}

// quoteIdent 用方括号包裹标识符（SQL Server 标准引用方式），内部 ] 转义为 ]]。
func quoteIdent(name string) string {
	out := make([]byte, 0, len(name)+2)
	out = append(out, '[')
	for i := 0; i < len(name); i++ {
		if name[i] == ']' {
			out = append(out, ']', ']')
		} else {
			out = append(out, name[i])
		}
	}
	out = append(out, ']')
	return string(out)
}
