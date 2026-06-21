package sqlserver

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

// versionRe 匹配 @@VERSION 中的版本号，如 "Microsoft SQL Server 2022 ... 16.00.1000"。
// SQL Server 用产品主版本号区分：2017=14, 2019=15, 2022=16。
var versionRe = regexp.MustCompile(`Microsoft SQL Server\s+(\d{4})`)
var engineRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// detectVersion 探测 SQL Server 版本。
func detectVersion(ctx context.Context, db *sql.DB, forceVersion string) (*driver.DBVersion, error) {
	if forceVersion != "" {
		return parseForcedVersion(forceVersion)
	}
	var verStr string
	// @@VERSION 返回多行文本，含产品年份与引擎版本号。
	if err := db.QueryRowContext(ctx, "SELECT @@VERSION").Scan(&verStr); err != nil {
		return nil, fmt.Errorf("sqlserver: cannot read @@VERSION: %w", err)
	}
	v := parseVersionString(verStr)
	v.DetectedAt = time.Now()
	return v, nil
}

// parseVersionString 解析 @@VERSION 返回串。
// SQL Server 用产品年份（2017/2019/2022）标识，引擎版本号映射：2017→14.x, 2022→16.x。
func parseVersionString(s string) *driver.DBVersion {
	v := &driver.DBVersion{
		Product: "SQL Server",
		Version: strings.TrimSpace(s),
		Banner:  s,
	}
	// 优先取产品年份
	if m := versionRe.FindStringSubmatch(s); len(m) >= 2 {
		v.Major, _ = strconv.Atoi(m[1])
	} else if m := engineRe.FindStringSubmatch(s); len(m) >= 3 {
		// 引擎主版本号映射到产品年份
		engineVer, _ := strconv.Atoi(m[1])
		v.Major = engineToYear(engineVer)
	}
	return v
}

// engineToYear 把引擎内部版本号映射为产品年份（用于 @@VERSION 含引擎号但无年份时）。
func engineToYear(engine int) int {
	switch {
	case engine >= 16:
		return 2022
	case engine == 15:
		return 2019
	case engine == 14:
		return 2017
	case engine == 13:
		return 2016
	case engine == 12:
		return 2014
	case engine == 11:
		return 2012
	default:
		return engine
	}
}

func parseForcedVersion(v string) (*driver.DBVersion, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, fmt.Errorf("sqlserver: empty force_version")
	}
	year := 0
	if m := regexp.MustCompile(`^\d{4}$`).FindString(v); m != "" {
		year, _ = strconv.Atoi(m)
	} else {
		return nil, fmt.Errorf("sqlserver: cannot parse force_version %q (expect like 2017, 2022)", v)
	}
	return &driver.DBVersion{
		Product: "SQL Server", Version: fmt.Sprintf("SQL Server (forced: %s)", v),
		Major: year, Banner: v, DetectedAt: time.Now(),
	}, nil
}
