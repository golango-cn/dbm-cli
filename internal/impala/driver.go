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

	// 解析可选的 kerberos 配置（来自 cfg.Raw 的 kerberos 段）。
	// 若配置了 keytab，先换取 ticket 写入临时 ccache 并设置 KRB5CCNAME，
	// 驱动（go-gssapi krb5 backend）会读取该票据完成 GSSAPI 握手。
	krb := parseKerberosCfg(cfg)
	var cleanup func() = nil
	if krb != nil {
		c, err := setupKerberos(krb)
		if err != nil {
			return nil, err
		}
		cleanup = c
	}

	pool, err := newPool(cfg, krb)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, err
	}
	return &conn{pool: pool, cfg: cfg, cleanup: cleanup}, nil
}
