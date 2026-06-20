package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/golango-cn/dbm-cli/internal/driver"
	"gopkg.in/yaml.v3"
)

// envPattern 匹配 ${VAR} 形式的环境变量占位符。
var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// DefaultSearchPaths 返回配置文件的查找路径（按优先级从高到低），
// 但不包含显式 --config 指定的路径（该路径在 Load 中单独处理）。
// 注意：这里返回的都是「候选」，首个存在者生效。
//
// 默认主位置是 ~/.dbm-cli.yaml；此外也兼容当前目录与 XDG 标准位置。
func DefaultSearchPaths() []string {
	var paths []string
	// 当前工作目录
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, "dbm-cli.yaml"), filepath.Join(cwd, "dbm-cli.yml"))
	}
	// 2. 用户主目录的 ~/.dbm-cli.yaml（推荐的主默认位置）
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".dbm-cli.yaml"))
	}
	// 3. XDG: $XDG_CONFIG_HOME/dbm-cli/config.yaml，回退到 ~/.config/dbm-cli/config.yaml
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "dbm-cli", "config.yaml"))
	} else if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "dbm-cli", "config.yaml"))
	}
	return paths
}

// Load 从指定路径加载配置。
// 若 path 为空，则按 DefaultSearchPaths 顺序查找首个存在文件。
// 找不到任何配置文件时返回 ErrNotFound。
func Load(path string) (*File, error) {
	if path != "" {
		return loadFrom(path)
	}
	for _, p := range DefaultSearchPaths() {
		if fileExists(p) {
			return loadFrom(p)
		}
	}
	return nil, ErrNotFound
}

// ErrNotFound 表示未找到任何配置文件。
var ErrNotFound = errors.New("config: no dbm-cli config file found (create ~/.dbm-cli.yaml or ./dbm-cli.yaml, or set --config)")

func loadFrom(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	expanded := expandEnv(string(raw))
	var f File
	if err := yaml.Unmarshal([]byte(expanded), &f); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if f.Datasources == nil {
		f.Datasources = map[string]*driver.DatasourceConfig{}
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return &f, nil
}

// expandEnv 把字符串里 ${VAR} 替换为对应环境变量值；未设置则替换为空串。
func expandEnv(s string) string {
	return envPattern.ReplaceAllStringFunc(s, func(m string) string {
		name := envPattern.FindStringSubmatch(m)[1]
		return os.Getenv(name)
	})
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false
		}
		return false
	}
	return !info.IsDir()
}
