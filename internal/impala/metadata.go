package impala

import (
	"context"
	"fmt"
	"strings"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// metadataProvider 实现 driver.MetadataProvider。
//
// Impala 无 INFORMATION_SCHEMA，元数据走 SHOW / DESCRIBE 命令：
//   - Databases: SHOW DATABASES          → (name, comment)
//   - Tables:    SHOW TABLES IN <db>     → (name)
//   - Columns:   DESCRIBE <db>.<table>   → (name, type, comment)
//   - Views:     SHOW TABLES IN <db> 后按 table_type 过滤（或 DESCRIBE 区分）
//
// 概念：Impala 中 database == schema；无传统主键/唯一索引概念。
type metadataProvider struct {
	conn *conn
}

// resolveDB 解析库名：传入非空用之；为空用配置的 database，再不行用 default。
func (m *metadataProvider) resolveDB(db string) (string, error) {
	if db != "" {
		return db, nil
	}
	if m.conn.cfg.Database != "" {
		return m.conn.cfg.Database, nil
	}
	return "default", nil
}

// Databases 列出所有库（含系统库，标注）。SHOW DATABASES 返回 name, comment。
func (m *metadataProvider) Databases(ctx context.Context) ([]string, error) {
	rows, err := m.conn.pool.QueryContext(ctx, databasesSQL)
	if err != nil {
		return nil, fmt.Errorf("impala: databases: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name, comment string
		if err := rows.Scan(&name, &comment); err != nil {
			return nil, err
		}
		// 过滤内置系统库
		if name == "_impala_builtins" {
			continue
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// Schemas 同 Databases（Impala 中 database == schema）。
func (m *metadataProvider) Schemas(ctx context.Context) ([]string, error) {
	return m.Databases(ctx)
}

// Tables 列出某库的表。SHOW TABLES IN <db> 返回单列 name。
func (m *metadataProvider) Tables(ctx context.Context, schema string) ([]driver.TableInfo, error) {
	schema, err := m.resolveDB(schema)
	if err != nil {
		return nil, err
	}
	rows, err := m.conn.pool.QueryContext(ctx, buildTablesSQL(schema))
	if err != nil {
		return nil, fmt.Errorf("impala: tables: %w", err)
	}
	defer rows.Close()
	var out []driver.TableInfo
	for rows.Next() {
		var t driver.TableInfo
		if err := rows.Scan(&t.Name); err != nil {
			return nil, err
		}
		t.Schema = schema
		t.Type = "TABLE"
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("impala: tables: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("impala: tables: no tables found in %s", schema)
	}
	return out, nil
}

// Columns 列出表列。DESCRIBE <db>.<table> 返回 (name, type, comment)。
func (m *metadataProvider) Columns(ctx context.Context, schema, table string) ([]driver.ColumnInfo, error) {
	if table == "" {
		return nil, fmt.Errorf("impala: columns: table name is required")
	}
	schema, err := m.resolveDB(schema)
	if err != nil {
		return nil, err
	}
	rows, err := m.conn.pool.QueryContext(ctx, buildDescribeSQL(schema, table))
	if err != nil {
		return nil, fmt.Errorf("impala: columns: %w", err)
	}
	defer rows.Close()
	var out []driver.ColumnInfo
	pos := 0
	for rows.Next() {
		var name, typ, comment string
		if err := rows.Scan(&name, &typ, &comment); err != nil {
			return nil, err
		}
		// DESCRIBE 输出里可能含分区信息行（# Partition information 段），跳过。
		if strings.HasPrefix(name, "#") || name == "" {
			continue
		}
		pos++
		out = append(out, driver.ColumnInfo{
			Name:     name,
			DataType: typ,
			Comment:  comment,
			Position: pos,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("impala: columns: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("impala: columns: no columns found for %s.%s (table may not exist)", schema, table)
	}
	return out, nil
}

// Indexes：Impala 无传统索引，返回说明性错误。
func (m *metadataProvider) Indexes(ctx context.Context, schema, table string) ([]driver.IndexInfo, error) {
	return nil, fmt.Errorf("impala: %s", indexesNotSupported)
}

// Views：Impala 视图也出现在 SHOW TABLES 中，但 SHOW TABLES 不区分类型。
// 这里返回空列表（不报错），用户可用 DESCRIBE 查看是否为视图。
func (m *metadataProvider) Views(ctx context.Context, schema string) ([]driver.ViewInfo, error) {
	return nil, nil
}

// RowCount：Impala 用精确 COUNT（无统计估算）。
func (m *metadataProvider) RowCount(ctx context.Context, schema, table string) (int64, error) {
	if table == "" {
		return 0, fmt.Errorf("impala: row count: table name is required")
	}
	schema, err := m.resolveDB(schema)
	if err != nil {
		return 0, err
	}
	var cnt int64
	if err := m.conn.pool.QueryRowContext(ctx, buildCountSQL(schema, table)).Scan(&cnt); err != nil {
		return 0, fmt.Errorf("impala: row count: %w", err)
	}
	return cnt, nil
}
