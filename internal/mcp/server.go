// Package mcp 把 dbm-cli 的数据库能力以 MCP（Model Context Protocol）server
// 形式暴露，让 AI 客户端（Claude Desktop / Cursor 等）能直接调用。
//
// 设计原则：完全复用现有的 driver/config 抽象，零侵入 CLI。
//   - 连接经 driver.Get + NewWriteGuard 打开，自动继承 allow_write 守卫与
//     各 driver 的连接池实现，MCP 层不重做权限/连接逻辑。
//   - 每个 tool 对应一个现有 CLI 子命令的语义（见 tools.go 注释表），
//     文案与 manifest 保持一致。
//   - MCP server 是长驻进程，不混用 cobra 一次性 flag 解析；连接按 datasource
//     名缓存并 lazy 打开，进程退出时统一 Close。
//
// 启动入口见 Run。cobra 子命令封装见 internal/cli/mcp.go。
package mcp

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/mark3labs/mcp-go/server"

	"github.com/golango-cn/dbm-cli/internal/buildinfo"
	"github.com/golango-cn/dbm-cli/internal/config"
	"github.com/golango-cn/dbm-cli/internal/driver"
)

// Session 持有一个 MCP server 实例的运行期状态：配置 + 连接缓存。
//
// 它是 MCP handler 与 driver 层之间的桥梁：handler 通过 Session.Conn(name)
// 取得带写守卫的连接，无需关心 driver 查找/打开/缓存细节。
type Session struct {
	cfg        *config.File
	mu         sync.Mutex
	conns      map[string]driver.Conn // datasource 名 -> 已打开连接（带 WriteGuard）
	logger     *log.Logger
	queryLimit int // 只读查询的默认行数上限
}

// NewSession 创建一个绑定到指定配置的 MCP 会话。
// cfg 可为 nil（此时除 list_datasources 外的 tool 都会在调用时报「未配置数据源」）。
func NewSession(cfg *config.File, opts ...Option) *Session {
	s := &Session{
		cfg:        cfg,
		conns:      make(map[string]driver.Conn),
		logger:     log.New(log.Writer(), "[dbm-mcp] ", log.LstdFlags|log.Lmsgprefix),
		queryLimit: defaultQueryLimit,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Option 配置 Session。
type Option func(*Session)

// WithQueryLimit 设置只读查询的默认行数上限（防 SELECT * 拉爆大表）。
func WithQueryLimit(n int) Option {
	return func(s *Session) {
		if n > 0 {
			s.queryLimit = n
		}
	}
}

// WithLogger 替换默认 logger（默认写 stderr，带 [dbm-mcp] 前缀）。
func WithLogger(l *log.Logger) Option {
	return func(s *Session) {
		if l != nil {
			s.logger = l
		}
	}
}

// resolveDatasource 解析目标数据源名：优先显式指定，否则用 default。
func (s *Session) resolveDatasource(name string) (string, *driver.DatasourceConfig, error) {
	if s.cfg == nil || len(s.cfg.Datasources) == 0 {
		return "", nil, fmt.Errorf("no datasource configured: create a dbm-cli config first (see examples/config.yaml.example)")
	}
	if name != "" {
		ds, ok := s.cfg.Datasources[name]
		if !ok {
			return "", nil, fmt.Errorf("datasource %q not found in config; available: %v", name, s.datasourceNames())
		}
		return name, ds, nil
	}
	if s.cfg.Default != "" {
		ds, ok := s.cfg.Datasources[s.cfg.Default]
		if ok {
			return s.cfg.Default, ds, nil
		}
	}
	return "", nil, fmt.Errorf("no datasource specified and no default set; pass the 'datasource' argument (available: %v)", s.datasourceNames())
}

func (s *Session) datasourceNames() []string {
	if s.cfg == nil {
		return nil
	}
	names := make([]string, 0, len(s.cfg.Datasources))
	for n := range s.cfg.Datasources {
		names = append(names, n)
	}
	return names
}

// Conn 返回指定数据源（name 为空时用 default）的已缓存连接。
// 首次访问时打开并 Ping；后续复用。连接由 Session 管理，调用方不要 Close。
//
// 返回的 Conn 已被 WriteGuard 包裹，因此 Exec 会受 allow_write 守卫保护，
// 与 CLI 行为完全一致。
func (s *Session) Conn(ctx context.Context, name string) (driver.Conn, string, error) {
	resolved, dsCfg, err := s.resolveDatasource(name)
	if err != nil {
		return nil, "", err
	}

	s.mu.Lock()
	if c, ok := s.conns[resolved]; ok {
		s.mu.Unlock()
		return c, resolved, nil
	}
	s.mu.Unlock()

	d, err := driver.Get(dsCfg.Type)
	if err != nil {
		return nil, resolved, err
	}
	c, err := d.Open(dsCfg)
	if err != nil {
		return nil, resolved, fmt.Errorf("cannot open datasource %q: %w", resolved, err)
	}
	if err := c.Ping(ctx); err != nil {
		_ = c.Close()
		return nil, resolved, fmt.Errorf("cannot connect to datasource %q: %w", resolved, err)
	}
	guarded := driver.NewWriteGuard(c, dsCfg.AllowWrite)

	s.mu.Lock()
	// 双检：极端情况下两次并发打开同一 ds，保留先到者并关闭后来的。
	if existing, ok := s.conns[resolved]; ok {
		s.mu.Unlock()
		_ = guarded.Close()
		return existing, resolved, nil
	}
	s.conns[resolved] = guarded
	s.mu.Unlock()
	return guarded, resolved, nil
}

// Close 释放所有缓存连接。可在 server 退出时调用。
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for name, c := range s.conns {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(s.conns, name)
	}
	return firstErr
}

// cfgOf 返回数据源的 DatasourceConfig（只读访问，如读取 Type/AllowWrite）。
func (s *Session) cfgOf(name string) (*driver.DatasourceConfig, bool) {
	if s.cfg == nil {
		return nil, false
	}
	ds, ok := s.cfg.Datasources[name]
	return ds, ok
}

// BuildServer 构建并返回注册好全部 tool 的 MCPServer。
// 传输层（stdio/http）由调用方在 Run 中决定。
func (s *Session) BuildServer() *server.MCPServer {
	srv := server.NewMCPServer(
		buildinfo.AppName,
		buildinfo.Version,
		server.WithInstructions(serverInstructions()),
	)
	for _, t := range s.tools() {
		srv.AddTool(t.definition, t.handler)
	}
	return srv
}

// Run 在 stdio 上启动 MCP server，阻塞直到 ctx 取消或 stdin 关闭。
// 这是 AI 客户端（Claude Desktop 等）的标准接入方式。
func Run(ctx context.Context, cfg *config.File, opts ...Option) error {
	sess := NewSession(cfg, opts...)
	defer sess.Close()
	srv := sess.BuildServer()
	sess.logger.Printf("MCP server starting on stdio (tools=%d)", len(sess.tools()))
	return server.ServeStdio(srv)
}
