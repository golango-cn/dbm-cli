// Package config 负责加载与解析 dbm 的 YAML 配置。
//
// 配置文件查找顺序（首个命中即用）：
//  1. --config 指定的路径
//  2. 当前目录 ./dbm.yaml
//  3. $XDG_CONFIG_HOME/dbm/config.yaml（或 ~/.config/dbm/config.yaml）
//  4. ~/.dbm.yaml
//
// 配置内支持 ${ENV_VAR} 占位，加载时从环境变量展开，避免在文件里明文存放密码。
package config

import "github.com/golango-cn/dbm-cli/internal/driver"

// File 是 dbm.yaml 的顶层结构。
type File struct {
	Default     string                       `yaml:"default"`      // 默认数据源名
	Datasources map[string]*driver.DatasourceConfig `yaml:"datasources"` // 数据源集合
}

// Validate 做基本完整性校验：default 指向的数据源必须存在。
func (f *File) Validate() error {
	if f.Default == "" && len(f.Datasources) == 1 {
		// 只有一个数据源时，自动把它当作默认，提升易用性。
		for name := range f.Datasources {
			f.Default = name
		}
	}
	if f.Default != "" {
		if _, ok := f.Datasources[f.Default]; !ok {
			return &ValidationError{
				Field:   "default",
				Message: "default datasource " + f.Default + " is not defined in datasources",
			}
		}
	}
	return nil
}

// ValidationError 表示配置校验失败。
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return "config: invalid field " + e.Field + ": " + e.Message
}
