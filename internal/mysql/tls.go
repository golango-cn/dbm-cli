package mysql

import (
	"strings"
)

// normalizeTLS 把配置里的 tls 字段归一化。
//   - "" / "false" / "no"  → 不启用 TLS
//   - "skip-verify" / "insecure" / "true" / "yes" → 映射为 skip-verify（驱动内置，覆盖自签场景）
//
// 统一映射到 skip-verify：go-sql-driver/mysql 内置支持，无需用户额外注册 TLS 配置，
// 且覆盖最常见的自签证书场景。
func normalizeTLS(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "false", "no", "off", "0":
		return ""
	case "skip-verify", "insecure", "true", "yes", "tls", "1":
		return "skip-verify"
	}
	// 其它值视为自定义 TLS 配置名（用户自行 RegisterTLSConfig），原样返回。
	return strings.ToLower(strings.TrimSpace(s))
}
