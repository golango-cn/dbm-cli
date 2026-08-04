// Package driver 定义数据源驱动的核心抽象。
//
// 这是整个项目的「扩展点」：每新增一种数据库（mysql / sqlserver / ...），
// 只需实现本包的 Driver + Conn + MetadataProvider 接口，并在自身包的
// init() 里调用 Register 即可被 CLI 使用，无需改动 cli 与 driver 核心。
// 这体现「对扩展开放、对修改封闭」（OCP）。
//
// 涉及的设计模式：
//   - Strategy：MetadataProvider 的不同实现
//   - Registry / Factory：Register/Get
//   - Adapter：把各驱动（如 go-ora）差异适配成统一 Conn
//   - Decorator / Guard：写操作守卫（guard.go）
package driver

import (
	"context"
	"time"
)

// DatasourceConfig 描述一份来自 yaml 的数据源连接配置。
// 它是各驱动共用的「配置载体」，驱动按自身需要读取其中的字段。
type DatasourceConfig struct {
	// 通用字段
	Type         string `yaml:"type"`          // 驱动名，如 "oracle"/"mysql"，对应 Driver.Name()
	Description  string        `yaml:"description"`   // 可选：数据源的人类/AI 可读说明（职责、用途），帮助 AI 区分同类型数据源
	Host         string `yaml:"host"`          // 主机
	Port         int    `yaml:"port"`          // 端口
	User         string `yaml:"user"`          // 用户名
	Password     string `yaml:"password"`      // 密码（已展开环境变量占位）
	Database     string `yaml:"database"`      // 数据库名（MySQL 等；Oracle 用 service_name/sid）
	AllowWrite   bool   `yaml:"allow_write"`   // 是否允许写操作，默认 false（只读）
	ForceVersion string `yaml:"force_version"` // 可选：跳过版本探测，强制指定版本标识（如 "11g"/"8.0"）
	TLS          string `yaml:"tls"`           // 可选 TLS 配置：skip-verify / true / 自定义名；空表示不启用
	Timeout      time.Duration `yaml:"timeout"`       // 连接超时（如 "10s"）；0 表示用默认 30s

	// Oracle 相关（其它驱动可在此扩展或使用 Raw）
	ServiceName string `yaml:"service_name"` // Oracle service_name（与 SID 二选一）
	SID         string `yaml:"sid"`          // Oracle SID
	FetchSize   int    `yaml:"fetch_size"`   // 行预取大小，0 表示用驱动默认

	// 连接池
	MaxOpenConns int `yaml:"max_open_conns"` // 最大打开连接数，0 表示驱动默认
	MaxIdleConns int `yaml:"max_idle_conns"` // 最大空闲连接数

	// Raw 保留原始 map，供需要私有字段的驱动读取（如 SSL 参数等）。
	Raw map[string]any `yaml:",inline"`
}

// DBVersion 表示探测到的数据库版本信息。
type DBVersion struct {
	Product    string // 产品名，如 "Oracle"
	Version    string // 完整版本字符串，如 "Oracle Database 19c Enterprise Edition Release 19.0.0.0.0"
	Major      int    // 主版本号，如 19
	Minor      int    // 次版本号，如 0
	Banner     string // v$version banner 原文（如可获取）
	DetectedAt time.Time
}

// Driver 是一种数据库类型的工厂。
// 实现者通常在自身包的 init() 中调用 Register 完成自注册。
type Driver interface {
	// Name 返回驱动名，需与 DatasourceConfig.Type 一致，如 "oracle"。
	Name() string
	// SupportedVersions 返回该驱动支持的数据库版本描述，如 "10.2+"。
	SupportedVersions() string
	// Description 返回面向人类的简短描述，用于 manifest / help。
	Description() string
	// Open 依据配置建立连接，返回统一的 Conn 抽象。
	Open(cfg *DatasourceConfig) (Conn, error)
}

// Conn 是一次与数据库的活动连接（或连接池句柄）。
type Conn interface {
	// Query 执行返回结果集的语句（SELECT 及各 DB 的等价物）。
	Query(ctx context.Context, sql string, args ...any) (*Result, error)
	// Exec 执行不返回结果集的语句（INSERT/UPDATE/DELETE/DDL）。
	// 注意：是否允许到达此方法由 cli 层的 guard 决定（见 guard.go），
	// 驱动实现本身不做权限判断，保持单一职责。
	Exec(ctx context.Context, sql string, args ...any) (ExecResult, error)
	// Version 探测并返回数据库版本。
	Version(ctx context.Context) (*DBVersion, error)
	// Metadata 返回该连接的元数据查询提供者。
	Metadata() MetadataProvider
	// Ping 验证连接是否真实可用（database/sql 的 Ping 语义）。
	// sql.Open 通常是惰性的，Ping 用于在打开后尽早暴露连接错误。
	Ping(ctx context.Context) error
	// Close 释放连接资源。
	Close() error
}

// MetadataProvider 暴露数据库的结构化元数据查询能力。
// 不同数据库实现各自的版本（Strategy）。
type MetadataProvider interface {
	// Databases 返回「库」列表。
	// Oracle: 非 CDB 通常返回单元素（当前库）；CDB 返回各 PDB 名称。
	Databases(ctx context.Context) ([]string, error)
	// Schemas 返回 schema / user 列表。
	Schemas(ctx context.Context) ([]string, error)
	// Tables 返回指定 schema 下的表。
	Tables(ctx context.Context, schema string) ([]TableInfo, error)
	// Columns 返回指定表的列定义。
	Columns(ctx context.Context, schema, table string) ([]ColumnInfo, error)
	// Indexes 返回指定表的索引。
	Indexes(ctx context.Context, schema, table string) ([]IndexInfo, error)
	// Views 返回指定 schema 下的视图。
	Views(ctx context.Context, schema string) ([]ViewInfo, error)
	// RowCount 返回指定表的近似行数（可能使用字典统计值）。
	RowCount(ctx context.Context, schema, table string) (int64, error)
}

// PagedDataProvider 是一个「可选」能力接口：支持分页读取表数据的驱动实现它。
//
// 之所以单独成接口而非塞进 Conn/MetadataProvider，是因为分页方言差异巨大
// （Oracle 的 ROWNUM、MySQL 的 LIMIT、SQLServer 的 OFFSET/FETCH），
// 不应强制每个驱动都实现。CLI 层通过类型断言检测该能力：
//
//	if p, ok := conn.(driver.PagedDataProvider); ok { ... }
//
// 未实现时，cli 的 `table` 命令可回退到「SELECT * + 通用 LIMIT 提示」或报错提示。
type PagedDataProvider interface {
	// QueryTable 分页读取某张表的数据。
	// schema 为空表示用当前用户；limit<=0 表示驱动默认值；orderCol 为排序列（空则不排序）。
	QueryTable(ctx context.Context, schema, table string, limit, offset int64, orderCol string) (*Result, error)
}

// TableInfo 描述一张表。
type TableInfo struct {
	Schema    string `json:"schema"`
	Name      string `json:"name"`
	Type      string `json:"type"`       // TABLE / VIEW / MATERIALIZED VIEW ...
	Comment   string `json:"comment,omitempty"`
	RowCount  int64  `json:"row_count,omitempty"` // 近似行数（按需填充）
}

// ColumnInfo 描述一列。
type ColumnInfo struct {
	Name          string `json:"name"`
	DataType      string `json:"data_type"`            // 类型名，如 NUMBER、VARCHAR2
	Length        int64  `json:"length,omitempty"`
	Precision     int    `json:"precision,omitempty"`
	Scale         int    `json:"scale,omitempty"`
	Nullable      bool   `json:"nullable"`
	DefaultValue  string `json:"default_value,omitempty"`
	Position      int    `json:"position"`
	Comment       string `json:"comment,omitempty"`
}

// IndexInfo 描述一个索引。
type IndexInfo struct {
	Name      string   `json:"name"`
	IsUnique  bool     `json:"is_unique"`
	IsPrimary bool     `json:"is_primary"`
	Columns   []string `json:"columns"`
}

// ViewInfo 描述一个视图。
type ViewInfo struct {
	Schema  string `json:"schema"`
	Name    string `json:"name"`
	Comment string `json:"comment,omitempty"`
}

// Result 是查询结果集的通用载体，与具体驱动解耦。
type Result struct {
	Columns []string   // 列名
	Rows    [][]any    // 按行的值；每行元素顺序与 Columns 一致
	Affected int64     // 受影响行数（仅 Exec 相关，Query 时为 0）
}

// ExecResult 是写操作的返回。
type ExecResult struct {
	RowsAffected int64
	LastInsertID int64 // 部分数据库支持
}

// ParamDirection 描述存储过程参数的方向。
type ParamDirection int

const (
	ParamIn    ParamDirection = iota // 输入参数
	ParamOut                         // 输出参数
	ParamInOut                       // 输入输出参数
)

// ProcParam 描述一个存储过程参数（IN / OUT / INOUT）。
//
// 对 IN 参数：Value 即为传入值。
// 对 OUT/INOUT 参数：OutType 指示回收值的类型，取值：
//   - "number"  → 数值（float64 回收）
//   - "string"  → 字符串（需 OutSize 给缓冲长度，默认 4000）
//   - "cursor"  → REF CURSOR（结果集，填入 ProcResult.ResultSet）
// INOUT 参数同时使用 Value（输入）与 OutType（输出）。
type ProcParam struct {
	Name      string         // 参数名（仅用于 OUT 值回显的 key，可选）
	Value     any            // IN / INOUT 的输入值
	Direction ParamDirection // 方向
	OutType   string         // OUT 目标类型：number|string|cursor
	OutSize   int            // OUT 字符串/集合的缓冲大小（默认 4000）
}

// ProcResult 是存储过程调用的返回。
type ProcResult struct {
	OutParams map[string]any // OUT 标量参数名→值（数值/字符串等；不含 cursor）
	ResultSet *Result        // REF CURSOR 的结果集（若有 cursor 类型的 OUT 参数）
	Output    string         // DBMS_OUTPUT（PL/SQL 里 PUT_LINE 的打印文本），驱动不支持则为空
}

// StoredProcCaller 是一个「可选」能力接口：调用存储过程并捕获 DBMS_OUTPUT。
//
// 之所以单独成接口而非塞进 Conn，是因为：
//  1. 并非所有数据库都有"存储过程 + 服务端 print"的等价物（如 SQLite/MySQL 模型不同）；
//  2. REF CURSOR / DBMS_OUTPUT 的处理高度数据库特定，不应强制每个驱动实现。
//
// CLI 层通过类型断言检测该能力（类似 PagedDataProvider）：
//
//	if sp, ok := conn.(driver.StoredProcCaller); ok { ... }
//
// 未实现时，`call` 命令给出清晰的"该驱动不支持存储过程"提示。
// 注意：经 WriteGuard 包裹的连接，需先 Unwrap 拿到底层 conn 再做断言。
type StoredProcCaller interface {
	// CallProc 调用存储过程 proc，按 params 顺序绑定参数（先 IN 后 OUT）。
	// 返回 OUT 标量值、REF CURSOR 结果集、DBMS_OUTPUT 文本。
	CallProc(ctx context.Context, proc string, params []ProcParam) (*ProcResult, error)
}
