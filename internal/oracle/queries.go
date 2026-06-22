package oracle

import (
	"fmt"
	"strings"
)

// 本文件集中放置 Oracle 元数据字典查询。
//
// 多版本兼容策略：
//   - 10g/11g 用 all_* 字典视图（所有版本通用）
//   - 12c+（CDB 架构）可选用 cdb_* / dba_pdbs，但 all_* 始终可用
//   - 默认全部走 all_* 视图（当前用户可见范围），保证跨版本一致性
//   - 受限账号若连 all_* 都不全，查询会返回部分结果或空（不报错）

// schemasSQL 列出当前用户可见的 schema（user）。
// all_users 在所有 Oracle 版本均存在，是最稳妥的选择。
const schemasSQL = `
SELECT username
FROM all_users
ORDER BY username`

// tablesSQL 列出某 schema 下的表（含类型）。
// owner 用绑定变量传入，避免注入。
const tablesSQL = `
SELECT t.owner AS schema_name,
       t.table_name,
       NVL(t.iot_type, 'TABLE') AS table_type,
       c.comments
FROM all_tables t
LEFT JOIN all_tab_comments c
  ON c.owner = t.owner AND c.table_name = t.table_name
WHERE t.owner = :1
ORDER BY t.table_name`

// tablesLikeSQL 支持 like 模糊匹配表名。
const tablesLikeSQL = `
SELECT t.owner AS schema_name,
       t.table_name,
       NVL(t.iot_type, 'TABLE') AS table_type,
       c.comments
FROM all_tables t
LEFT JOIN all_tab_comments c
  ON c.owner = t.owner AND c.table_name = t.table_name
WHERE t.owner = :1 AND t.table_name LIKE :2
ORDER BY t.table_name`

// columnsSQL 列出某表的列定义。
const columnsSQL = `
SELECT c.column_name,
       c.data_type,
       NVL(c.data_length, 0),
       NVL(c.data_precision, 0),
       NVL(c.data_scale, 0),
       CASE c.nullable WHEN 'Y' THEN 1 ELSE 0 END,
       NVL(c.data_default, ''),
       c.column_id,
       NVL(cc.comments, '')
FROM all_tab_columns c
LEFT JOIN all_col_comments cc
  ON cc.owner = c.owner AND cc.table_name = c.table_name AND cc.column_name = c.column_name
WHERE c.owner = :1 AND c.table_name = :2
ORDER BY c.column_id`

// indexesSQL 列出某表的索引及其列。
// 用两段查询：先取索引信息，再按需拼接列（在 metadata.go 中聚合）。
const indexesSQL = `
SELECT i.index_name,
       CASE i.uniqueness WHEN 'UNIQUE' THEN 1 ELSE 0 END,
       CASE WHEN i.index_type = 'IOT - TOP' THEN 1 ELSE 0 END,
       ic.column_name,
       ic.column_position
FROM all_indexes i
JOIN all_ind_columns ic
  ON ic.index_owner = i.owner AND ic.index_name = i.index_name
WHERE i.table_owner = :1 AND i.table_name = :2
ORDER BY i.index_name, ic.column_position`

// primaryIndexNameSQL 取主键索引名（用于标记 is_primary）。
const primaryIndexNameSQL = `
SELECT c.index_name
FROM all_constraints c
WHERE c.owner = :1 AND c.table_name = :2 AND c.constraint_type = 'P'`

// viewsSQL 列出某 schema 下的视图。
const viewsSQL = `
SELECT owner, view_name
FROM all_views
WHERE owner = :1
ORDER BY view_name`

// rowCountSQL 取近似行数。
// 优先用 all_tables.num_rows（统计值，瞬时且不锁表），
// 若统计未收集会返回 0，调用方可再走精确 COUNT。
const rowCountSQL = `
SELECT NVL(num_rows, 0)
FROM all_tables
WHERE owner = :1 AND table_name = :2`

// databasesPDBsSQL 仅 12c+ 可用：列出 CDB 中所有 PDB。
const databasesPDBsSQL = `
SELECT name FROM v$pdbs ORDER BY name`

// versionBannerSQL 用于版本探测。
const versionBannerSQL = `SELECT banner FROM v$version WHERE banner LIKE 'Oracle%'`

// buildPagedSelectSQL 用 Oracle 的 ROWNUM 包装实现分页（Builder 模式）。
// Oracle 12c+ 也可用 OFFSET/FETCH，但 ROWNUM 全版本兼容，故统一采用。
//
// 三层结构保证：结果集只含原表列（不暴露辅助的 rnum 列），同时实现 offset/limit：
//
//	SELECT * FROM (                       <- 最外层：丢弃 rnum，应用 offset 下界
//	  SELECT t.*, ROWNUM AS rnum FROM (    <- 中间层：打行号、应用 limit 上界
//	    <base query>                       <- 最内层：原查询 + 可选 ORDER BY
//	  ) t WHERE ROWNUM <= offset+limit
//	) WHERE rnum > offset
//
// schema/table 已由调用方校验，这里直接拼接（经 quoteIdent 转义）。
func buildPagedSelectSQL(schema, table string, limit, offset int64, orderCol string) string {
	base := fmt.Sprintf("SELECT * FROM %s.%s", quoteIdent(schema), quoteIdent(table))
	if orderCol != "" {
		base += " ORDER BY " + quoteIdent(orderCol)
	}
	upper := offset + limit
	return fmt.Sprintf(
		"SELECT * FROM (SELECT t.*, ROWNUM AS rnum FROM (%s) t WHERE ROWNUM <= %d) WHERE rnum > %d",
		base, upper, offset)
}

// buildCountSQL 生成精确行数 SQL（当字典统计值不可信时回退）。
func buildCountSQL(schema, table string) string {
	return fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", quoteIdent(schema), quoteIdent(table))
}

// quoteIdent 把标识符用双引号包裹，避免保留字冲突与注入。
// Oracle 把未加引号的标识符默认存为大写，因此这里先转大写再包裹，
// 使大小写不敏感的表名输入（如 dbm_demo）能匹配大写存储（DBM_DEMO）。
// 内部的双引号需转义为两个双引号。
func quoteIdent(name string) string {
	escaped := ""
	for _, r := range strings.ToUpper(name) {
		if r == '"' {
			escaped += `""`
		} else {
			escaped += string(r)
		}
	}
	return `"` + escaped + `"`
}
