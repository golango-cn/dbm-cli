package postgresql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// conn 实现 driver.Conn，底层是 *sql.DB（pgx/v5/stdlib 驱动）。
type conn struct {
	pool    *sql.DB
	cfg     *driver.DatasourceConfig
	version *driver.DBVersion
}

// buildDSN 构造 pgx 的 database/sql DSN（key=value 形式）。
// pgx/v5/stdlib 接受 postgres:// URL 或 key=value DSN；这里用后者。
func buildDSN(cfg *driver.DatasourceConfig) string {
	sslmode := "disable"
	if normalizeTLS(cfg.TLS) {
		sslmode = "require"
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, sslmode)
}

// newPool 构建并配置连接池。
func newPool(cfg *driver.DatasourceConfig) (*sql.DB, error) {
	dsn := buildDSN(cfg)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgresql: open: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	return db, nil
}

// Query 执行返回结果集的语句。
func (c *conn) Query(ctx context.Context, query string, args ...any) (*driver.Result, error) {
	rows, err := c.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgresql: query: %w", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("postgresql: columns: %w", err)
	}
	result := &driver.Result{Columns: cols}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("postgresql: scan: %w", err)
		}
		result.Rows = append(result.Rows, values)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgresql: rows: %w", err)
	}
	return result, nil
}

// Exec 执行写语句。权限由 cli 层 WriteGuard 控制。
func (c *conn) Exec(ctx context.Context, query string, args ...any) (driver.ExecResult, error) {
	res, err := c.pool.ExecContext(ctx, query, args...)
	if err != nil {
		return driver.ExecResult{}, fmt.Errorf("postgresql: exec: %w", err)
	}
	affected, _ := res.RowsAffected()
	return driver.ExecResult{RowsAffected: affected}, nil
}

// Version 探测版本。
func (c *conn) Version(ctx context.Context) (*driver.DBVersion, error) {
	if c.version != nil {
		return c.version, nil
	}
	v, err := detectVersion(ctx, c.pool, c.cfg.ForceVersion)
	if err != nil {
		return nil, err
	}
	c.version = v
	return v, nil
}

// Metadata 返回元数据查询提供者。
func (c *conn) Metadata() driver.MetadataProvider { return &metadataProvider{conn: c} }

// Ping 验证连接。
func (c *conn) Ping(ctx context.Context) error { return c.pool.PingContext(ctx) }

// Close 关闭连接池。
func (c *conn) Close() error {
	if c.pool != nil {
		return c.pool.Close()
	}
	return nil
}

// normalizeTLS 归一化 TLS：非空即启用（sslmode=require）。
func normalizeTLS(s string) bool {
	switch s {
	case "", "false", "no", "off", "0", "disable":
		return false
	}
	return true
}
