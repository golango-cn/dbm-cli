package clickhouse

import (
	"context"
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// metadataProvider 实现 driver.MetadataProvider。
//
// ClickHouse 概念说明：
//   - "database" 即其它库的 schema/database。
//   - "table" 含普通表、视图、物化视图，由 engine 区分。
//   - 没有传统主键/唯一索引；用 ORDER BY / PRIMARY KEY（排序键）和跳数索引替代。
//     本实现的 Indexes() 把排序键的首列标记为主键，跳数索引作为普通索引列出。
type metadataProvider struct {
	conn *conn
}

// resolveDB 解析库名：传入非空则用之；为空则用连接配置的 database。
func (m *metadataProvider) resolveDB(db string) (string, error) {
	if db != "" {
		return db, nil
	}
	if m.conn.cfg.Database != "" {
		return m.conn.cfg.Database, nil
	}
	return "", fmt.Errorf("clickhouse: no database selected; please specify --schema or set 'database' in config")
}

// Databases 列出所有用户库（排除系统库）。
func (m *metadataProvider) Databases(ctx context.Context) ([]string, error) {
	rows, err := m.conn.pool.QueryContext(ctx, databasesSQL)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: databases: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// Schemas 同 Databases（ClickHouse 中 database == schema）。
func (m *metadataProvider) Schemas(ctx context.Context) ([]string, error) {
	return m.Databases(ctx)
}

// Tables 列出某库的表（含引擎类型与注释）。
func (m *metadataProvider) Tables(ctx context.Context, schema string) ([]driver.TableInfo, error) {
	schema, err := m.resolveDB(schema)
	if err != nil {
		return nil, err
	}
	rows, err := m.conn.pool.QueryContext(ctx, tablesSQL, schema)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: tables: %w", err)
	}
	defer rows.Close()
	var out []driver.TableInfo
	for rows.Next() {
		var t driver.TableInfo
		if err := rows.Scan(&t.Schema, &t.Name, &t.Type, &t.Comment); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse: tables: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("clickhouse: tables: no tables found in %s (database may not exist)", schema)
	}
	return out, nil
}

// Columns 列出表列。ClickHouse 列类型是完整类型串（如 Nullable(String)、Array(Int32)），
// 不拆分 length/precision（直接放 DataType）。
func (m *metadataProvider) Columns(ctx context.Context, schema, table string) ([]driver.ColumnInfo, error) {
	if table == "" {
		return nil, fmt.Errorf("clickhouse: columns: table name is required")
	}
	schema, err := m.resolveDB(schema)
	if err != nil {
		return nil, err
	}
	rows, err := m.conn.pool.QueryContext(ctx, columnsSQL, schema, table)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: columns: %w", err)
	}
	defer rows.Close()
	var out []driver.ColumnInfo
	for rows.Next() {
		var c driver.ColumnInfo
		var defaultKind, defaultExpr string
		if err := rows.Scan(&c.Name, &c.DataType, &c.Position, &defaultKind, &defaultExpr, &c.Comment); err != nil {
			return nil, err
		}
		// default_kind 为空表示无默认值；否则用 "kind: expression" 表示
		if defaultKind != "" && defaultKind != "DEFAULT" {
			c.DefaultValue = defaultKind + ":" + defaultExpr
		} else if defaultExpr != "" {
			c.DefaultValue = defaultExpr
		}
		// ClickHouse 列几乎都可为 NULL（由类型决定），这里粗略判断类型名是否含 Nullable
		c.Nullable = containsNullable(c.DataType)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse: columns: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("clickhouse: columns: no columns found for %s.%s (table may not exist)", schema, table)
	}
	return out, nil
}

// Indexes 列出表的「索引」。
// ClickHouse 无传统索引，这里把主键/排序键首列标记为主键，跳数索引作为普通索引。
func (m *metadataProvider) Indexes(ctx context.Context, schema, table string) ([]driver.IndexInfo, error) {
	if table == "" {
		return nil, fmt.Errorf("clickhouse: indexes: table name is required")
	}
	schema, err := m.resolveDB(schema)
	if err != nil {
		return nil, err
	}
	// 主键/排序键首列
	var pkCol string
	_ = m.conn.pool.QueryRowContext(ctx, primaryKeySQL, schema, table).Scan(&pkCol)

	var out []driver.IndexInfo
	if pkCol != "" {
		out = append(out, driver.IndexInfo{
			Name:      "PRIMARY",
			IsPrimary: true,
			Columns:   []string{pkCol},
		})
	}
	// 跳数索引（二级索引）
	rows, err := m.conn.pool.QueryContext(ctx, dataSkippingIndicesSQL, schema, table)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, typ, expr string
			if err := rows.Scan(&name, &typ, &expr); err != nil {
				return nil, err
			}
			out = append(out, driver.IndexInfo{
				Name:    name,
				Columns: []string{expr + " (" + typ + ")"},
			})
		}
	}
	return out, nil
}

// Views 列出库中的视图（含物化视图）。
func (m *metadataProvider) Views(ctx context.Context, schema string) ([]driver.ViewInfo, error) {
	schema, err := m.resolveDB(schema)
	if err != nil {
		return nil, err
	}
	rows, err := m.conn.pool.QueryContext(ctx, viewsSQL, schema)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: views: %w", err)
	}
	defer rows.Close()
	var out []driver.ViewInfo
	for rows.Next() {
		var v driver.ViewInfo
		if err := rows.Scan(&v.Schema, &v.Name); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse: views: %w", err)
	}
	return out, nil
}

// RowCount 取行数。ClickHouse 的 system.tables.total_rows 是实时估算（无需 ANALYZE），较准。
func (m *metadataProvider) RowCount(ctx context.Context, schema, table string) (int64, error) {
	if table == "" {
		return 0, fmt.Errorf("clickhouse: row count: table name is required")
	}
	schema, err := m.resolveDB(schema)
	if err != nil {
		return 0, err
	}
	var cnt int64
	if qerr := m.conn.pool.QueryRowContext(ctx, rowCountSQL, schema, table).Scan(&cnt); qerr == nil && cnt > 0 {
		return cnt, nil
	}
	if err := m.conn.pool.QueryRowContext(ctx, buildCountSQL(schema, table)).Scan(&cnt); err != nil {
		return 0, fmt.Errorf("clickhouse: row count: %w", err)
	}
	return cnt, nil
}

// containsNullable 判断 ClickHouse 类型串是否为 Nullable(...)。
func containsNullable(typeStr string) bool {
	return len(typeStr) >= 9 && typeStr[:9] == "Nullable("
}

// （未使用占位，保留 orderByColsSQL 以备后续扩展排序键完整展示）
var _ = orderByColsSQL
