package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// versionRe 匹配 VERSION() 的返回值，捕获主次版本号。
// 形如 "8.0.37"、"5.7.43-log"、"8.0.12"、"10.6.16-MariaDB"。
var versionRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// detectVersion 探测 MySQL 版本。
// 通过 SELECT VERSION() 获取；失败且配置了 force_version 则用配置值。
// 与 oracle 不同：MySQL 元数据查询跨版本一致，版本信息主要用于展示，
// 因此 force_version 主要用于离线/受限场景。
func detectVersion(ctx context.Context, db *sql.DB, forceVersion string) (*driver.DBVersion, error) {
	if forceVersion != "" {
		return parseForcedVersion(forceVersion)
	}

	var verStr string
	if err := db.QueryRowContext(ctx, versionSQL).Scan(&verStr); err != nil {
		return nil, fmt.Errorf("mysql: cannot read VERSION(): %w", err)
	}
	v := parseVersionString(verStr)
	v.DetectedAt = time.Now()
	return v, nil
}

// parseVersionString 解析 VERSION() 返回串。
func parseVersionString(s string) *driver.DBVersion {
	v := &driver.DBVersion{
		Product: detectProduct(s),
		Version: strings.TrimSpace(s),
		Banner:  s,
	}
	if m := versionRe.FindStringSubmatch(s); len(m) >= 3 {
		v.Major, _ = strconv.Atoi(m[1])
		v.Minor, _ = strconv.Atoi(m[2])
	}
	return v
}

// detectProduct 从版本串推断产品（MySQL 或 MariaDB）。
func detectProduct(s string) string {
	if strings.Contains(strings.ToUpper(s), "MARIADB") {
		return "MariaDB"
	}
	return "MySQL"
}

// parseForcedVersion 解析 force_version 配置值（如 "8.0"、"5.7"、"8.0.37"）。
func parseForcedVersion(v string) (*driver.DBVersion, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, fmt.Errorf("mysql: empty force_version")
	}
	m := regexp.MustCompile(`^(\d+)\.(\d+)(?:\.(\d+))?$`).FindStringSubmatch(v)
	if m == nil {
		return nil, fmt.Errorf("mysql: cannot parse force_version %q (expect forms like 5.7, 8.0, 8.0.37)", v)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return &driver.DBVersion{
		Product:    "MySQL",
		Version:    fmt.Sprintf("MySQL (forced: %s)", v),
		Major:      major,
		Minor:      minor,
		Banner:     v,
		DetectedAt: time.Now(),
	}, nil
}
