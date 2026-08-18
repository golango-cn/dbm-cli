package impala

import (
	"net/url"
	"testing"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// parseQuery 把 DSN 的 query 部分解析成 map，便于断言。
func parseQuery(t *testing.T, dsn string) url.Values {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn %q: %v", dsn, err)
	}
	return u.Query()
}

func TestBuildDSNNoAuth(t *testing.T) {
	// 无认证：不配 user/password，不配 auth_mech —— 不能出现 auth 参数。
	cfg := &driver.DatasourceConfig{Host: "h", Port: 21050, Database: "ods"}
	dsn := buildDSN(cfg)
	q := parseQuery(t, dsn)
	if q.Get("auth") != "" {
		t.Errorf("no-auth datasource must not set auth, got %q (dsn=%s)", q.Get("auth"), dsn)
	}
	if u, err := url.Parse(dsn); err != nil || u.User != nil {
		t.Errorf("no-auth datasource must not embed userinfo, got %q", dsn)
	}
}

func TestBuildDSNLDAP(t *testing.T) {
	// LDAP：auth_mech: LDAP + user/password —— DSN 必须带 auth=ldap 且凭据进入 userinfo。
	cfg := &driver.DatasourceConfig{
		Host:     "10.95.210.12",
		Port:     21050,
		Database: "ods",
		User:     "bdp_admin",
		Password: "secret",
		Raw: map[string]any{
			"auth_mech": "LDAP",
		},
	}
	dsn := buildDSN(cfg)
	q := parseQuery(t, dsn)
	if q.Get("auth") != "ldap" {
		t.Errorf("LDAP datasource must set auth=ldap, got %q (dsn=%s)", q.Get("auth"), dsn)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if u.User == nil || u.User.Username() != "bdp_admin" {
		t.Errorf("LDAP datasource must embed username, got %q", dsn)
	}
	pw, ok := u.User.Password()
	if !ok || pw != "secret" {
		t.Errorf("LDAP datasource must embed password, got %q", dsn)
	}
}

func TestBuildDSNUserPasswordWithoutAuthMechStaysNoSASL(t *testing.T) {
	// 兼容性保障：旧配置里有 user/password 但没配 auth_mech（此前是无效凭据 + 无 SASL），
	// 现在必须保持原行为：不启用任何认证（不设 auth 参数），避免改变现有无认证集群的行为。
	cfg := &driver.DatasourceConfig{
		Host:     "h",
		Port:     21050,
		User:     "legacy_user",
		Password: "legacy_pw",
	}
	dsn := buildDSN(cfg)
	q := parseQuery(t, dsn)
	if q.Get("auth") != "" {
		t.Errorf("user/password without auth_mech must stay no-SASL, got auth=%q (dsn=%s)", q.Get("auth"), dsn)
	}
}

func TestBuildDSNLDAPCustomPort(t *testing.T) {
	cfg := &driver.DatasourceConfig{
		Host:     "h",
		Port:     21051,
		User:     "u",
		Password: "p",
		Raw:      map[string]any{"auth_mech": "ldap"}, // 小写也接受
	}
	dsn := buildDSN(cfg)
	q := parseQuery(t, dsn)
	if q.Get("auth") != "ldap" {
		t.Errorf("lowercase ldap should be accepted, got %q (dsn=%s)", q.Get("auth"), dsn)
	}
}

func TestBuildKerberosDSNUnchanged(t *testing.T) {
	// Kerberos DSN 不受 auth_mech 影响。
	cfg := &driver.DatasourceConfig{Host: "h", Port: 21051, Database: "default",
		Raw: map[string]any{"auth_mech": "LDAP"}}
	krb := &kerberosCfg{Realm: "TESTBOE.COM", Service: "impala", KrbHost: "impalad-krb-1"}
	dsn := buildKerberosDSN(cfg, krb)
	q := parseQuery(t, dsn)
	if q.Get("auth") != "kerberos" {
		t.Errorf("kerberos dsn must keep auth=kerberos, got %q (dsn=%s)", q.Get("auth"), dsn)
	}
}

func TestParseAuthMech(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{"nil raw", nil, ""},
		{"empty", map[string]any{}, ""},
		{"upper", map[string]any{"auth_mech": "LDAP"}, "ldap"},
		{"lower", map[string]any{"auth_mech": "ldap"}, "ldap"},
		{"mixed", map[string]any{"auth_mech": "Ldap"}, "ldap"},
		{"nosasl", map[string]any{"auth_mech": "NOSASL"}, ""},
		{"unknown", map[string]any{"auth_mech": "bogus"}, ""},
		{"non-string", map[string]any{"auth_mech": 3}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &driver.DatasourceConfig{Raw: tt.raw}
			if got := parseAuthMech(cfg); got != tt.want {
				t.Errorf("parseAuthMech(%v) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
