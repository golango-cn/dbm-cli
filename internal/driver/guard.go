package driver

import (
	"context"
	"errors"
	"fmt"
)

// ErrWriteDisabled 表示当前数据源不允许写操作。
// CLI 在收到非只读语句但 allow_write=false 时返回该错误。
// 它是一个 sentinel：main.go 用 errors.Is(err, ErrWriteDisabled) 判定 exit code=2。
var ErrWriteDisabled = errors.New("write operations are disabled for this datasource (set allow_write: true to enable)")

// WriteDisabledError 包装 ErrWriteDisabled，允许调用方附加上下文（语句类别、数据源名）
// 同时保持 errors.Is(err, ErrWriteDisabled) 成立。.Error() 只返回附加的中文消息，
// 不重复 sentinel 的英文原文，避免提示冗余。
type WriteDisabledError struct {
	// Detail 是面向用户的中文说明（含语句类别、数据源名、修改建议）。
	Detail string
}

// Error 实现 error 接口，只返回附加的中文详情。
func (e *WriteDisabledError) Error() string { return e.Detail }

// Is 让 errors.Is(err, ErrWriteDisabled) 对 WriteDisabledError 成立，
// 从而 main.go 能据此返回 exit code=2（配置/权限类错误）。
func (e *WriteDisabledError) Is(target error) bool { return target == ErrWriteDisabled }

// NewWriteDisabledError 构造一个带上下文的写拒绝错误。
func NewWriteDisabledError(detail string) *WriteDisabledError {
	return &WriteDisabledError{Detail: detail}
}

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
