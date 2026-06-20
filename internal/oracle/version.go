package oracle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// versionBannerRe 匹配 v$version 中形如 "Oracle Database 19c ..." 的 banner，
// 并捕获其中的数字版本（如 19.0.0.0.0）。
var versionBannerRe = regexp.MustCompile(`(?i)Oracle\s+(?:Database\s+)?(\d+)(?:\.(\d+))?(?:\.(\d+))*`)

// detectVersion 探测 Oracle 版本，逐级降级但每一步都保留失败原因。
//
// 优先级：force_version（配置强制）→ v$version → product_component_version → 失败。
// 设计要点：探测失败不再「假装成功」返回 unknown 而吞掉错误。
// 当所有路径都失败时，返回带原因的错误，让调用方（及 AI）看清为什么拿不到版本，
// 而不是得到一个看似成功但毫无信息的结果。
func detectVersion(ctx context.Context, db *sql.DB, forceVersion string) (*driver.DBVersion, error) {
	if forceVersion != "" {
		return parseForcedVersion(forceVersion)
	}

	// 收集每条路径的失败原因，最终汇总进错误信息。
	var errs []string

	// 路径1：v$version banner（多数账号可见）
	if v, err := queryBannerVersion(ctx, db,
		"SELECT banner FROM v$version WHERE banner LIKE 'Oracle%'"); err != nil {
		errs = append(errs, "v$version: "+err.Error())
	} else if v != nil {
		v.DetectedAt = time.Now()
		return v, nil
	}

	// 路径2：兼容性视图（product_component_version 在所有版本可用）
	if v, err := queryBannerVersion(ctx, db,
		"SELECT product FROM product_component_version WHERE product LIKE 'Oracle%' AND ROWNUM = 1"); err != nil {
		errs = append(errs, "product_component_version: "+err.Error())
	} else if v != nil {
		v.DetectedAt = time.Now()
		return v, nil
	}

	// 所有探测路径失败：返回明确的错误，而非静默 unknown。
	// 这让 version 命令与 AI 都能看到根因（如权限不足、视图不存在）。
	return nil, fmt.Errorf("oracle: cannot determine database version; tried:\n  - %s", strings.Join(errs, "\n  - "))
}

// queryBannerVersion 执行一条返回版本 banner 的查询并解析。
// 返回 (nil, nil) 表示查询成功但未取到可用 banner（如空结果）。
func queryBannerVersion(ctx context.Context, db *sql.DB, query string) (*driver.DBVersion, error) {
	var banner string
	err := db.QueryRowContext(ctx, query).Scan(&banner)
	if err != nil {
		// sql.ErrNoRows 视为「该路径无结果」，而非硬错误
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if banner == "" {
		return nil, nil
	}
	return parseBanner(banner), nil
}

// parseBanner 解析 banner 文本，如
// "Oracle Database 19c Enterprise Edition Release 19.0.0.0.0 - 64bit Production"
func parseBanner(banner string) *driver.DBVersion {
	m := versionBannerRe.FindStringSubmatch(banner)
	if len(m) < 2 {
		return nil
	}
	major, _ := strconv.Atoi(m[1])
	minor := 0
	if len(m) >= 3 && m[2] != "" {
		minor, _ = strconv.Atoi(m[2])
	}
	return &driver.DBVersion{
		Product: "Oracle",
		Version: strings.TrimSpace(banner),
		Major:   major,
		Minor:   minor,
		Banner:  banner,
	}
}

// parseForcedVersion 解析 force_version 配置值。
// 支持两种写法：简写版本代号 "11g"/"19c"，或数字 "12.2"。
func parseForcedVersion(v string) (*driver.DBVersion, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, fmt.Errorf("oracle: empty force_version")
	}
	major, minor, ok := parseVersionToken(v)
	if !ok {
		return nil, fmt.Errorf("oracle: cannot parse force_version %q (expect forms like 11g, 19c, 12.2)", v)
	}
	return &driver.DBVersion{
		Product:    "Oracle",
		Version:    fmt.Sprintf("Oracle (forced: %s)", v),
		Major:      major,
		Minor:      minor,
		Banner:     v,
		DetectedAt: time.Now(),
	}, nil
}

// parseVersionToken 解析 "19c"/"11g"/"12.2"/"21" 等写法。
var versionCodeRe = regexp.MustCompile(`^(\d+)(?:\.(\d+))?[a-zA-Z]?$`)

func parseVersionToken(s string) (major, minor int, ok bool) {
	m := versionCodeRe.FindStringSubmatch(s)
	if len(m) < 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, 0, false
	}
	minor = 0
	if len(m) >= 3 && m[2] != "" {
		minor, _ = strconv.Atoi(m[2])
	}
	return major, minor, true
}

// isCDB 判断当前连接是否处于多租户架构（12c+）。
// 12c 以下必定非 CDB；12c+ 若能查到 v$pdbs 则为 CDB。
// 这是能力探测，失败时安全降级为 false（按非 CDB 处理），
// 不影响主流程，因此不向上抛错。
func (c *conn) isCDB(ctx context.Context) bool {
	v, err := c.Version(ctx)
	if err != nil || v.Major < 12 {
		return false
	}
	var cnt int
	if err := c.pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM v$pdbs").Scan(&cnt); err != nil {
		return false
	}
	return cnt > 0
}
