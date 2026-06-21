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
// impala-go 接受 impala://user:pass@host:port?auth=... 形式。
// quickstart 集群默认无认证（noauth），auth 留空即可。
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
	// 默认无认证（quickstart 场景）。如需 LDAP/NOSASL 等，可由 Raw 扩展。
	if normalizeTLS(cfg.TLS) {
		q.Set("tls", "true")
	}
	if cfg.Database != "" {
		// 用 use 参数让驱动连上后自动 USE 该库（Impala 概念中库 == database/schema）
		q.Set("use", cfg.Database)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// newPool 构建并配置连接池。
func newPool(cfg *driver.DatasourceConfig) (*sql.DB, error) {
	dsn := buildDSN(cfg)
	db, err := sql.Open("impala", dsn)
	if err != nil {
		return nil, fmt.Errorf("impala: open: %w", err)
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

// Exec 执行写语句。权限由 cli 层 WriteGuard 控制。
// 注意：HiveServer2 的写操作（INSERT/CREATE 等）通过同一查询接口执行，
// 返回的 RowsAffected 在 Impala 上通常为 0（它不精确统计）。
func (c *conn) Exec(ctx context.Context, query string, args ...any) (driver.ExecResult, error) {
	res, err := c.pool.ExecContext(ctx, query, args...)
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

// normalizeTLS 归一化 TLS：非空即启用。
func normalizeTLS(s string) bool {
	switch s {
	case "", "false", "no", "off", "0", "disable":
		return false
	}
	return true
}
