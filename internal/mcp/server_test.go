package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/golango-cn/dbm-cli/internal/config"
	"github.com/golango-cn/dbm-cli/internal/driver"
)

// fakeDriver 是一个不连真实数据库的测试用 driver。
// 它实现 Driver + Conn + MetadataProvider + PagedDataProvider，让单元测试
// 能覆盖 MCP handler 的全部行为路径（参数解析、权限、占位符、上限）。
type fakeDriver struct{ name string }

func (f *fakeDriver) Name() string              { return f.name }
func (f *fakeDriver) SupportedVersions() string { return "test" }
func (f *fakeDriver) Description() string       { return "fake driver for tests" }
func (f *fakeDriver) Open(cfg *driver.DatasourceConfig) (driver.Conn, error) {
	return &fakeConn{allowWrite: cfg.AllowWrite, typeStr: cfg.Type}, nil
}

// fakeConn 记录最后一次调用的 SQL/args，供断言。
type fakeConn struct {
	allowWrite  bool
	typeStr     string
	lastSQL     string
	lastArgs    []any
	pingFail    bool
	queryFail   error
	queryResult *driver.Result // 非空时 Query 返回它（用于构造 []byte 等 cell）
}

func (c *fakeConn) Ping(ctx context.Context) error {
	if c.pingFail {
		return errors.New("ping failed")
	}
	return nil
}
func (c *fakeConn) Close() error { return nil }
func (c *fakeConn) Version(ctx context.Context) (*driver.DBVersion, error) {
	return &driver.DBVersion{Product: "FakeDB", Version: "1.0", Major: 1, Minor: 0}, nil
}
func (c *fakeConn) Metadata() driver.MetadataProvider { return c }
func (c *fakeConn) Query(ctx context.Context, sql string, args ...any) (*driver.Result, error) {
	c.lastSQL, c.lastArgs = sql, args
	if c.queryFail != nil {
		return nil, c.queryFail
	}
	if c.queryResult != nil {
		return c.queryResult, nil
	}
	return &driver.Result{
		Columns: []string{"id", "name"},
		Rows:    [][]any{{int64(1), "alice"}, {int64(2), "bob"}, {int64(3), "carol"}},
	}, nil
}
func (c *fakeConn) Exec(ctx context.Context, sql string, args ...any) (driver.ExecResult, error) {
	c.lastSQL, c.lastArgs = sql, args
	if !c.allowWrite {
		return driver.ExecResult{}, errors.New("write operations are disabled for this datasource (set allow_write: true to enable): rejected statement")
	}
	return driver.ExecResult{RowsAffected: 1}, nil
}

// MetadataProvider 实现
func (c *fakeConn) Databases(ctx context.Context) ([]string, error) {
	return []string{"db1", "db2"}, nil
}
func (c *fakeConn) Schemas(ctx context.Context) ([]string, error) {
	return []string{"sch1", "sch2", "hr"}, nil
}
func (c *fakeConn) Tables(ctx context.Context, schema string) ([]driver.TableInfo, error) {
	return []driver.TableInfo{
		{Schema: schema, Name: "users", Type: "TABLE"},
		{Schema: schema, Name: "orders", Type: "TABLE"},
		{Schema: schema, Name: "v_users", Type: "VIEW"},
	}, nil
}
func (c *fakeConn) Columns(ctx context.Context, schema, table string) ([]driver.ColumnInfo, error) {
	return []driver.ColumnInfo{{Position: 1, Name: "id", DataType: "INT", Nullable: false}}, nil
}
func (c *fakeConn) Indexes(ctx context.Context, schema, table string) ([]driver.IndexInfo, error) {
	return []driver.IndexInfo{{Name: "pk", IsPrimary: true, Columns: []string{"id"}}}, nil
}
func (c *fakeConn) Views(ctx context.Context, schema string) ([]driver.ViewInfo, error) {
	return []driver.ViewInfo{{Schema: schema, Name: "v_users"}}, nil
}
func (c *fakeConn) RowCount(ctx context.Context, schema, table string) (int64, error) {
	return 100, nil
}

// PagedDataProvider 实现（让 sample_table 可测）
func (c *fakeConn) QueryTable(ctx context.Context, schema, table string, limit, offset int64, orderCol string) (*driver.Result, error) {
	c.lastSQL = "QueryTable"
	return &driver.Result{Columns: []string{"id"}, Rows: [][]any{{int64(1)}}}, nil
}

// ---- 测试辅助 ----

// registerFakeOnce 确保 fakeDriver 只注册一次（重复 Register 会 panic）。
var registerFakeOnce sync.Once

// newTestSession 注册一个 fakeDriver 并构造带两个数据源（一只读、一可写）的 Session。
func newTestSession(t *testing.T) *Session {
	t.Helper()
	registerFakeOnce.Do(func() {
		driver.Register(&fakeDriver{name: "fake"})
	})
	cfg := &config.File{
		Default: "ro",
		Datasources: map[string]*driver.DatasourceConfig{
			"ro": {Type: "fake", Description: "read-only replica", Host: "h", Port: 1, AllowWrite: false},
			"rw": {Type: "fake", Description: "primary writable", Host: "h", Port: 1, AllowWrite: true},
		},
	}
	return NewSession(cfg, WithQueryLimit(2))
}

// call 构造一个带指定参数的 CallToolRequest 并调用 handler，返回结果。
func call(ctx context.Context, s *Session, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	for _, td := range s.tools() {
		if td.definition.Name != toolName {
			continue
		}
		req := mcp.CallToolRequest{}
		req.Params.Name = toolName
		req.Params.Arguments = args
		return td.handler(ctx, req)
	}
	panic("unknown tool: " + toolName)
}

// resultText 解析 tool 返回的文本内容为 map（便于断言）。
func resultText(t *testing.T, r *mcp.CallToolResult) map[string]any {
	t.Helper()
	if len(r.Content) == 0 {
		t.Fatalf("no content in result")
	}
	tc, ok := r.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content is not TextContent: %T", r.Content[0])
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &m); err != nil {
		// 可能是错误消息（非 JSON），返回特殊标记。
		return map[string]any{"__text__": tc.Text}
	}
	return m
}

// ---- 测试用例 ----

func TestListDatasources(t *testing.T) {
	s := newTestSession(t)
	r, err := call(context.Background(), s, "list_datasources", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := resultText(t, r)
	arr, _ := m["datasources"].([]any)
	if len(arr) != 2 {
		t.Fatalf("expected 2 datasources, got %v", m)
	}
	// 验证密码未泄露（fakeConn 本就没密码，但确认字段集合不含 password）
	first := arr[0].(map[string]any)
	if _, hasPwd := first["password"]; hasPwd {
		t.Fatalf("datasource info must not include password: %v", first)
	}
	// 验证按 name 排序（ro < rw）
	if arr[0].(map[string]any)["name"] != "ro" || arr[1].(map[string]any)["name"] != "rw" {
		t.Fatalf("should be sorted by name: %v", arr)
	}
	// 验证 description 字段被透传（AI 据此区分同类型数据源）
	if arr[0].(map[string]any)["description"] != "read-only replica" {
		t.Fatalf("expected description on 'ro', got %v", arr[0])
	}
}

func TestResolveDefaultDatasource(t *testing.T) {
	s := newTestSession(t) // default = "ro"
	// 不传 datasource，应解析到 default "ro"
	r, _ := call(context.Background(), s, "get_version", map[string]any{})
	m := resultText(t, r)
	if m["datasource"] != "ro" {
		t.Fatalf("expected default datasource 'ro', got %v", m["datasource"])
	}
	if m["product"] != "FakeDB" {
		t.Fatalf("expected product FakeDB, got %v", m["product"])
	}
}

func TestExplicitDatasource(t *testing.T) {
	s := newTestSession(t)
	r, _ := call(context.Background(), s, "get_version", map[string]any{"datasource": "rw"})
	m := resultText(t, r)
	if m["datasource"] != "rw" {
		t.Fatalf("expected datasource 'rw', got %v", m["datasource"])
	}
}

func TestUnknownDatasource(t *testing.T) {
	s := newTestSession(t)
	r, _ := call(context.Background(), s, "get_version", map[string]any{"datasource": "nope"})
	if !r.IsError {
		t.Fatalf("expected error result for unknown datasource")
	}
}

func TestListSchemasWithLike(t *testing.T) {
	s := newTestSession(t)
	r, _ := call(context.Background(), s, "list_schemas", map[string]any{"like": "sch%"})
	m := resultText(t, r)
	names, _ := m["schemas"].([]any)
	if len(names) != 2 { // sch1, sch2
		t.Fatalf("expected 2 schemas matching 'sch%%', got %v", names)
	}
}

func TestQueryPlaceholderNormalization(t *testing.T) {
	s := newTestSession(t)
	// Conn 是 lazy 打开的，先触发一次连接建立。
	if _, _, err := s.Conn(context.Background(), "rw"); err != nil {
		t.Fatalf("connect rw: %v", err)
	}
	fc := getFakeConn(t, s, "rw")

	_, err := call(context.Background(), s, "query", map[string]any{
		"datasource": "rw",
		"sql":        "SELECT * FROM t WHERE id=? AND name=?",
		"params":     []any{"100", "alice"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// fake 驱动默认占位符风格为 ?（未知驱动走 StyleQuestion），SQL 原样。
	if fc.lastSQL != "SELECT * FROM t WHERE id=? AND name=?" {
		t.Fatalf("SQL not normalized as expected: %q", fc.lastSQL)
	}
	if len(fc.lastArgs) != 2 {
		t.Fatalf("expected 2 args, got %d", len(fc.lastArgs))
	}
	if fc.lastArgs[0] != int64(100) { // "100" 应被 coerce 为 int64
		t.Fatalf("expected first arg int64(100), got %T %v", fc.lastArgs[0], fc.lastArgs[0])
	}
	if fc.lastArgs[1] != "alice" {
		t.Fatalf("expected second arg string alice, got %v", fc.lastArgs[1])
	}
}

func TestQueryLimitCap(t *testing.T) {
	s := newTestSession(t) // WithQueryLimit(2)
	// fakeConn.Query 返回 3 行；Session 默认上限 2 应截断。
	r, _ := call(context.Background(), s, "query", map[string]any{
		"datasource": "rw",
		"sql":        "SELECT * FROM t",
		// 不传 limit，使用 Session 默认上限
	})
	m := resultText(t, r)
	rows, _ := m["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (default cap), got %d", len(rows))
	}
}

func TestQueryExplicitLimit(t *testing.T) {
	s := newTestSession(t)
	r, _ := call(context.Background(), s, "query", map[string]any{
		"datasource": "rw",
		"sql":        "SELECT * FROM t",
		"limit":      float64(1),
	})
	m := resultText(t, r)
	rows, _ := m["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (explicit limit), got %d", len(rows))
	}
}

func TestQueryRejectsWriteSQL(t *testing.T) {
	s := newTestSession(t)
	r, _ := call(context.Background(), s, "query", map[string]any{
		"datasource": "rw",
		"sql":        "DELETE FROM t WHERE id=1",
	})
	if !r.IsError {
		t.Fatalf("query tool must reject non-read-only SQL")
	}
}

// TestQueryAcceptsShowCreateTable 是只读元数据关键字（SHOW/DESCRIBE/EXPLAIN）
// 补全后对 MCP query 路径的回归保护：SHOW CREATE TABLE 必须被当作只读、
// 走 conn.Query 并返回结果集，而不是被拒绝或走 Exec 丢弃结果。
func TestQueryAcceptsShowCreateTable(t *testing.T) {
	s := newTestSession(t)
	if _, _, err := s.Conn(context.Background(), "rw"); err != nil {
		t.Fatalf("connect rw: %v", err)
	}
	fc := getFakeConn(t, s, "rw")

	r, _ := call(context.Background(), s, "query", map[string]any{
		"datasource": "rw",
		"sql":        "SHOW CREATE TABLE ods.ODS_EDS_LOT_HIST_MOD_HI",
	})
	if r.IsError {
		t.Fatalf("SHOW CREATE TABLE must be accepted as read-only, got error: %v", r.Content)
	}
	// 必须真的走了 Query（而非 Exec 丢弃结果）。
	if fc.lastSQL != "SHOW CREATE TABLE ods.ODS_EDS_LOT_HIST_MOD_HI" {
		t.Fatalf("expected SQL dispatched to Query, got %q", fc.lastSQL)
	}
	m := resultText(t, r)
	if _, ok := m["rows"]; !ok {
		t.Fatalf("expected rows in result, got %v", m)
	}
}

func TestExecuteReadOnlyRejected(t *testing.T) {
	s := newTestSession(t)
	r, _ := call(context.Background(), s, "execute", map[string]any{
		"datasource": "rw",
		"sql":        "SELECT 1",
	})
	if !r.IsError {
		t.Fatalf("execute tool must reject read-only SQL")
	}
}

func TestExecuteRejectedOnReadOnlyDatasource(t *testing.T) {
	s := newTestSession(t) // ro 数据源 allow_write=false
	r, _ := call(context.Background(), s, "execute", map[string]any{
		"datasource": "ro",
		"sql":        "INSERT INTO t VALUES (1)",
	})
	if !r.IsError {
		t.Fatalf("execute must be rejected on read-only datasource")
	}
	tc := r.Content[0].(mcp.TextContent).Text
	if !strings.Contains(tc, "read-only") {
		t.Fatalf("expected read-only message, got: %s", tc)
	}
}

func TestExecuteDestructiveNeedsConfirm(t *testing.T) {
	s := newTestSession(t) // rw 允许写
	// DROP 不传 confirm_destructive → 拒绝
	r, _ := call(context.Background(), s, "execute", map[string]any{
		"datasource": "rw",
		"sql":        "DROP TABLE t",
	})
	if !r.IsError {
		t.Fatalf("destructive statement must be rejected without confirm_destructive")
	}
	tc := r.Content[0].(mcp.TextContent).Text
	if !strings.Contains(tc, "confirm_destructive") {
		t.Fatalf("expected hint about confirm_destructive, got: %s", tc)
	}
}

func TestExecuteDestructiveWithConfirm(t *testing.T) {
	s := newTestSession(t)
	// DROP 传 confirm_destructive=true → 允许执行（fakeConn 不会真删）
	r, _ := call(context.Background(), s, "execute", map[string]any{
		"datasource":          "rw",
		"sql":                 "DROP TABLE t",
		"confirm_destructive": true,
	})
	if r.IsError {
		t.Fatalf("expected success with confirm, got error: %v", r.Content)
	}
}

func TestExecuteNonDestructiveWrite(t *testing.T) {
	s := newTestSession(t)
	// INSERT 不需要 confirm
	r, _ := call(context.Background(), s, "execute", map[string]any{
		"datasource": "rw",
		"sql":        "INSERT INTO t VALUES (1)",
	})
	if r.IsError {
		t.Fatalf("non-destructive write should succeed, got: %v", r.Content)
	}
	m := resultText(t, r)
	if m["rows_affected"] != float64(1) {
		t.Fatalf("expected rows_affected=1, got %v", m["rows_affected"])
	}
}

// TestQueryByteSliceNotBase64 是对 resultToJSON 归一化的回归保护：
// database/sql 把变长字符串列返回为 []byte，而 Go 的 encoding/json 默认会把
// []byte 当作 base64 字节流输出（`impala` → `aW1wYWxh`）。MCP 层必须把
// []byte 还原为字符串，否则 AI 看到的是乱码。
func TestQueryByteSliceNotBase64(t *testing.T) {
	s := newTestSession(t)
	if _, _, err := s.Conn(context.Background(), "rw"); err != nil {
		t.Fatalf("connect rw: %v", err)
	}
	fc := getFakeConn(t, s, "rw")
	// 模拟真实驱动：VARCHAR/TEXT 列以 []byte 返回。
	fc.queryResult = &driver.Result{
		Columns: []string{"id", "name", "cfg"},
		Rows: [][]any{
			{int64(1), []byte("impala"), []byte(`{"speed":1}`)},
			{int64(2), []byte("clickhouse"), nil},
		},
	}

	r, _ := call(context.Background(), s, "query", map[string]any{
		"datasource": "rw",
		"sql":        "SELECT * FROM t",
	})
	if r.IsError {
		t.Fatalf("unexpected error: %v", r.Content)
	}
	m := resultText(t, r)
	rows, _ := m["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	row0 := rows[0].([]any)
	// 关键断言：[]byte 必须以原始字符串返回，而非 base64。
	if row0[1] != "impala" {
		t.Fatalf("[]byte cell must be returned as plain string, got %v (type %T)", row0[1], row0[1])
	}
	if row0[2] != `{"speed":1}` {
		t.Fatalf("[]byte JSON cell must be returned as plain string, got %v", row0[2])
	}
	// nil 仍为 nil（JSON null），不能被改写。
	row1 := rows[1].([]any)
	if row1[2] != nil {
		t.Fatalf("nil cell must stay nil, got %v", row1[2])
	}
}

func TestSampleTableRequiresTableArg(t *testing.T) {
	s := newTestSession(t)
	r, _ := call(context.Background(), s, "sample_table", map[string]any{
		"datasource": "rw",
	})
	if !r.IsError {
		t.Fatalf("missing 'table' should be an error")
	}
}

func TestDescribeTableMissingTable(t *testing.T) {
	s := newTestSession(t)
	r, _ := call(context.Background(), s, "describe_table", map[string]any{"datasource": "rw"})
	if !r.IsError {
		t.Fatalf("missing 'table' should be an error")
	}
}

func TestParamCountMismatch(t *testing.T) {
	s := newTestSession(t)
	r, _ := call(context.Background(), s, "query", map[string]any{
		"datasource": "rw",
		"sql":        "SELECT * FROM t WHERE id=?",
		"params":     []any{"1", "2"}, // 多了一个
	})
	if !r.IsError {
		t.Fatalf("param count mismatch should be an error")
	}
}

// getFakeConn 从 Session 缓存中取出底层 fakeConn（穿透 WriteGuard）。
func getFakeConn(t *testing.T, s *Session, name string) *fakeConn {
	t.Helper()
	s.mu.Lock()
	g, ok := s.conns[name].(*driver.WriteGuard)
	s.mu.Unlock()
	if !ok {
		t.Fatalf("conn for %q is not a *WriteGuard", name)
	}
	fc, ok := g.Unwrap().(*fakeConn)
	if !ok {
		t.Fatalf("underlying conn is not *fakeConn")
	}
	return fc
}
