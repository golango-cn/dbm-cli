package clickhouse

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// conn 实现 driver.Conn，底层是 *sql.DB 连接池（clickhouse-go/v2 驱动）。
type conn struct {
	pool    *sql.DB
	cfg     *driver.DatasourceConfig
	version *driver.DBVersion
}

// buildDSN 构造 clickhouse-go/v2 的 DSN。
// clickhouse-go/v2 接受 clickhouse://user:pass@host:port/db?param=value 形式。
// 当 cfg.TLS 设置为非空时附加 secure=true 启用 TLS（原生协议走 TLS）。
func buildDSN(cfg *driver.DatasourceConfig) string {
	u := url.URL{
		Scheme: "clickhouse",
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
	}
	if cfg.Database != "" {
		u.Path = cfg.Database
	}
	q := u.Query()
	if normalizeTLS(cfg.TLS) {
		q.Set("secure", "true")
		// 自签场景：跳过证书校验
		q.Set("skip_verify", "true")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// newPool 构建并配置连接池。
func newPool(cfg *driver.DatasourceConfig) (*sql.DB, error) {
	dsn := buildDSN(cfg)
	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: open: %w", err)
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
// ClickHouse 使用命名占位符 {name:Type} 或 ?，这里用 database/sql 的 ? 形式。
func (c *conn) Query(ctx context.Context, query string, args ...any) (*driver.Result, error) {
	rows, err := c.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("clickhouse: columns: %w", err)
	}

	result := &driver.Result{Columns: cols}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("clickhouse: scan: %w", err)
		}
		result.Rows = append(result.Rows, values)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse: rows: %w", err)
	}
	return result, nil
}

// Exec 执行不返回结果集的语句。权限由 cli 层 WriteGuard 控制。
func (c *conn) Exec(ctx context.Context, query string, args ...any) (driver.ExecResult, error) {
	res, err := c.pool.ExecContext(ctx, query, args...)
	if err != nil {
		return driver.ExecResult{}, fmt.Errorf("clickhouse: exec: %w", err)
	}
	affected, _ := res.RowsAffected()
	return driver.ExecResult{RowsAffected: affected}, nil
}

// Version 探测并缓存版本。
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
func (c *conn) Metadata() driver.MetadataProvider {
	return &metadataProvider{conn: c}
}

// Ping 验证连接可用性。
func (c *conn) Ping(ctx context.Context) error {
	return c.pool.PingContext(ctx)
}

// Close 关闭连接池。
func (c *conn) Close() error {
	if c.pool != nil {
		return c.pool.Close()
	}
	return nil
}

// normalizeTLS 归一化 TLS 配置：非空即视为启用 TLS（附带 skip_verify）。
func normalizeTLS(s string) bool {
	switch s {
	case "", "false", "no", "off", "0":
		return false
	}
	return true
}
