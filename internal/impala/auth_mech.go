package impala

// Impala 认证方式配置。
//
// 配置来自 DatasourceConfig.Raw 的 "auth_mech" 字段（yaml inline），形如：
//
//   auth_mech: LDAP            # NOSASL（默认，不配即无认证）/ LDAP / KERBEROS
//   user: bdp_admin
//   password: ${IMPALA_LDAP_PW}
//
// 与 Cloudera JDBC 的 AuthMech 取值对齐：
//   - 不配置 / NOSASL  → 无认证（裸 thrift，不发 SASL 握手）
//   - LDAP（AuthMech=3）→ SASL PLAIN 握手，需 user/password
//   - KERBEROS（AuthMech=1）→ 由 kerberos: 段驱动（见 kerberos_cfg.go），
//     配置了 kerberos 段时忽略 auth_mech
//
// 不修改 driver.DatasourceConfig 核心结构，经 Raw 扩展点读取（与 kerberos 同模式）。

import (
	"fmt"
	"strings"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// parseAuthMech 归一化 auth_mech 配置。
// 仅 "ldap" 返回 "ldap"（驱动 DSN 的 auth=ldap 参数）；
// 未配置 / NOSASL / 其它值返回 ""（无认证路径，保持既有行为）。
// KERBEROS 不在此处理：由 kerberos 段的存在性决定（parseKerberosCfg）。
func parseAuthMech(cfg *driver.DatasourceConfig) string {
	if cfg == nil || cfg.Raw == nil {
		return ""
	}
	v, ok := cfg.Raw["auth_mech"]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ldap":
		return "ldap"
	default:
		// NOSASL / KERBEROS / 未知值都不改变默认路径：
		// KERBEROS 的真正开关是 kerberos 段；NOSASL 即默认。
		return ""
	}
}

// validateAuthMech 在建连前校验认证配置的完整性。
// LDAP 要求 user 与 password 同时存在，缺失时给出可定位的报错
// （否则错误会在连接池首次取连接时才以 "bad connection" 形式出现，
// 即 issue 中诊断困难的那类报错）。
func validateAuthMech(cfg *driver.DatasourceConfig) error {
	if parseAuthMech(cfg) != "ldap" {
		return nil
	}
	if cfg.User == "" || cfg.Password == "" {
		return fmt.Errorf("impala: auth_mech=LDAP requires both user and password (datasource host=%s)", cfg.Host)
	}
	return nil
}
