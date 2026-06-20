package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// conn 实现 driver.Conn，底层是 *sql.DB 连接池（go-sql-driver/mysql 驱动）。
type conn struct {
	pool   *sql.DB
	cfg    *driver.DatasourceConfig
	// version 缓存版本探测结果，避免重复查询。
	version *driver.DBVersion
}

// buildDSN 构造 go-sql-driver/mysql 的 DSN。
// 默认即支持 caching_sha2_password（8.0 默认认证）；parseTime 把时间映射到 time.Time；
// interpolateParams 让参数在驱动层拼接（减少往返）。
// TLS 经 normalizeTLS 统一映射：skip-verify 覆盖自签场景（驱动内置，无需注册）。
func buildDSN(cfg *driver.DatasourceConfig) (string, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&interpolateParams=true",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
	if tlsName := normalizeTLS(cfg.TLS); tlsName == "skip-verify" {
		dsn += "&tls=skip-verify"
	} else if tlsName != "" {
		// 用户自定义 TLS 配置名（需自行 RegisterTLSConfig），原样透传。
		dsn += "&tls=" + tlsName
	}
	return dsn, nil
}

// newPool 构建并配置连接池。
func newPool(cfg *driver.DatasourceConfig) (*sql.DB, error) {
	dsn, err := buildDSN(cfg)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql: open: %w", err)
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
		return nil, fmt.Errorf("mysql: query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("mysql: columns: %w", err)
	}

	result := &driver.Result{Columns: cols}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("mysql: scan: %w", err)
		}
		result.Rows = append(result.Rows, values)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: rows: %w", err)
	}
	return result, nil
}

// Exec 执行不返回结果集的语句。权限由 cli 层 WriteGuard 控制。
func (c *conn) Exec(ctx context.Context, query string, args ...any) (driver.ExecResult, error) {
	res, err := c.pool.ExecContext(ctx, query, args...)
	if err != nil {
		return driver.ExecResult{}, fmt.Errorf("mysql: exec: %w", err)
	}
	affected, _ := res.RowsAffected()
	lastID, _ := res.LastInsertId()
	return driver.ExecResult{RowsAffected: affected, LastInsertID: lastID}, nil
}

// Version 探测并缓存 MySQL 版本。
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
