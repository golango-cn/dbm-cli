package postgresql

import (
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// Driver 实现 driver.Driver，注册名 "postgresql"。
type Driver struct{}

func (d *Driver) Name() string              { return "postgresql" }
func (d *Driver) SupportedVersions() string { return "9.6+ (tested 12 / 17)" }
func (d *Driver) Description() string {
	return "PostgreSQL (pure-Go via pgx/v5, no CGO). Tested on 12 / 17."
}

// Open 依据配置创建连接。
func (d *Driver) Open(cfg *driver.DatasourceConfig) (driver.Conn, error) {
	if cfg == nil {
		return nil, fmt.Errorf("postgresql: nil config")
	}
	if cfg.Database == "" {
		return nil, fmt.Errorf("postgresql: database is required")
	}
	if cfg.User == "" {
		return nil, fmt.Errorf("postgresql: user is required")
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("postgresql: host is required")
	}
	if cfg.Port == 0 {
		cfg.Port = 5432
	}
	pool, err := newPool(cfg)
	if err != nil {
		return nil, err
	}
	return &conn{pool: pool, cfg: cfg}, nil
}
