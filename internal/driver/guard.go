package driver

import (
	"context"
	"errors"
	"fmt"
)

// ErrWriteDisabled 表示当前数据源不允许写操作。
// CLI 在收到非只读语句但 allow_write=false 时返回该错误。
var ErrWriteDisabled = errors.New("write operations are disabled for this datasource (set allow_write: true to enable)")

// WriteGuard 是写操作守卫（Decorator 模式）。
// 它包裹一个 Conn，强制让 Exec 受 allow_write 开关控制。
// 这样无论哪个命令路径，只要经由 guard，写权限就一定能被校验，
// 避免在多个命令里重复写「if !allow_write」散落判断。
type WriteGuard struct {
	inner      Conn
	allowWrite bool
}

// NewWriteGuard 用 allowWrite 开关包裹一个 Conn。
func NewWriteGuard(inner Conn, allowWrite bool) *WriteGuard {
	return &WriteGuard{inner: inner, allowWrite: allowWrite}
}

// Unwrap 暴露底层 Conn，供需要访问元数据等只读能力的调用方使用。
func (g *WriteGuard) Unwrap() Conn { return g.inner }

// Query 直接透传（只读，不受守卫限制）。
func (g *WriteGuard) Query(ctx context.Context, sql string, args ...any) (*Result, error) {
	return g.inner.Query(ctx, sql, args...)
}

// Exec 受守卫保护：未开启 allow_write 则拒绝。
func (g *WriteGuard) Exec(ctx context.Context, sql string, args ...any) (ExecResult, error) {
	if !g.allowWrite {
		return ExecResult{}, fmt.Errorf("%w: rejected statement", ErrWriteDisabled)
	}
	return g.inner.Exec(ctx, sql, args...)
}

// Version 透传。
func (g *WriteGuard) Version(ctx context.Context) (*DBVersion, error) {
	return g.inner.Version(ctx)
}

// Metadata 透传。
func (g *WriteGuard) Metadata() MetadataProvider { return g.inner.Metadata() }

// Ping 透传（只读，不受守卫限制）。
func (g *WriteGuard) Ping(ctx context.Context) error { return g.inner.Ping(ctx) }

// Close 透传。
func (g *WriteGuard) Close() error { return g.inner.Close() }
