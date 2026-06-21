package sqlserver

import (
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// Driver 实现 driver.Driver，注册名 "sqlserver"。
type Driver struct{}

func (d *Driver) Name() string              { return "sqlserver" }
func (d *Driver) SupportedVersions() string { return "2017+ (tested 2017 / 2022)" }
func (d *Driver) Description() string {
	return "Microsoft SQL Server (pure-Go via go-mssqldb, no CGO, no ODBC). Tested on 2017 / 2022."
}

// Open 依据配置创建连接。
func (d *Driver) Open(cfg *driver.DatasourceConfig) (driver.Conn, error) {
	if cfg == nil {
		return nil, fmt.Errorf("sqlserver: nil config")
	}
	if cfg.User == "" {
		return nil, fmt.Errorf("sqlserver: user is required")
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("sqlserver: host is required")
	}
	if cfg.Port == 0 {
		cfg.Port = 1433
	}
	pool, err := newPool(cfg)
	if err != nil {
		return nil, err
	}
	return &conn{pool: pool, cfg: cfg}, nil
}
