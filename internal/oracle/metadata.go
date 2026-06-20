package oracle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// metadataProvider 实现 driver.MetadataProvider。
type metadataProvider struct {
	conn *conn
}

// Databases 返回「库」列表。
// Oracle 非 CDB：返回单元素（当前数据库名/实例名）；
// CDB：返回各 PDB 名称；若当前账号看不到 v$pdbs，降级返回当前容器名。
//
// 错误处理：v$pdbs 不可见属于常见情况（非 CDB 或权限受限），安全降级；
// 但若连降级查询（DB_NAME）也失败，则把根因返回，避免静默吞错。
func (m *metadataProvider) Databases(ctx context.Context) ([]string, error) {
	// 尝试 PDB 列表（12c+ CDB）。视图不可见属正常，忽略该错误继续降级。
	if rows, err := m.conn.pool.QueryContext(ctx, databasesPDBsSQL); err == nil {
		names, scanErr := scanStringColumn(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("oracle: databases (v$pdbs): %w", scanErr)
		}
		if len(names) > 0 {
			return names, nil
		}
	}

	// 降级：当前数据库名（dual 查询在所有版本可用）。
	var name string
	if err := m.conn.pool.QueryRowContext(ctx,
		"SELECT sys_context('USERENV','DB_NAME') FROM dual").Scan(&name); err != nil {
		return nil, fmt.Errorf("oracle: databases: cannot read DB_NAME (and v$pdbs unavailable): %w", err)
	}
	if name == "" {
		return []string{"(current database)"}, nil
	}
	return []string{name}, nil
}

// scanStringColumn 读取单列字符串结果集并安全关闭 rows。
func scanStringColumn(rows interface {
	Next() bool
	Scan(...any) error
	Close() error
	Err() error
}) ([]string, error) {
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Schemas 列出可见 schema。
func (m *metadataProvider) Schemas(ctx context.Context) ([]string, error) {
	rows, err := m.conn.pool.QueryContext(ctx, schemasSQL)
	if err != nil {
		return nil, fmt.Errorf("oracle: schemas: %w", err)
	}
	names, err := scanStringColumn(rows)
	if err != nil {
		return nil, fmt.Errorf("oracle: schemas: %w", err)
	}
	return names, nil
}

// Tables 列出某 schema 的表。
func (m *metadataProvider) Tables(ctx context.Context, schema string) ([]driver.TableInfo, error) {
	schema, err := m.resolveSchema(ctx, schema)
	if err != nil {
		return nil, err
	}
	rows, err := m.conn.pool.QueryContext(ctx, tablesSQL, strings.ToUpper(schema))
	if err != nil {
		return nil, fmt.Errorf("oracle: tables: %w", err)
	}
	defer rows.Close()
	return scanTables(rows)
}

// tablesLike（供 --like 使用），暴露给 cli 包装。
func (m *metadataProvider) tablesLike(ctx context.Context, schema, pattern string) ([]driver.TableInfo, error) {
	schema, err := m.resolveSchema(ctx, schema)
	if err != nil {
		return nil, err
	}
	rows, err := m.conn.pool.QueryContext(ctx, tablesLikeSQL,
		strings.ToUpper(schema), strings.ToUpper(pattern))
	if err != nil {
		return nil, fmt.Errorf("oracle: tables: %w", err)
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
		var typ, comment sql.NullString
		if err := rows.Scan(&t.Schema, &t.Name, &typ, &comment); err != nil {
			return nil, err
		}
		t.Type = typ.String
		t.Comment = comment.String
		out = append(out, t)
	}
	return out, rows.Err()
}

// Columns 列出表列。
func (m *metadataProvider) Columns(ctx context.Context, schema, table string) ([]driver.ColumnInfo, error) {
	if table == "" {
		return nil, fmt.Errorf("oracle: columns: table name is required")
	}
	schema, err := m.resolveSchema(ctx, schema)
	if err != nil {
		return nil, err
	}
	rows, err := m.conn.pool.QueryContext(ctx, columnsSQL,
		strings.ToUpper(schema), strings.ToUpper(table))
	if err != nil {
		return nil, fmt.Errorf("oracle: columns: %w", err)
	}
	defer rows.Close()
	var out []driver.ColumnInfo
	for rows.Next() {
		var c driver.ColumnInfo
		var nullable int
		var dataDefault, colComment sql.NullString
		if err := rows.Scan(&c.Name, &c.DataType, &c.Length, &c.Precision,
			&c.Scale, &nullable, &dataDefault, &c.Position, &colComment); err != nil {
			return nil, err
		}
		c.DefaultValue = dataDefault.String
		c.Comment = colComment.String
		c.Nullable = nullable == 1
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("oracle: columns: %w", err)
	}
	if len(out) == 0 {
		// 显式提示「未找到」，帮助用户区分「表不存在」与「真无列」。
		return nil, fmt.Errorf("oracle: columns: no columns found for %s.%s (table may not exist or is not visible to current user)",
			strings.ToUpper(schema), strings.ToUpper(table))
	}
	return out, nil
}

// Indexes 列出表索引（聚合列）。
// 主键索引名查询失败时不阻断整体——索引列表仍可用，只是 is_primary 标记可能不准。
func (m *metadataProvider) Indexes(ctx context.Context, schema, table string) ([]driver.IndexInfo, error) {
	if table == "" {
		return nil, fmt.Errorf("oracle: indexes: table name is required")
	}
	schema, err := m.resolveSchema(ctx, schema)
	if err != nil {
		return nil, err
	}
	// 主键索引名（用于标记 is_primary）。视图不可见时安全忽略。
	var pkName string
	_ = m.conn.pool.QueryRowContext(ctx, primaryIndexNameSQL,
		strings.ToUpper(schema), strings.ToUpper(table)).Scan(&pkName)

	rows, err := m.conn.pool.QueryContext(ctx, indexesSQL,
		strings.ToUpper(schema), strings.ToUpper(table))
	if err != nil {
		return nil, fmt.Errorf("oracle: indexes: %w", err)
	}
	defer rows.Close()

	byName := map[string]*driver.IndexInfo{}
	var order []string
	for rows.Next() {
		var name, col string
		var unique, isIOT, pos int
		if err := rows.Scan(&name, &unique, &isIOT, &col, &pos); err != nil {
			return nil, err
		}
		_ = isIOT
		_ = pos
		idx, ok := byName[name]
		if !ok {
			idx = &driver.IndexInfo{Name: name, IsUnique: unique == 1, IsPrimary: name == pkName}
			byName[name] = idx
			order = append(order, name)
		}
		idx.Columns = append(idx.Columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("oracle: indexes: %w", err)
	}
	out := make([]driver.IndexInfo, 0, len(order))
	for _, n := range order {
		out = append(out, *byName[n])
	}
	return out, nil
}

// Views 列出视图。
func (m *metadataProvider) Views(ctx context.Context, schema string) ([]driver.ViewInfo, error) {
	schema, err := m.resolveSchema(ctx, schema)
	if err != nil {
		return nil, err
	}
	rows, err := m.conn.pool.QueryContext(ctx, viewsSQL, strings.ToUpper(schema))
	if err != nil {
		return nil, fmt.Errorf("oracle: views: %w", err)
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
		return nil, fmt.Errorf("oracle: views: %w", err)
	}
	return out, nil
}

// RowCount 取近似行数（来自字典统计）。若为 0，回退到精确 COUNT。
func (m *metadataProvider) RowCount(ctx context.Context, schema, table string) (int64, error) {
	if table == "" {
		return 0, fmt.Errorf("oracle: row count: table name is required")
	}
	schema, err := m.resolveSchema(ctx, schema)
	if err != nil {
		return 0, err
	}
	var approx int64
	if qerr := m.conn.pool.QueryRowContext(ctx, rowCountSQL,
		strings.ToUpper(schema), strings.ToUpper(table)).Scan(&approx); qerr == nil && approx > 0 {
		return approx, nil
	}
	var exact int64
	if err := m.conn.pool.QueryRowContext(ctx,
		buildCountSQL(quoteIdent(schema), quoteIdent(table))).Scan(&exact); err != nil {
		return 0, fmt.Errorf("oracle: row count: %w", err)
	}
	return exact, nil
}

// resolveSchema 解析 schema：传入非空则直接用；为空时取当前 schema。
// 当前 schema 取不到时返回明确错误，让用户/AI 知道需显式指定 --schema。
func (m *metadataProvider) resolveSchema(ctx context.Context, schema string) (string, error) {
	if schema != "" {
		return schema, nil
	}
	s, err := m.currentSchema(ctx)
	if err != nil {
		return "", fmt.Errorf("oracle: cannot determine default schema (%w); please specify --schema", err)
	}
	return s, nil
}

// currentSchema 返回当前会话 schema（大写）。两条查询任一成功即可。
func (m *metadataProvider) currentSchema(ctx context.Context) (string, error) {
	var u string
	err := m.conn.pool.QueryRowContext(ctx,
		"SELECT sys_context('USERENV','CURRENT_SCHEMA') FROM dual").Scan(&u)
	if err == nil && u != "" {
		return strings.ToUpper(u), nil
	}
	if qerr := m.conn.pool.QueryRowContext(ctx, "SELECT user FROM dual").Scan(&u); qerr != nil {
		return "", fmt.Errorf("CURRENT_SCHEMA and user queries both failed: %w", qerr)
	}
	if u == "" {
		return "", errors.New("both CURRENT_SCHEMA and user returned empty")
	}
	return strings.ToUpper(u), nil
}
