package impala

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// conn 实现 driver.Conn，底层是 *sql.DB（impala-go 驱动，走 HiveServer2）。
type conn struct {
	pool    *sql.DB
	cfg     *driver.DatasourceConfig
	version *driver.DBVersion
}

// buildDSN 构造 impala-go 的 DSN。
func buildDSN(cfg *driver.DatasourceConfig) string {
	u := url.URL{Scheme: "impala", Host: fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)}
	if cfg.User != "" {
		if cfg.Password != "" {
			u.User = url.UserPassword(cfg.User, cfg.Password)
		} else {
			u.User = url.User(cfg.User)
		}
	}
	q := u.Query()
	if normalizeTLS(cfg.TLS) {
		q.Set("tls", "true")
	}
	if cfg.Database != "" {
		q.Set("use", cfg.Database)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// newPool 构建并配置连接池。
// Impala 的"当前库"是连接级状态：USE 只作用于执行它的那条连接。
// 为确保 Query 里先 USE 再查的语义在同一物理连接上成立，强制单连接池。
func newPool(cfg *driver.DatasourceConfig) (*sql.DB, error) {
	dsn := buildDSN(cfg)
	db, err := sql.Open("impala", dsn)
	if err != nil {
		return nil, fmt.Errorf("impala: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}

// leasedConn 借一条固定连接并确保当前库正确。
// Impala 当前库是连接级状态，必须先 USE 再在同一连接上执行后续 SQL。
func (c *conn) leasedConn(ctx context.Context) (*sql.Conn, error) {
	conn, err := c.pool.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("impala: get conn: %w", err)
	}
	if c.cfg.Database != "" {
		if _, err := conn.ExecContext(ctx, "USE "+quoteIdent(c.cfg.Database)); err != nil {
			conn.Close()
			return nil, fmt.Errorf("impala: USE %s: %w", c.cfg.Database, err)
		}
	}
	return conn, nil
}

// Query 执行返回结果集的语句。
// 用专属连接（pool.Conn），先 USE 再查，保证当前库正确。
func (c *conn) Query(ctx context.Context, query string, args ...any) (*driver.Result, error) {
	conn, err := c.leasedConn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("impala: query: %w", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("impala: columns: %w", err)
	}
	result := &driver.Result{Columns: cols}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("impala: scan: %w", err)
		}
		result.Rows = append(result.Rows, values)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("impala: rows: %w", err)
	}
	return result, nil
}

// Exec 执行写语句。同样用专属连接先 USE。
func (c *conn) Exec(ctx context.Context, query string, args ...any) (driver.ExecResult, error) {
	conn, err := c.leasedConn(ctx)
	if err != nil {
		return driver.ExecResult{}, err
	}
	defer conn.Close()
	res, err := conn.ExecContext(ctx, query, args...)
	if err != nil {
		return driver.ExecResult{}, fmt.Errorf("impala: exec: %w", err)
	}
	affected, _ := res.RowsAffected()
	return driver.ExecResult{RowsAffected: affected}, nil
}

// Version 探测版本。
func (c *conn) Version(ctx context.Context) (*driver.DBVersion, error) {
	if c.version != nil {
		return c.version, nil
	}
	conn, err := c.leasedConn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var verStr string
	if err := conn.QueryRowContext(ctx, "SELECT version()").Scan(&verStr); err != nil {
		return nil, fmt.Errorf("impala: cannot read version(): %w", err)
	}
	v := parseVersionString(verStr)
	c.version = v
	return v, nil
}

// Metadata 返回元数据查询提供者。
func (c *conn) Metadata() driver.MetadataProvider { return &metadataProvider{conn: c} }

// Ping 验证连接。
func (c *conn) Ping(ctx context.Context) error {
	conn, err := c.leasedConn(ctx)
	if err != nil {
		return err
	}
	return conn.Close()
}

// Close 关闭连接池。
func (c *conn) Close() error {
	if c.pool != nil {
		return c.pool.Close()
	}
	return nil
}

// normalizeTLS 归一化 TLS：非空即启用。
func normalizeTLS(s string) bool {
	switch s {
	case "", "false", "no", "off", "0", "disable":
		return false
	}
	return true
}
