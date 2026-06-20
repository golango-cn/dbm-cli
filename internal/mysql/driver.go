package mysql

import (
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// Driver 实现 driver.Driver，注册名 "mysql"。
type Driver struct{}

// Name 返回驱动名。
func (d *Driver) Name() string { return "mysql" }

// SupportedVersions 返回支持的 MySQL 版本范围。
func (d *Driver) SupportedVersions() string { return "5.7 / 8.0.x" }

// Description 返回面向 manifest 的描述。
func (d *Driver) Description() string {
	return "MySQL 5.7 / 8.0 (pure-Go via go-sql-driver/mysql, no CGO). " +
		"Supports caching_sha2_password (8.0 default auth). " +
		"Also serves MariaDB via the 'mariadb' type alias (protocol-compatible)."
}

// Open 依据配置创建连接。
func (d *Driver) Open(cfg *driver.DatasourceConfig) (driver.Conn, error) {
	if cfg == nil {
		return nil, fmt.Errorf("mysql: nil config")
	}
	if cfg.Database == "" {
		return nil, fmt.Errorf("mysql: database name is required")
	}
	if cfg.User == "" {
		return nil, fmt.Errorf("mysql: user is required")
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("mysql: host is required")
	}
	if cfg.Port == 0 {
		cfg.Port = 3306
	}

	pool, err := newPool(cfg)
	if err != nil {
		return nil, err
	}
	return &conn{pool: pool, cfg: cfg}, nil
}
