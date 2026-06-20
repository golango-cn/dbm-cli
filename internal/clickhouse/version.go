package clickhouse

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

// versionRe 匹配 ClickHouse version() 返回值，如 "25.8.1.1"。
var versionRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// detectVersion 探测 ClickHouse 版本。
func detectVersion(ctx context.Context, db *sql.DB, forceVersion string) (*driver.DBVersion, error) {
	if forceVersion != "" {
		return parseForcedVersion(forceVersion)
	}
	var verStr string
	if err := db.QueryRowContext(ctx, versionSQL).Scan(&verStr); err != nil {
		return nil, fmt.Errorf("clickhouse: cannot read version(): %w", err)
	}
	v := parseVersionString(verStr)
	v.DetectedAt = time.Now()
	return v, nil
}

func parseVersionString(s string) *driver.DBVersion {
	v := &driver.DBVersion{
		Product: "ClickHouse",
		Version: strings.TrimSpace(s),
		Banner:  s,
	}
	if m := versionRe.FindStringSubmatch(s); len(m) >= 3 {
		v.Major, _ = strconv.Atoi(m[1])
		v.Minor, _ = strconv.Atoi(m[2])
	}
	return v
}

func parseForcedVersion(v string) (*driver.DBVersion, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, fmt.Errorf("clickhouse: empty force_version")
	}
	m := regexp.MustCompile(`^(\d+)\.(\d+)(?:\.(\d+))?$`).FindStringSubmatch(v)
	if m == nil {
		return nil, fmt.Errorf("clickhouse: cannot parse force_version %q (expect forms like 25.8)", v)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return &driver.DBVersion{
		Product:    "ClickHouse",
		Version:    fmt.Sprintf("ClickHouse (forced: %s)", v),
		Major:      major,
		Minor:      minor,
		Banner:     v,
		DetectedAt: time.Now(),
	}, nil
}
