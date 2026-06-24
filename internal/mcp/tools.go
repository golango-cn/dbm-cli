// tools.go 定义 MCP server 暴露的全部 tool 及其 handler。
//
// 每个 tool 与一个现有 CLI 子命令一一对应，保持语义与文案一致：
//
//   list_datasources  <-> dbm-cli datasources      （不连库）
//   get_version       <-> dbm-cli version
//   list_databases    <-> dbm-cli databases
//   list_schemas      <-> dbm-cli schemas
//   list_tables       <-> dbm-cli tables
//   describe_table    <-> dbm-cli columns
//   list_indexes      <-> dbm-cli indexes
//   list_views        <-> dbm-cli views
//   sample_table      <-> dbm-cli table（分页读取）
//   query             <-> dbm-cli query（只读）
//   execute           <-> dbm-cli query（写，受 allow_write 守卫）
//
// 结果以 JSON 文本返回（mcp.NewToolResultText），与 CLI 的 -o json 口径一致，
// 便于 AI 客户端解析。错误统一用 mcp.NewToolResultErrorFromErr 标记 isError=true。
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/golango-cn/dbm-cli/internal/driver"
	"github.com/golango-cn/dbm-cli/internal/query"
)

// defaultQueryLimit 与 CLI 的 query 命令保持一致（见 internal/cli/query.go）。
const defaultQueryLimit = 1000

// toolDef 把一个 MCP tool 的定义与 handler 绑在一起，便于在 BuildServer 中遍历注册。
type toolDef struct {
	definition mcp.Tool
	handler    server.ToolHandlerFunc
}

// tools 返回全部 tool 定义。注册逻辑见 server.go 的 BuildServer。
func (s *Session) tools() []toolDef {
	return []toolDef{
		// ---- 不连库：数据源发现 ----
		{
			definition: mcp.NewTool("list_datasources",
				mcp.WithDescription(`列出 dbm-cli 配置中已定义的全部数据源（脱敏，不含密码）。
不需要任何参数，也不连接任何数据库。AI 在调用其它 tool 前，应先调用本 tool 了解可用数据源名与各自类型/是否允许写。`),
			),
			handler: s.handleListDatasources,
		},
		// ---- 版本 ----
		{
			definition: mcp.NewTool("get_version",
				mcp.WithDescription(`查询指定数据源的数据库产品名与版本（如 Oracle 19c、MySQL 8.0）。
用于在执行特定方言 SQL 前确认引擎版本。`),
				mcp.WithString("datasource", mcp.Description("数据源名；省略则用配置中的 default")),
			),
			handler: s.handleGetVersion,
		},
		// ---- 元数据：库 / schema / 表 / 列 / 索引 / 视图 ----
		{
			definition: mcp.NewTool("list_databases",
				mcp.WithDescription(`列出数据库实例下的「库」。
Oracle CDB 返回各 PDB 名称；MySQL/PostgreSQL 返回当前实例可访问的数据库名。`),
				mcp.WithString("datasource", mcp.Description("数据源名；省略则用 default")),
			),
			handler: s.handleListDatabases,
		},
		{
			definition: mcp.NewTool("list_schemas",
				mcp.WithDescription(`列出 schema / user 列表（Oracle 即 user，MySQL 即 database，PostgreSQL 即 schema）。`),
				mcp.WithString("datasource", mcp.Description("数据源名；省略则用 default")),
				mcp.WithString("like", mcp.Description("可选：名称模糊匹配，SQL LIKE 语法（% 与 _）")),
			),
			handler: s.handleListSchemas,
		},
		{
			definition: mcp.NewTool("list_tables",
				mcp.WithDescription(`列出指定 schema 下的表（含视图/物化视图等类型）。`),
				mcp.WithString("datasource", mcp.Description("数据源名；省略则用 default")),
				mcp.WithString("schema", mcp.Description("schema 名；省略则用当前用户")),
				mcp.WithString("like", mcp.Description("可选：表名模糊匹配，SQL LIKE 语法")),
			),
			handler: s.handleListTables,
		},
		{
			definition: mcp.NewTool("describe_table",
				mcp.WithDescription(`查看某张表的列定义（列名、类型、长度、是否可空、默认值、注释等）。
适合在写 SQL 前确认列名与类型。`),
				mcp.WithString("datasource", mcp.Description("数据源名；省略则用 default")),
				mcp.WithString("table", mcp.Required(), mcp.Description("表名")),
				mcp.WithString("schema", mcp.Description("schema 名；省略则用当前用户")),
			),
			handler: s.handleDescribeTable,
		},
		{
			definition: mcp.NewTool("list_indexes",
				mcp.WithDescription(`查看某张表的索引（名称、是否唯一、是否主键、列）。`),
				mcp.WithString("datasource", mcp.Description("数据源名；省略则用 default")),
				mcp.WithString("table", mcp.Required(), mcp.Description("表名")),
				mcp.WithString("schema", mcp.Description("schema 名；省略则用当前用户")),
			),
			handler: s.handleListIndexes,
		},
		{
			definition: mcp.NewTool("list_views",
				mcp.WithDescription(`列出指定 schema 下的视图。`),
				mcp.WithString("datasource", mcp.Description("数据源名；省略则用 default")),
				mcp.WithString("schema", mcp.Description("schema 名；省略则用当前用户")),
			),
			handler: s.handleListViews,
		},
		// ---- 数据：分页采样 ----
		{
			definition: mcp.NewTool("sample_table",
				mcp.WithDescription(`分页读取某张表的数据（自动用该数据库的原生分页方言）。
适合快速了解表内容的样本，而非执行自定义 SQL。自定义查询请用 query tool。`),
				mcp.WithString("datasource", mcp.Description("数据源名；省略则用 default")),
				mcp.WithString("table", mcp.Required(), mcp.Description("表名")),
				mcp.WithString("schema", mcp.Description("schema 名；省略则用当前用户")),
				mcp.WithNumber("limit", mcp.DefaultNumber(100), mcp.Description("返回行数，默认 100")),
				mcp.WithNumber("offset", mcp.DefaultNumber(0), mcp.Description("跳过行数，默认 0")),
				mcp.WithString("order_by", mcp.Description("可选：排序列名")),
			),
			handler: s.handleSampleTable,
		},
		// ---- SQL：只读查询 ----
		{
			definition: mcp.NewTool("query",
				mcp.WithDescription(`执行只读 SQL 查询（SELECT / WITH / SHOW / DESCRIBE / EXPLAIN）并返回结果集。
占位符统一用 ?，自动按当前引擎转换为原生占位符（MySQL/ClickHouse=?、PostgreSQL=$1、Oracle=:1）。
结果集默认最多返回 `+strconv.Itoa(defaultQueryLimit)+` 行（可用 limit 调整），防止误查大表。`),
				mcp.WithString("datasource", mcp.Description("数据源名；省略则用 default")),
				mcp.WithString("sql", mcp.Required(), mcp.Description("只读 SQL 语句（SELECT/WITH/SHOW/DESCRIBE/EXPLAIN）")),
				mcp.WithArray("params",
					mcp.Description("可选：按顺序绑定到 ? 占位符的参数值（字符串或数字，按需转换）"),
					mcp.Items(map[string]any{"type": "string"}),
				),
				mcp.WithNumber("limit", mcp.DefaultNumber(defaultQueryLimit), mcp.Description("返回行数上限，<=0 表示不限制")),
			),
			handler: s.handleQuery,
		},
		// ---- SQL：写操作（受 allow_write 守卫）----
		{
			definition: mcp.NewTool("execute",
				mcp.WithDescription(`执行写 SQL（INSERT/UPDATE/DELETE/DDL），受数据源的 allow_write 守卫保护：
若数据源 allow_write=false 则直接拒绝。危险语句（DROP/TRUNCATE、无 WHERE 的 DELETE/UPDATE）必须显式传 confirm_destructive=true 才会执行。
MCP 场景无交互终端，因此比 CLI 更保守——这是自动化场景的安全预期。`),
				mcp.WithString("datasource", mcp.Description("数据源名；省略则用 default")),
				mcp.WithString("sql", mcp.Required(), mcp.Description("写 SQL 语句")),
				mcp.WithArray("params",
					mcp.Description("可选：按顺序绑定到 ? 占位符的参数值"),
					mcp.Items(map[string]any{"type": "string"}),
				),
				mcp.WithBoolean("confirm_destructive", mcp.Description("对危险语句（DROP/TRUNCATE/无 WHERE 的 DELETE/UPDATE）的二次确认，true=已知晓风险并执行")),
			),
			handler: s.handleExecute,
		},
	}
}

// serverInstructions 是 MCP server 的全局指引，出现在 initialize 响应里，
// 帮助 AI 理解工具集合的使用模式（如「先 list_datasources 再下钻」）。
func serverInstructions() string {
	return `dbm-cli MCP server exposes a database via a set of read/write tools.

Recommended exploration flow:
  1. list_datasources  — learn what's configured (no connection needed)
  2. get_version       — confirm the engine/version of the chosen datasource
  3. list_databases / list_schemas  — discover the namespace hierarchy
  4. list_tables       — find tables in a schema
  5. describe_table    — inspect a table's columns before writing SQL
  6. sample_table / query — read data

Safety:
  - Every datasource has an allow_write switch (read-only by default).
  - The execute tool is rejected on read-only datasources.
  - Destructive statements (DROP/TRUNCATE, or DELETE/UPDATE without WHERE)
    require confirm_destructive=true.
  - Use parameterized SQL (? placeholders) to avoid injection.`
}

// ============================================================================
// handlers —— 每个 handler 只做三件事：解析参数 → 调 driver → 序列化结果
// ============================================================================

// args 是从 CallToolRequest 中解析出的参数集合（全部可选，按 tool 需要取用）。
type args struct {
	datasource         string
	schema             string
	table              string
	like               string
	sql                string
	params             []string
	limit              int64
	offset             int64
	orderBy            string
	confirmDestructive bool
	hasLimit           bool
}

// parseArgs 从请求中宽松提取参数；缺失字段用零值。
func parseArgs(req mcp.CallToolRequest) args {
	a := args{limit: 100, offset: 0}
	m := req.GetArguments()
	if m == nil {
		return a
	}
	a.datasource = getString(m, "datasource")
	a.schema = getString(m, "schema")
	a.table = getString(m, "table")
	a.like = getString(m, "like")
	a.sql = getString(m, "sql")
	a.orderBy = getString(m, "order_by")
	if v, ok := getBool(m, "confirm_destructive"); ok {
		a.confirmDestructive = v
	}
	if v, ok := getInt64(m, "limit"); ok {
		a.limit = v
		a.hasLimit = true
	}
	if v, ok := getInt64(m, "offset"); ok {
		a.offset = v
	}
	a.params = getStringSlice(m, "params")
	return a
}

// ----- 数据源发现 -----

func (s *Session) handleListDatasources(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.cfg == nil || len(s.cfg.Datasources) == 0 {
		return mcp.NewToolResultError("no datasource configured: create a dbm-cli config file first (see examples/config.yaml.example)"), nil
	}
	type dsInfo struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Description string `json:"description,omitempty"` // 数据源的人类/AI 可读职责说明，帮助区分同类型数据源
		Host        string `json:"host"`
		Port        int    `json:"port"`
		AllowWrite  bool   `json:"allow_write"`
		IsDefault   bool   `json:"is_default"`
	}
	out := make([]dsInfo, 0, len(s.cfg.Datasources))
	for name, ds := range s.cfg.Datasources {
		out = append(out, dsInfo{
			Name: name, Type: ds.Type, Description: ds.Description,
			Host: ds.Host, Port: ds.Port,
			AllowWrite: ds.AllowWrite, IsDefault: name == s.cfg.Default,
		})
	}
	// 按 name 排序，保证输出稳定（便于 AI 比较/diff）。
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return jsonResult(map[string]any{"datasources": out})
}

// ----- 版本 -----

func (s *Session) handleGetVersion(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	a := parseArgs(req)
	conn, name, err := s.Conn(ctx, a.datasource)
	if err != nil {
		return mcp.NewToolResultErrorFromErr(fmt.Sprintf("connect datasource: %v", err), nil), nil
	}
	v, err := conn.Version(ctx)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("get version", err), nil
	}
	return jsonResult(map[string]any{
		"datasource": name,
		"product":    v.Product,
		"version":    v.Version,
		"major":      v.Major,
		"minor":      v.Minor,
	})
}

// ----- 元数据 -----

func (s *Session) handleListDatabases(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	a := parseArgs(req)
	conn, _, err := s.Conn(ctx, a.datasource)
	if err != nil {
		return mcp.NewToolResultErrorFromErr(fmt.Sprintf("connect datasource: %v", err), nil), nil
	}
	names, err := conn.Metadata().Databases(ctx)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("list databases", err), nil
	}
	return jsonResult(map[string]any{"databases": names})
}

func (s *Session) handleListSchemas(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	a := parseArgs(req)
	conn, _, err := s.Conn(ctx, a.datasource)
	if err != nil {
		return mcp.NewToolResultErrorFromErr(fmt.Sprintf("connect datasource: %v", err), nil), nil
	}
	names, err := conn.Metadata().Schemas(ctx)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("list schemas", err), nil
	}
	if a.like != "" {
		names = filterLike(names, a.like)
	}
	return jsonResult(map[string]any{"schemas": names})
}

func (s *Session) handleListTables(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	a := parseArgs(req)
	conn, _, err := s.Conn(ctx, a.datasource)
	if err != nil {
		return mcp.NewToolResultErrorFromErr(fmt.Sprintf("connect datasource: %v", err), nil), nil
	}
	tables, err := conn.Metadata().Tables(ctx, a.schema)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("list tables", err), nil
	}
	type t struct {
		Schema  string `json:"schema"`
		Name    string `json:"name"`
		Type    string `json:"type"`
		Comment string `json:"comment,omitempty"`
	}
	out := make([]t, 0, len(tables))
	for _, ti := range tables {
		if a.like != "" && !matchLike(ti.Name, a.like) {
			continue
		}
		out = append(out, t{Schema: ti.Schema, Name: ti.Name, Type: ti.Type, Comment: ti.Comment})
	}
	return jsonResult(map[string]any{"tables": out})
}

func (s *Session) handleDescribeTable(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	a := parseArgs(req)
	if a.table == "" {
		return mcp.NewToolResultError("missing required argument 'table'"), nil
	}
	conn, _, err := s.Conn(ctx, a.datasource)
	if err != nil {
		return mcp.NewToolResultErrorFromErr(fmt.Sprintf("connect datasource: %v", err), nil), nil
	}
	cols, err := conn.Metadata().Columns(ctx, a.schema, a.table)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("describe table", err), nil
	}
	type c struct {
		Position     int    `json:"position"`
		Name         string `json:"name"`
		DataType     string `json:"data_type"`
		Length       int64  `json:"length,omitempty"`
		Precision    int    `json:"precision,omitempty"`
		Scale        int    `json:"scale,omitempty"`
		Nullable     bool   `json:"nullable"`
		DefaultValue string `json:"default_value,omitempty"`
		Comment      string `json:"comment,omitempty"`
	}
	out := make([]c, 0, len(cols))
	for _, ci := range cols {
		out = append(out, c{
			Position: ci.Position, Name: ci.Name, DataType: ci.DataType,
			Length: ci.Length, Precision: ci.Precision, Scale: ci.Scale,
			Nullable: ci.Nullable, DefaultValue: ci.DefaultValue, Comment: ci.Comment,
		})
	}
	return jsonResult(map[string]any{"table": a.table, "schema": a.schema, "columns": out})
}

func (s *Session) handleListIndexes(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	a := parseArgs(req)
	if a.table == "" {
		return mcp.NewToolResultError("missing required argument 'table'"), nil
	}
	conn, _, err := s.Conn(ctx, a.datasource)
	if err != nil {
		return mcp.NewToolResultErrorFromErr(fmt.Sprintf("connect datasource: %v", err), nil), nil
	}
	idxs, err := conn.Metadata().Indexes(ctx, a.schema, a.table)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("list indexes", err), nil
	}
	type idx struct {
		Name      string   `json:"name"`
		IsUnique  bool     `json:"is_unique"`
		IsPrimary bool     `json:"is_primary"`
		Columns   []string `json:"columns"`
	}
	out := make([]idx, 0, len(idxs))
	for _, i := range idxs {
		out = append(out, idx{Name: i.Name, IsUnique: i.IsUnique, IsPrimary: i.IsPrimary, Columns: i.Columns})
	}
	return jsonResult(map[string]any{"table": a.table, "schema": a.schema, "indexes": out})
}

func (s *Session) handleListViews(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	a := parseArgs(req)
	conn, _, err := s.Conn(ctx, a.datasource)
	if err != nil {
		return mcp.NewToolResultErrorFromErr(fmt.Sprintf("connect datasource: %v", err), nil), nil
	}
	views, err := conn.Metadata().Views(ctx, a.schema)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("list views", err), nil
	}
	type v struct {
		Schema string `json:"schema"`
		Name   string `json:"name"`
	}
	out := make([]v, 0, len(views))
	for _, vi := range views {
		out = append(out, v{Schema: vi.Schema, Name: vi.Name})
	}
	return jsonResult(map[string]any{"views": out})
}

// ----- 分页采样 -----

func (s *Session) handleSampleTable(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	a := parseArgs(req)
	if a.table == "" {
		return mcp.NewToolResultError("missing required argument 'table'"), nil
	}
	conn, _, err := s.Conn(ctx, a.datasource)
	if err != nil {
		return mcp.NewToolResultErrorFromErr(fmt.Sprintf("connect datasource: %v", err), nil), nil
	}
	// WriteGuard 包裹了真实 Conn；取底层做能力断言（与 cli/table.go 一致）。
	realConn := conn
	if g, ok := conn.(*driver.WriteGuard); ok {
		realConn = g.Unwrap()
	}
	p, ok := realConn.(driver.PagedDataProvider)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf(
			"this datasource (type %q) does not support paged table reads; use the query tool instead",
			s.driverTypeOf(a.datasource))), nil
	}
	res, err := p.QueryTable(ctx, a.schema, a.table, a.limit, a.offset, a.orderBy)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("sample table", err), nil
	}
	return resultToJSON(res)
}

// ----- 查询 / 写 -----

func (s *Session) handleQuery(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	a := parseArgs(req)
	if a.sql == "" {
		return mcp.NewToolResultError("missing required argument 'sql'"), nil
	}
	conn, name, err := s.Conn(ctx, a.datasource)
	if err != nil {
		return mcp.NewToolResultErrorFromErr(fmt.Sprintf("connect datasource: %v", err), nil), nil
	}

	kind := query.Classify(a.sql)
	if !kind.IsReadOnly() {
		return mcp.NewToolResultError(fmt.Sprintf(
			"the query tool only accepts read-only SQL (SELECT/WITH/SHOW/DESCRIBE/EXPLAIN); got %s. Use the execute tool for writes.",
			kind)), nil
	}

	sqlStr, sqlArgs, err := s.normalizeSQL(a, name)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("prepare query", err), nil
	}

	res, err := conn.Query(ctx, sqlStr, sqlArgs...)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("query", err), nil
	}
	limit := a.limit
	if !a.hasLimit {
		limit = int64(s.queryLimit) // 未显式传则用 Session 默认上限
	}
	res = capRows(res, int(limit))
	return resultToJSON(res)
}

func (s *Session) handleExecute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	a := parseArgs(req)
	if a.sql == "" {
		return mcp.NewToolResultError("missing required argument 'sql'"), nil
	}
	conn, name, err := s.Conn(ctx, a.datasource)
	if err != nil {
		return mcp.NewToolResultErrorFromErr(fmt.Sprintf("connect datasource: %v", err), nil), nil
	}

	kind := query.Classify(a.sql)
	if kind.IsReadOnly() {
		return mcp.NewToolResultError(fmt.Sprintf(
			"the execute tool is for write SQL; got read-only %s. Use the query tool instead.",
			kind)), nil
	}

	// allow_write 守卫由 WriteGuard.Exec 内部强制（与 CLI 完全一致），
	// 这里先做一层清晰提示：让 AI 看到具体拒绝原因而非裸 error。
	if ds, ok := s.cfgOf(name); ok && !ds.AllowWrite {
		return mcp.NewToolResultError(fmt.Sprintf(
			"rejected: datasource %q is read-only (allow_write=false). Set allow_write: true in config to enable writes.",
			name)), nil
	}

	// MCP 无交互终端：危险语句要求显式 confirm_destructive=true。
	if query.IsDestructive(a.sql) && !a.confirmDestructive {
		return mcp.NewToolResultError(fmt.Sprintf(
			"rejected: statement classified as destructive (%s). Pass confirm_destructive=true to acknowledge and execute.\nSQL: %s",
			describeDestructive(a.sql), truncateSQL(a.sql, 200))), nil
	}

	sqlStr, sqlArgs, err := s.normalizeSQL(a, name)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("prepare execute", err), nil
	}

	res, err := conn.Exec(ctx, sqlStr, sqlArgs...)
	if err != nil {
		// 双保险：如果 WriteGuard 因 allow_write 拒绝，给出友好提示。
		if strings.Contains(err.Error(), "write operations are disabled") {
			return mcp.NewToolResultError(fmt.Sprintf(
				"rejected: datasource %q is read-only (allow_write=false).", name)), nil
		}
		return mcp.NewToolResultErrorFromErr("execute", err), nil
	}
	return jsonResult(map[string]any{
		"ok":            true,
		"rows_affected": res.RowsAffected,
	})
}

// ============================================================================
// 辅助：参数解析、占位符归一化、结果序列化、LIKE 匹配
// ============================================================================

// normalizeSQL 处理占位符归一化（复用 internal/query）与参数类型推断。
// name 是已解析的数据源名，用于确定引擎占位符风格。
func (s *Session) normalizeSQL(a args, name string) (string, []any, error) {
	if len(a.params) == 0 {
		return a.sql, nil, nil
	}
	ds, ok := s.cfgOf(name)
	driverType := ""
	if ok {
		driverType = ds.Type
	}
	need := query.CountPlaceholders(a.sql)
	if len(a.params) != need {
		return "", nil, fmt.Errorf("param count %d does not match placeholder '?' count %d", len(a.params), need)
	}
	sqlArgs := make([]any, 0, len(a.params))
	for _, p := range a.params {
		sqlArgs = append(sqlArgs, coerceParam(p))
	}
	style := query.PlaceholderStyleFor(driverType)
	return query.NormalizePlaceholders(a.sql, style), sqlArgs, nil
}

// driverTypeOf 返回数据源的驱动类型名（用于错误提示）。
func (s *Session) driverTypeOf(name string) string {
	if ds, ok := s.cfgOf(name); ok {
		return ds.Type
	}
	return "unknown"
}

// coerceParam 与 cli/coerceParam 同语义：纯整数串转 int64，否则保留字符串。
func coerceParam(p string) any {
	if n, err := strconv.ParseInt(p, 10, 64); err == nil {
		return n
	}
	return p
}

// describeDestructive 给出危险语句的人类可读理由。
func describeDestructive(sql string) string {
	switch query.Classify(sql) {
	case query.KindDDL:
		return "DROP/TRUNCATE permanently destroys objects or data"
	default:
		return "DELETE/UPDATE without WHERE clause"
	}
}

// truncateSQL 截断长 SQL 用于提示。
func truncateSQL(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// resultToJSON 把 driver.Result 转成「columns + rows」的 JSON 文本。
//
// 必须对每个 cell 做 normalizeJSONCell 归一化：database/sql 把变长字符串列
// （ClickHouse/Impala/MySQL 的 VARCHAR/TEXT 等）返回为 []byte，而 Go 的
// encoding/json 会把 []byte 当作 base64 编码的字节流输出——这会让 AI 看到
// `impala` 变成 `aW1wYWxh`。这里把 []byte 还原为字符串，与 CLI -o json 的
// normalizeJSONValue 行为保持一致。
func resultToJSON(res *driver.Result) (*mcp.CallToolResult, error) {
	rows := make([][]any, len(res.Rows))
	for i, row := range res.Rows {
		cells := make([]any, len(row))
		for j, v := range row {
			cells[j] = normalizeJSONCell(v)
		}
		rows[i] = cells
	}
	out := map[string]any{
		"columns": res.Columns,
		"rows":    rows,
	}
	if len(res.Rows) == 0 {
		out["rows"] = []any{} // 避免 nil 渲染成 null，让 AI 明确是空集
	}
	return jsonResult(out)
}

// normalizeJSONCell 与 cli/output 的 normalizeJSONValue 同语义：
// 把 []byte（变长字符串列）还原为 string，避免 json.Marshal 做 base64 编码。
// 其它类型原样返回（time.Time 等由 json.Marshal 自行处理）。
func normalizeJSONCell(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []byte:
		return string(x)
	}
	return v
}

// jsonResult 把任意值序列化为缩进 JSON 文本，作为 MCP tool 结果返回。
func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("marshal result", err), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

// ---------- 参数提取小工具 ----------

func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getBool(m map[string]any, key string) (bool, bool) {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b, true
		}
	}
	return false, false
}

func getInt64(m map[string]any, key string) (int64, bool) {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int64(n), true // JSON 数字默认解析为 float64
		case int:
			return int64(n), true
		case int64:
			return n, true
		}
	}
	return 0, false
}

// getStringSlice 兼容 JSON 数组（元素可为 string 或 number）。
func getStringSlice(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		switch x := e.(type) {
		case string:
			out = append(out, x)
		case float64:
			out = append(out, strconv.FormatFloat(x, 'f', -1, 64))
		case bool:
			out = append(out, strconv.FormatBool(x))
		default:
			b, _ := json.Marshal(x)
			out = append(out, string(b))
		}
	}
	return out
}

// ---------- LIKE 与排序（与 cli/metadata.go 的 matchLike 同语义）----------

// matchLike 用 SQL LIKE 风格（% 与 _）做匹配，大小写不敏感。
// 与 internal/cli/metadata.go 中实现保持一致，确保 CLI 与 MCP 行为统一。
func matchLike(s, pattern string) bool {
	var b strings.Builder
	b.WriteString("(?i)^")
	for _, r := range pattern {
		switch r {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteByte('.')
		default:
			if strings.ContainsRune(`\.+*?()|[]{}^$`, r) {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('$')
	matched, _ := regexp.MatchString(b.String(), s)
	return matched
}

// filterLike 对一个字符串切片做 LIKE 过滤。
func filterLike(names []string, pattern string) []string {
	out := names[:0]
	for _, n := range names {
		if matchLike(n, pattern) {
			out = append(out, n)
		}
	}
	return out
}

// capRows 按上限截断结果集行数。limit<=0 表示不限制。
// 与 internal/cli/query.go 的 capRows 同语义。
func capRows(res *driver.Result, limit int) *driver.Result {
	if limit <= 0 || len(res.Rows) <= limit {
		return res
	}
	res.Rows = res.Rows[:limit]
	return res
}
