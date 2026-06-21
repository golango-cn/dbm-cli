package impala

import (
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// Driver 实现 driver.Driver，注册名 "impala"。
type Driver struct{}

func (d *Driver) Name() string              { return "impala" }
func (d *Driver) SupportedVersions() string { return "3.x / 4.x (tested 4.5.0)" }
func (d *Driver) Description() string {
	return "Apache Impala (pure-Go via impala-go, no CGO). HiveServer2 protocol. Tested on 4.5.0."
}

// Open 依据配置创建连接。
func (d *Driver) Open(cfg *driver.DatasourceConfig) (driver.Conn, error) {
	if cfg == nil {
		return nil, fmt.Errorf("impala: nil config")
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("impala: host is required")
	}
	if cfg.Port == 0 {
		// 21050 是 HiveServer2 协议默认端口（不是 impala-shell 的 21000）
		cfg.Port = 21050
	}
	pool, err := newPool(cfg)
	if err != nil {
		return nil, err
	}
	return &conn{pool: pool, cfg: cfg}, nil
}
