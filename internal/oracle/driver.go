package oracle

import (
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// Driver 实现 driver.Driver，注册名 "oracle"。
type Driver struct{}

// Name 返回驱动名。
func (d *Driver) Name() string { return "oracle" }

// SupportedVersions 返回支持的 Oracle 版本范围。
func (d *Driver) SupportedVersions() string { return "10.2+" }

// Description 返回面向 manifest 的描述。
func (d *Driver) Description() string {
	return "Oracle Database (pure-Go via go-ora, no Instant Client / no CGO). Supports 10g/11g/12c/18c/19c/21c."
}

// Open 依据配置创建连接。
func (d *Driver) Open(cfg *driver.DatasourceConfig) (driver.Conn, error) {
	if cfg == nil {
		return nil, fmt.Errorf("oracle: nil config")
	}
	if cfg.ServiceName == "" && cfg.SID == "" {
		return nil, fmt.Errorf("oracle: either service_name or sid is required")
	}
	if cfg.User == "" {
		return nil, fmt.Errorf("oracle: user is required")
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("oracle: host is required")
	}
	if cfg.Port == 0 {
		cfg.Port = 1521
	}

	pool, err := newPool(cfg)
	if err != nil {
		return nil, err
	}
	return &conn{pool: pool, cfg: cfg}, nil
}
