package impala

// Kerberos 配置解析与 DSN 构造。
//
// 配置来自 DatasourceConfig.Raw 的 "kerberos" 子段（yaml inline），形如：
//
//   kerberos:
//     realm: TESTBOE.COM
//     service: impala          # 可选，默认 impala
//     krb_host: impalad-krb-1   # principal 里的 hostname
//     keytab: /path/to/x.keytab
//     principal: cdptest@TESTBOE.COM
//     krb5_conf: /path/to/krb5.conf
//
// 不修改 driver.DatasourceConfig 核心结构，经 Raw 扩展点读取。

import (
	"fmt"
	"net/url"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// kerberosCfg 描述一份 Kerberos 连接配置（从 yaml kerberos 段解析）。
type kerberosCfg struct {
	Realm     string `yaml:"realm"`
	Service   string `yaml:"service"`
	KrbHost   string `yaml:"krb_host"`
	Keytab    string `yaml:"keytab"`
	Principal string `yaml:"principal"`
	Krb5Conf  string `yaml:"krb5_conf"`
}

// parseKerberosCfg 从 DatasourceConfig.Raw 解析 kerberos 段。
// 返回 nil 表示未配置 Kerberos（走普通用户名/密码或无认证路径）。
func parseKerberosCfg(cfg *driver.DatasourceConfig) *kerberosCfg {
	if cfg == nil || cfg.Raw == nil {
		return nil
	}
	raw, ok := cfg.Raw["kerberos"]
	if !ok || raw == nil {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	get := func(k string) string {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	k := &kerberosCfg{
		Realm:     get("realm"),
		Service:   get("service"),
		KrbHost:   get("krb_host"),
		Keytab:    get("keytab"),
		Principal: get("principal"),
		Krb5Conf:  get("krb5_conf"),
	}
	// 任一必填项缺失则视为未配置（返回 nil，上层走非 Kerberos 路径）。
	if k.Principal == "" || k.Keytab == "" || k.Krb5Conf == "" || k.Realm == "" {
		return nil
	}
	if k.Service == "" {
		k.Service = "impala"
	}
	return k
}

// setupKerberos 用 keytab 换取 ticket，写入临时 ccache 并设 KRB5CCNAME。
// 返回 cleanup 在连接 Close 时调用。
func setupKerberos(krb *kerberosCfg) (func(), error) {
	if krb == nil {
		return nil, nil
	}
	_, cleanup, err := acquireTicketCache(krb.Principal, krb.Keytab, krb.Krb5Conf)
	if err != nil {
		return nil, err
	}
	return cleanup, nil
}

// buildKerberosDSN 构造 Kerberos 模式下的 DSN。
//   impala://host:port/database?auth=kerberos&service=impala&krb_host=impalad-krb-1
// 驱动 connect() 看到 auth=kerberos 会启用 GSSAPI SASL transport，
// 并用 service/krb_host 构造服务主体（impala/krb_host@REALM）。
// 票据来源由调用方事先设好 KRB5CCNAME（见 setupKerberos）。
func buildKerberosDSN(cfg *driver.DatasourceConfig, krb *kerberosCfg) string {
	u := url.URL{Scheme: "impala", Host: fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)}
	q := u.Query()
	q.Set("auth", "kerberos")
	q.Set("service", krb.Service)
	q.Set("krb_host", krb.KrbHost)
	if cfg.Database != "" {
		q.Set("use", cfg.Database)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
