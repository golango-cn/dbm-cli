// Package buildinfo 存放编译期注入的版本信息。
// 这些变量通过 -ldflags '-X ...=value' 在 Makefile 构建时注入；
// 缺省值用于 `go run` / 直接 `go build` 的开发场景。
package buildinfo

// Version 工具版本号。默认 dev，正式构建由 Makefile 注入 git tag。
var Version = "dev"

// Commit 枥简哈希。默认 unknown，正式构建由 Makefile 注入。
var Commit = "unknown"

// AppName 工具名，用于 help / manifest 输出。
const AppName = "dbm-cli"
