package oracle

import (
	"context"
	"database/sql"
	"fmt"

	go_ora "github.com/sijms/go-ora/v2"
	"github.com/golango-cn/dbm-cli/internal/driver"
)

// defaultFetchSize 是未配置 fetch_size 时的行预取大小。
const defaultFetchSize = 1000

// conn 实现 driver.Conn，底层是 *sql.DB 连接池（go-ora 驱动）。
type conn struct {
	pool *sql.DB
	cfg  *driver.DatasourceConfig
	// version 缓存版本探测结果，避免重复查询。
	version *driver.DBVersion
}

// buildDSN 用 go_ora.BuildUrl 构建连接串（Builder 模式）。
// service_name 与 sid 二选一：sid 通过 options["SID"] 传入。
func buildDSN(cfg *driver.DatasourceConfig) (string, error) {
	options := map[string]string{}
	if cfg.SID != "" {
		// 当使用 SID 时，第三个参数（service）置空，通过 SID 选项区分
		options["SID"] = cfg.SID
	}
	service := cfg.ServiceName // 若 SID 模式则为空
	dsn := go_ora.BuildUrl(cfg.Host, cfg.Port, service, cfg.User, cfg.Password, options)
	return dsn, nil
}

// newPool 构建并配置连接池。
func newPool(cfg *driver.DatasourceConfig) (*sql.DB, error) {
	dsn, err := buildDSN(cfg)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("oracle", dsn)
	if err != nil {
		return nil, fmt.Errorf("oracle: open: %w", err)
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
		return nil, fmt.Errorf("oracle: query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("oracle: columns: %w", err)
	}

	result := &driver.Result{Columns: cols}
	for rows.Next() {
		// 用 []any 配合 make([]any, n) 的经典扫描模式
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("oracle: scan: %w", err)
		}
		result.Rows = append(result.Rows, values)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("oracle: rows: %w", err)
	}
	return result, nil
}

// Exec 执行不返回结果集的语句。
// 注意：权限由 cli 层的 WriteGuard 控制，这里不做判断。
func (c *conn) Exec(ctx context.Context, query string, args ...any) (driver.ExecResult, error) {
	res, err := c.pool.ExecContext(ctx, query, args...)
	if err != nil {
		return driver.ExecResult{}, fmt.Errorf("oracle: exec: %w", err)
	}
	affected, _ := res.RowsAffected()
	return driver.ExecResult{RowsAffected: affected}, nil
}

// Version 探测并缓存数据库版本。
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

// Ping 验证连接可用性（触发真实建连，让配置错误尽早暴露）。
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
