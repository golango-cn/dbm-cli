package impala

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

// versionRe 匹配 Impala 版本串中的 x.y.z。
var versionRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// detectVersion 探测 Impala 版本。
// Impala 的 version() 返回一段含 "impalad version 4.5.0-RELEASE" 的描述文本，
// 解析其中的版本号。
func detectVersion(ctx context.Context, db *sql.DB, forceVersion string) (*driver.DBVersion, error) {
	if forceVersion != "" {
		return parseForcedVersion(forceVersion)
	}
	var verStr string
	if err := db.QueryRowContext(ctx, "SELECT version()").Scan(&verStr); err != nil {
		return nil, fmt.Errorf("impala: cannot read version(): %w", err)
	}
	v := parseVersionString(verStr)
	v.DetectedAt = time.Now()
	return v, nil
}

func parseVersionString(s string) *driver.DBVersion {
	v := &driver.DBVersion{
		Product: "Impala",
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
		return nil, fmt.Errorf("impala: empty force_version")
	}
	m := regexp.MustCompile(`^(\d+)\.(\d+)(?:\.(\d+))?$`).FindStringSubmatch(v)
	if m == nil {
		return nil, fmt.Errorf("impala: cannot parse force_version %q (expect like 4.5.0)", v)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return &driver.DBVersion{
		Product: "Impala", Version: "Impala (forced: " + v + ")",
		Major: major, Minor: minor, Banner: v, DetectedAt: time.Now(),
	}, nil
}
