package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// metadataProvider 实现 driver.MetadataProvider。
//
// MySQL 概念说明：在 MySQL 中，"schema" 与 "database" 是同义词（CREATE DATABASE
// 等价于 CREATE SCHEMA）。因此本实现里：
//   - Databases() 列出所有用户库（schema）
//   - Schemas() 同 Databases()
//   - 各方法里的 schema 参数即数据库名，为空时用连接配置的 database。
type metadataProvider struct {
	conn *conn
}

// resolveSchema 解析 schema：传入非空则用之；为空则用连接配置的 database。
// MySQL 必须显式选定 database 才能查 information_schema 的表信息，
// 取不到时返回明确错误而非静默。
func (m *metadataProvider) resolveSchema(schema string) (string, error) {
	if schema != "" {
		return schema, nil
	}
	if m.conn.cfg.Database != "" {
		return m.conn.cfg.Database, nil
	}
	return "", fmt.Errorf("mysql: no database selected; please specify --schema or set 'database' in config")
}

// Databases 列出所有用户库（排除系统库）。
func (m *metadataProvider) Databases(ctx context.Context) ([]string, error) {
	rows, err := m.conn.pool.QueryContext(ctx, databasesSQL)
	if err != nil {
		return nil, fmt.Errorf("mysql: databases: %w", err)
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

// Schemas 列出所有用户库（MySQL 中 schema == database）。
func (m *metadataProvider) Schemas(ctx context.Context) ([]string, error) {
	return m.Databases(ctx)
}

// Tables 列出某库的表。
func (m *metadataProvider) Tables(ctx context.Context, schema string) ([]driver.TableInfo, error) {
	schema, err := m.resolveSchema(schema)
	if err != nil {
		return nil, err
	}
	rows, err := m.conn.pool.QueryContext(ctx, tablesSQL, schema)
	if err != nil {
		return nil, fmt.Errorf("mysql: tables: %w", err)
	}
	defer rows.Close()
	return scanTables(rows)
}

func scanTables(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]driver.TableInfo, error) {
	var out []driver.TableInfo
	for rows.Next() {
		var t driver.TableInfo
		if err := rows.Scan(&t.Schema, &t.Name, &t.Type, &t.Comment); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Columns 列出表列。
func (m *metadataProvider) Columns(ctx context.Context, schema, table string) ([]driver.ColumnInfo, error) {
	if table == "" {
		return nil, fmt.Errorf("mysql: columns: table name is required")
	}
	schema, err := m.resolveSchema(schema)
	if err != nil {
		return nil, err
	}
	rows, err := m.conn.pool.QueryContext(ctx, columnsSQL, schema, table)
	if err != nil {
		return nil, fmt.Errorf("mysql: columns: %w", err)
	}
	defer rows.Close()
	var out []driver.ColumnInfo
	for rows.Next() {
		var c driver.ColumnInfo
		var length, precision, scale sql.NullInt64
		var isNullable, extra string
		if err := rows.Scan(&c.Name, &c.DataType, &length, &precision, &scale,
			&isNullable, &c.DefaultValue, &c.Position, &c.Comment, &extra); err != nil {
			return nil, err
		}
		c.Length = length.Int64
		c.Precision = int(precision.Int64)
		c.Scale = int(scale.Int64)
		c.Nullable = isNullable == "YES"
		_ = extra // 预留：auto_increment 等标记
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: columns: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("mysql: columns: no columns found for %s.%s (table may not exist)",
			schema, table)
	}
	return out, nil
}

// Indexes 列出表索引（聚合列）。主键索引名为 PRIMARY。
func (m *metadataProvider) Indexes(ctx context.Context, schema, table string) ([]driver.IndexInfo, error) {
	if table == "" {
		return nil, fmt.Errorf("mysql: indexes: table name is required")
	}
	schema, err := m.resolveSchema(schema)
	if err != nil {
		return nil, err
	}
	rows, err := m.conn.pool.QueryContext(ctx, indexesSQL, schema, table)
	if err != nil {
		return nil, fmt.Errorf("mysql: indexes: %w", err)
	}
	defer rows.Close()

	byName := map[string]*driver.IndexInfo{}
	var order []string
	for rows.Next() {
		var name, col string
		var unique, seq int
		if err := rows.Scan(&name, &unique, &col, &seq); err != nil {
			return nil, err
		}
		idx, ok := byName[name]
		if !ok {
			idx = &driver.IndexInfo{
				Name:      name,
				IsUnique:  unique == 1,
				IsPrimary: name == "PRIMARY",
			}
			byName[name] = idx
			order = append(order, name)
		}
		idx.Columns = append(idx.Columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: indexes: %w", err)
	}
	out := make([]driver.IndexInfo, 0, len(order))
	for _, n := range order {
		out = append(out, *byName[n])
	}
	return out, nil
}

// Views 列出库中的视图。
func (m *metadataProvider) Views(ctx context.Context, schema string) ([]driver.ViewInfo, error) {
	schema, err := m.resolveSchema(schema)
	if err != nil {
		return nil, err
	}
	rows, err := m.conn.pool.QueryContext(ctx, viewsSQL, schema)
	if err != nil {
		return nil, fmt.Errorf("mysql: views: %w", err)
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
		return nil, fmt.Errorf("mysql: views: %w", err)
	}
	return out, nil
}

// RowCount 取近似行数（来自统计值）。InnoDB 下为估算值，为 0 时回退精确 COUNT。
func (m *metadataProvider) RowCount(ctx context.Context, schema, table string) (int64, error) {
	if table == "" {
		return 0, fmt.Errorf("mysql: row count: table name is required")
	}
	schema, err := m.resolveSchema(schema)
	if err != nil {
		return 0, err
	}
	var approx int64
	if qerr := m.conn.pool.QueryRowContext(ctx, rowCountSQL, schema, table).Scan(&approx); qerr == nil && approx > 0 {
		return approx, nil
	}
	var exact int64
	if err := m.conn.pool.QueryRowContext(ctx, buildCountSQL(schema, table)).Scan(&exact); err != nil {
		return 0, fmt.Errorf("mysql: row count: %w", err)
	}
	return exact, nil
}
