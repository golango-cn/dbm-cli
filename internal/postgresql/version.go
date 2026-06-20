package postgresql

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

// versionRe 匹配 PostgreSQL version()，如 "PostgreSQL 17.5 (Debian ...)"。
var versionRe = regexp.MustCompile(`PostgreSQL\s+(\d+)\.(\d+)`)

// detectVersion 探测版本。show_server_version 返回如 "17.5"。
func detectVersion(ctx context.Context, db *sql.DB, forceVersion string) (*driver.DBVersion, error) {
	if forceVersion != "" {
		return parseForcedVersion(forceVersion)
	}
	var verStr string
	// SHOW server_version 返回简洁版本号（如 "17.5"），比 version() 更易解析
	if err := db.QueryRowContext(ctx, "SHOW server_version").Scan(&verStr); err != nil {
		return nil, fmt.Errorf("postgresql: cannot read server_version: %w", err)
	}
	v := parseVersionString(verStr)
	v.DetectedAt = time.Now()
	return v, nil
}

func parseVersionString(s string) *driver.DBVersion {
	v := &driver.DBVersion{
		Product: "PostgreSQL",
		Version: strings.TrimSpace(s),
		Banner:  s,
	}
	if m := versionRe.FindStringSubmatch(s); len(m) >= 3 {
		v.Major, _ = strconv.Atoi(m[1])
		v.Minor, _ = strconv.Atoi(m[2])
	} else {
		// server_version 形如 "17.5"，无 "PostgreSQL" 前缀
		if m := regexp.MustCompile(`(\d+)\.(\d+)`).FindStringSubmatch(s); len(m) >= 3 {
			v.Major, _ = strconv.Atoi(m[1])
			v.Minor, _ = strconv.Atoi(m[2])
		}
	}
	return v
}

func parseForcedVersion(v string) (*driver.DBVersion, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, fmt.Errorf("postgresql: empty force_version")
	}
	m := regexp.MustCompile(`^(\d+)\.(\d+)$`).FindStringSubmatch(v)
	if m == nil {
		return nil, fmt.Errorf("postgresql: cannot parse force_version %q (expect like 12.5, 17.2)", v)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return &driver.DBVersion{
		Product: "PostgreSQL", Version: "PostgreSQL (forced: " + v + ")",
		Major: major, Minor: minor, Banner: v, DetectedAt: time.Now(),
	}, nil
}
