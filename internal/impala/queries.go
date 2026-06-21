package impala

import "fmt"

// 本文件集中放置 Impala 元数据查询。
//
// 重要：Impala 4.x 没有 INFORMATION_SCHEMA（与 ClickHouse/PostgreSQL 不同），
// 元数据必须用 SHOW / DESCRIBE 命令获取。这些命令的输出经 impala-go 驱动
// 可像普通 SELECT 结果集一样扫描。
//
// 概念映射：Impala 中「database」即 schema；表含 managed/external/view。
// 分页：Impala 支持 LIMIT 但不支持 OFFSET，故 QueryTable 仅用 LIMIT（不支持跳页）。
//
// 注意：comment 是 Impala 保留字，作为列名引用时必须用反引号包裹。

// databasesSQL 列出所有库。SHOW DATABASES 返回 (name, comment) 两列。
const databasesSQL = `SHOW DATABASES`

// buildTablesSQL 构造某库的表列表 SQL。
// SHOW TABLES IN <db> 返回单列 (name)。
func buildTablesSQL(db string) string {
	return "SHOW TABLES IN " + quoteIdent(db)
}

// buildDescribeSQL 构造 DESCRIBE 语句，返回 (name, type, comment) 三列。
// DESCRIBE 是获取列定义的标准方式。
func buildDescribeSQL(db, table string) string {
	return fmt.Sprintf("DESCRIBE %s.%s", quoteIdent(db), quoteIdent(table))
}

// indexesNotSupported：Impala 无传统索引概念，用 PARTITION BY / SORT BY 代替。
const indexesNotSupported = "Impala 不支持传统索引（用 PARTITIONED BY / SORTED BY 替代），请用 columns 查看"

// buildPagedSelectSQL：Impala 仅支持 LIMIT，不支持 OFFSET。
// 跳页（offset>0）会返回提示错误；offset=0 时正常取前 N 行。
func buildPagedSelectSQL(schema, table string, limit, offset int64, orderCol string) (string, error) {
	if offset > 0 {
		return "", fmt.Errorf("impala: 不支持 OFFSET 分页（Impala 仅支持 LIMIT）；请用 query 命令自定义 SQL")
	}
	base := fmt.Sprintf("SELECT * FROM %s.%s", quoteIdent(schema), quoteIdent(table))
	if orderCol != "" {
		base += " ORDER BY " + quoteIdent(orderCol)
	}
	return fmt.Sprintf("%s LIMIT %d", base, limit), nil
}

// buildCountSQL 生成精确行数 SQL。
func buildCountSQL(schema, table string) string {
	return fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", quoteIdent(schema), quoteIdent(table))
}

// quoteIdent 用反引号包裹标识符（Impala 兼容 Hive SQL，用反引号引用标识符）。
// comment 等保留字作为列名/库名时必须这样转义。
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
