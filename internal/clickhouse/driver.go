package clickhouse

import (
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// Driver 实现 driver.Driver，注册名 "clickhouse"。
type Driver struct{}

// Name 返回驱动名。
func (d *Driver) Name() string { return "clickhouse" }

// SupportedVersions 返回支持的 ClickHouse 版本范围。
func (d *Driver) SupportedVersions() string { return "22.x+ (tested 25.x)" }

// Description 返回面向 manifest 的描述。
func (d *Driver) Description() string {
	return "ClickHouse (pure-Go via clickhouse-go/v2, no CGO). Native TCP protocol. Tested on 25.x."
}

// Open 依据配置创建连接。
func (d *Driver) Open(cfg *driver.DatasourceConfig) (driver.Conn, error) {
	if cfg == nil {
		return nil, fmt.Errorf("clickhouse: nil config")
	}
	if cfg.User == "" {
		return nil, fmt.Errorf("clickhouse: user is required")
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("clickhouse: host is required")
	}
	if cfg.Port == 0 {
		// 9000 是原生 TCP 协议默认端口（clickhouse-go/v2 原生协议）
		cfg.Port = 9000
	}

	pool, err := newPool(cfg)
	if err != nil {
		return nil, err
	}
	return &conn{pool: pool, cfg: cfg}, nil
}
