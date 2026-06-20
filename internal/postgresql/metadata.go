package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// metadataProvider 实现 driver.MetadataProvider。
//
// PostgreSQL 概念：database（库）是顶层概念，schema 是库内命名空间（默认 public）。
// 因此 Tables/Columns/Indexes/Views 的 schema 参数默认为 "public"。
type metadataProvider struct {
	conn *conn
}

// resolveSchema：传入非空用之；为空默认 public。
func (m *metadataProvider) resolveSchema(schema string) string {
	if schema != "" {
		return schema
	}
	return "public"
}

// Databases 列出实例下所有用户库。
func (m *metadataProvider) Databases(ctx context.Context) ([]string, error) {
	rows, err := m.conn.pool.QueryContext(ctx, databasesSQL)
	if err != nil {
		return nil, fmt.Errorf("postgresql: databases: %w", err)
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

// Schemas 列出当前库下的 schema。
func (m *metadataProvider) Schemas(ctx context.Context) ([]string, error) {
	rows, err := m.conn.pool.QueryContext(ctx, schemasSQL)
	if err != nil {
		return nil, fmt.Errorf("postgresql: schemas: %w", err)
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

// Tables 列出某 schema 的表。
func (m *metadataProvider) Tables(ctx context.Context, schema string) ([]driver.TableInfo, error) {
	schema = m.resolveSchema(schema)
	rows, err := m.conn.pool.QueryContext(ctx, tablesSQL, schema)
	if err != nil {
		return nil, fmt.Errorf("postgresql: tables: %w", err)
	}
	defer rows.Close()
	var out []driver.TableInfo
	for rows.Next() {
		var t driver.TableInfo
		if err := rows.Scan(&t.Name, &t.Type, &t.Comment); err != nil {
			return nil, err
		}
		t.Schema = schema
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgresql: tables: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("postgresql: tables: no tables found in schema %s", schema)
	}
	return out, nil
}

// Columns 列出表列。
func (m *metadataProvider) Columns(ctx context.Context, schema, table string) ([]driver.ColumnInfo, error) {
	if table == "" {
		return nil, fmt.Errorf("postgresql: columns: table name is required")
	}
	schema = m.resolveSchema(schema)
	rows, err := m.conn.pool.QueryContext(ctx, columnsSQL, schema, table)
	if err != nil {
		return nil, fmt.Errorf("postgresql: columns: %w", err)
	}
	defer rows.Close()
	var out []driver.ColumnInfo
	for rows.Next() {
		var c driver.ColumnInfo
		var length, precision, scale sql.NullInt64
		var isNullable string
		if err := rows.Scan(&c.Name, &c.DataType, &length, &precision, &scale,
			&isNullable, &c.DefaultValue, &c.Position, &c.Comment); err != nil {
			return nil, err
		}
		c.Length = length.Int64
		c.Precision = int(precision.Int64)
		c.Scale = int(scale.Int64)
		c.Nullable = isNullable == "YES"
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgresql: columns: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("postgresql: columns: no columns found for %s.%s (table may not exist)", schema, table)
	}
	return out, nil
}

// Indexes 列出表索引。pg 的聚合列来自 array_agg。
func (m *metadataProvider) Indexes(ctx context.Context, schema, table string) ([]driver.IndexInfo, error) {
	if table == "" {
		return nil, fmt.Errorf("postgresql: indexes: table name is required")
	}
	schema = m.resolveSchema(schema)
	rows, err := m.conn.pool.QueryContext(ctx, indexesSQL, schema, table)
	if err != nil {
		return nil, fmt.Errorf("postgresql: indexes: %w", err)
	}
	defer rows.Close()
	var out []driver.IndexInfo
	for rows.Next() {
		var idx driver.IndexInfo
		var cols string // pg array_agg 经 pgx 返回形如 {col1,col2} 或字符串
		var isUnique, isPrimary bool
		if err := rows.Scan(&idx.Name, &isUnique, &isPrimary, &cols); err != nil {
			return nil, err
		}
		idx.IsUnique = isUnique
		idx.IsPrimary = isPrimary
		idx.Columns = parsePGArray(cols)
		out = append(out, idx)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgresql: indexes: %w", err)
	}
	return out, nil
}

// Views 列出 schema 的视图。
func (m *metadataProvider) Views(ctx context.Context, schema string) ([]driver.ViewInfo, error) {
	schema = m.resolveSchema(schema)
	rows, err := m.conn.pool.QueryContext(ctx, viewsSQL, schema)
	if err != nil {
		return nil, fmt.Errorf("postgresql: views: %w", err)
	}
	defer rows.Close()
	var out []driver.ViewInfo
	for rows.Next() {
		var v driver.ViewInfo
		if err := rows.Scan(&v.Name); err != nil {
			return nil, err
		}
		v.Schema = schema
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgresql: views: %w", err)
	}
	return out, nil
}

// RowCount：PostgreSQL 无统计估算，直接精确 COUNT。
func (m *metadataProvider) RowCount(ctx context.Context, schema, table string) (int64, error) {
	if table == "" {
		return 0, fmt.Errorf("postgresql: row count: table name is required")
	}
	schema = m.resolveSchema(schema)
	var cnt int64
	if err := m.conn.pool.QueryRowContext(ctx, buildCountSQL(schema, table)).Scan(&cnt); err != nil {
		return 0, fmt.Errorf("postgresql: row count: %w", err)
	}
	return cnt, nil
}

// parsePGArray 解析 pgx 返回的数组字符串（{a,b} 或 a,b 或 [a,b]）。
func parsePGArray(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
