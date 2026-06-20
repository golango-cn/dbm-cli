// Package manifest 生成 dbm 的自描述清单（面向 AI 调用）。
//
// 设计目标：让 AI agent 只需读取 `dbm manifest` 的 JSON 输出，就能完整掌握：
//   - 工具身份与版本
//   - 已配置哪些数据源（不含密钥）
//   - 支持哪些驱动及各自版本范围（由各 driver 自描述，新增数据源自动出现）
//   - 有哪些命令、各自参数与示例（由 cli 层注入）
//   - 输出格式与配置 schema
//
// 这是「AI 友好」的关键：把分散在 help/help-flag/README 里的调用知识
// 压缩成一份机器可读的清单。
package manifest

import (
	"github.com/golango-cn/dbm-cli/internal/buildinfo"
	"github.com/golango-cn/dbm-cli/internal/config"
	"github.com/golango-cn/dbm-cli/internal/driver"
)

// Manifest 是 `dbm manifest` 命令的顶层输出结构。
type Manifest struct {
	Tool        string            `json:"tool"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Drivers     []DriverManifest  `json:"supported_drivers"`
	Datasources []DatasourceInfo  `json:"datasources_configured"`
	Commands    []CommandInfo     `json:"commands"`
	OutputFormats []string            `json:"output_formats"`
	OutputFormatsDetail []OutputFormatDetail `json:"output_formats_detail"`
	ConfigSchema map[string]string `json:"config_schema"`
	Examples    []Example         `json:"examples"`
}

// DriverManifest 是单个驱动的自描述。各 driver 实现提供这些字段，
// 从而实现「加新数据源 → manifest 自动包含」（OCP）。
type DriverManifest struct {
	Name              string   `json:"name"`
	Aliases           []string `json:"aliases,omitempty"` // 兼容别名，如 mariadb 是 mysql 的别名
	SupportedVersions string   `json:"versions"`
	Description       string   `json:"description"`
}

// DatasourceInfo 是已配置数据源的脱敏信息（不含密码）。
type DatasourceInfo struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	AllowWrite bool   `json:"allow_write"`
	IsDefault  bool   `json:"is_default"`
}

// OutputFormatDetail 描述一种输出格式及其适用场景，帮助 AI/用户选择。
type OutputFormatDetail struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	BestFor     string `json:"best_for"`
}

// CommandInfo 描述一条 CLI 命令。
type CommandInfo struct {
	Name        string   `json:"name"`
	Summary     string   `json:"summary"`
	Usage       string   `json:"usage"`
	Flags       []Flag   `json:"flags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
}

// Flag 描述命令的一个 flag。
type Flag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Default   string `json:"default,omitempty"`
	Usage     string `json:"usage"`
}

// Example 是一个带说明的调用样例。
type Example struct {
	Description string `json:"description"`
	Command     string `json:"command"`
}

// Build 依据当前已注册驱动、已加载配置、以及 cli 注入的命令清单，组装 manifest。
func Build(cfg *config.File, commands []CommandInfo) *Manifest {
	m := &Manifest{
		Tool:        buildinfo.AppName,
		Version:     buildinfo.Version,
		Description: "dbm-cli is a zero-dependency database CLI. Query metadata and data via a single static binary. Currently supports Oracle (10.2+) via pure-Go driver.",
		OutputFormats: []string{"table", "json", "csv", "yaml", "vertical"},
		OutputFormatsDetail: []OutputFormatDetail{
			{Name: "table", Description: "带边框的对齐表格（+---+ / | a | b |），正确处理 CJK 宽字符对齐", BestFor: "人类终端阅读（默认）"},
			{Name: "json", Description: "缩进美化的对象数组 JSON（2 空格缩进，时间转 ISO8601）", BestFor: "程序解析 / AI 调用"},
			{Name: "csv", Description: "标准 CSV，首行表头", BestFor: "导入电子表格 / 数据交换"},
			{Name: "yaml", Description: "YAML 对象数组", BestFor: "配置式阅读"},
			{Name: "vertical", Description: "\\G 风格，每条记录一段（列名: 值）", BestFor: "宽表单行查看"},
		},
		Commands:      commands,
		ConfigSchema:  defaultConfigSchema(),
		Examples:      defaultExamples(),
	}

	// 聚合各 driver 自描述
	for _, d := range driver.List() {
		m.Drivers = append(m.Drivers, DriverManifest{
			Name:              d.Name(),
			Aliases:           driver.AliasesOf(d.Name()),
			SupportedVersions: d.SupportedVersions(),
			Description:       d.Description(),
		})
	}

	// 聚合已配置数据源（脱敏）
	if cfg != nil {
		for name, ds := range cfg.Datasources {
			m.Datasources = append(m.Datasources, DatasourceInfo{
				Name:       name,
				Type:       ds.Type,
				Host:       ds.Host,
				Port:       ds.Port,
				AllowWrite: ds.AllowWrite,
				IsDefault:  name == cfg.Default,
			})
		}
	}
	return m
}

// defaultConfigSchema 描述 yaml 字段，帮助 AI 正确编写配置。
func defaultConfigSchema() map[string]string {
	return map[string]string{
		"default":               "默认数据源名（可选；仅一个数据源时自动取其为默认）",
		"datasources.<name>.type": "驱动名，如 oracle",
		"datasources.<name>.description": "数据源职责说明（人类/AI 可读），用于区分同类型数据源",
		"datasources.<name>.host": "主机地址",
		"datasources.<name>.port": "端口，如 1521",
		"datasources.<name>.service_name": "Oracle service_name（与 sid 二选一）",
		"datasources.<name>.sid":          "Oracle SID（与 service_name 二选一）",
		"datasources.<name>.user":         "用户名",
		"datasources.<name>.password":     "密码，支持 ${ENV_VAR} 占位",
		"datasources.<name>.allow_write":  "是否允许写操作，默认 false（只读）",
		"datasources.<name>.force_version": "可选，跳过版本探测强制指定版本（如 11g）",
		"datasources.<name>.fetch_size":    "行预取大小",
		"datasources.<name>.max_open_conns": "连接池最大打开连接数",
		"datasources.<name>.timeout":       "连接超时（如 10s）；超时报错并提示排查，默认 30s",
	}
}

func defaultExamples() []Example {
	return []Example{
		{Description: "列出已配置数据源", Command: "dbm-cli datasources"},
		{Description: "查询数据库版本", Command: "dbm-cli version -d prod-ro"},
		{Description: "列出某 schema 的表", Command: "dbm-cli tables -d prod-ro --schema HR"},
		{Description: "查看表结构", Command: "dbm-cli columns -d prod-ro --table EMPLOYEES --schema HR"},
		{Description: "分页查表数据（JSON 输出）", Command: "dbm-cli table -d prod-ro --name EMPLOYEES --schema HR --limit 20 -o json"},
		{Description: "执行任意只读 SQL", Command: "dbm-cli query -d prod-ro \"SELECT * FROM HR.EMPLOYEES WHERE ROWNUM <= 10\""},
		{Description: "获取自描述清单（供 AI 调用）", Command: "dbm-cli manifest"},
	}
}
