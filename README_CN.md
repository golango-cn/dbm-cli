# dbm-cli

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Oracle](https://img.shields.io/badge/Oracle-10g%20~%2021c-F80000?logo=oracle)](https://www.oracle.com/database/)
[![MySQL](https://img.shields.io/badge/MySQL-5.7%20~%208.0-4479A1?logo=mysql&logoColor=white)](https://www.mysql.com/)
[![ClickHouse](https://img.shields.io/badge/ClickHouse-22.x%2B-FFCC01?logo=clickhouse&logoColor=black)](https://clickhouse.com/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-9.6%2B-4169E1?logo=postgresql&logoColor=white)](https://www.postgresql.org/)

> **零外部依赖**的数据库命令行工具：**单个静态二进制**即可查询数据库元数据与数据。
> 支持 **Oracle**（10g–21c，纯 Go [go-ora](https://github.com/sijms/go-ora)）、**MySQL / MariaDB**（5.7 / 8.0.x / MariaDB 10.x+，纯 Go [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql)）、**ClickHouse**（22.x+，纯 Go [clickhouse-go/v2](https://github.com/ClickHouse/clickhouse-go)）、**PostgreSQL**（9.6+，纯 Go [pgx/v5](https://github.com/jackc/pgx)）——均无需 Instant Client / CGO。通过 driver 接口扩展更多数据库。
> **面向 AI**：`dbm-cli manifest` 输出自描述 JSON 契约。

[English](README.md)

---

## ✨ 特性

- 🪶 **单二进制** —— 所有驱动均为纯 Go 实现，编译产物无外部原生库依赖，拷贝即用。
- 🗂️ **多版本 Oracle** —— 自动探测 10g / 11g / 12c / 18c / 19c / 21c，按版本选择字典视图；受限账号可用 `force_version` 跳过探测。
- 🐬 **MySQL 5.7 / 8.0.x 与 MariaDB** —— 元数据走 `information_schema`（跨版本一致）；原生支持 `caching_sha2_password`（8.0 默认认证）。MariaDB 协议兼容，通过 `mariadb` 类型别名即可使用。可选 TLS（`skip-verify`）适配自签证书。
- ⚡ **ClickHouse 22.x+** —— 元数据走 `system.*` 表；原生 TCP 协议（9000 端口）低开销。理解 ClickHouse 专有类型（`Nullable(...)`、`Array(...)`）与基于引擎的表类型。
- 🐘 **PostgreSQL 9.6+** —— 元数据走 `information_schema` + `pg_catalog`；正确区分 schema/database；解析数组类型的索引列。
- 🧩 **可扩展** —— `Driver` + `MetadataProvider` + 注册表设计。新增数据库只需新建一个包并 `import _`，核心代码无需改动（开闭原则）。
- 🔒 **读写受控** —— 每个数据源 `allow_write` 开关（默认只读）。危险语句（`DROP` / `TRUNCATE` / 无 `WHERE` 的 `DELETE`/`UPDATE`）需交互式确认。
- 🤖 **AI 友好** —— `dbm-cli manifest` 输出 JSON 契约，描述全部命令、参数、驱动与示例，AI 读取一次即学会如何调用。
- 🎨 **多种输出格式** —— `table` / `json` / `csv` / `yaml` / `vertical`。
- 🧭 **机器友好的错误** —— 任何失败都以 `[dbm-cli] error: <原因>` 形式打印到 stderr，附带重试提示；退出码区分「可重试」（1）与「需改配置」（2）。

## 📦 安装

```bash
git clone https://github.com/golango-cn/dbm-cli.git
cd dbm-cli
make build          # 产出 ./bin/dbm-cli
# 或直接
go build -o dbm-cli ./cmd/dbm-cli
```

跨平台静态编译（Linux / macOS / Windows 的 amd64 + arm64）：

```bash
make dist           # 输出到 ./dist/
```

## 🧪 在线演示（live demo）

仓库内置一个演示目录 [`demo/`](demo)，通过 Makefile 演示全部命令。它与具体数据库无关——指向你配置的任意数据源即可：

```bash
cd demo
# 1. 编译二进制到 ./demo
go build -o dbm-cli ../cmd/dbm-cli
# 2. 创建配置（复制并编辑 ../examples/config.yaml.example）
cp ../examples/config.yaml.example dbm-cli.yaml   # 然后修改 host/user/password
# 3. 在默认数据源上初始化演示表
make init
# 4. 运行演示
make manifest       # 支持的驱动
make demo           # 默认数据源的 version/tables/columns/data
make formats        # 全部输出格式（table/json/csv/yaml/vertical）
make demo DS=other  # 换一个数据源演示
```

> demo 的 `dbm-cli` 二进制和 `dbm-cli.yaml`（含凭据）已被 .gitignore 忽略，不入库；仅 `Makefile` 和 `init-demo-data.sh` 入库。

## ⚙️ 配置

复制 [`examples/config.yaml.example`](examples/config.yaml.example) 为 `~/.dbm-cli.yaml`（推荐的默认位置）并修改。若不带 `-c` 运行，按以下顺序查找（首个命中即用）：
`--config` → `./dbm-cli.yaml` → **`~/.dbm-cli.yaml`** → `$XDG_CONFIG_HOME/dbm-cli/config.yaml`

```yaml
default: prod-ro
datasources:
  # Oracle（service_name 与 sid 二选一）
  prod-ro:
    type: oracle
    host: 10.0.0.1
    port: 1521
    service_name: ORCLPDB1
    user: readonly
    password: ${DB_PWD_PROD} # 支持环境变量占位，避免明文密码
    allow_write: false       # 默认只读

  # MySQL（database = 数据库名 / schema 名）
  mysql-prod:
    type: mysql
    host: 10.0.0.2
    port: 3306
    database: app_db         # 必填：MySQL 中 schema == database
    user: readonly
    password: ${DB_PWD_MYSQL}
    allow_write: false
    # tls: skip-verify        # 可选：自签证书场景启用 TLS（跳过证书校验）

  # MariaDB —— 与 MySQL 协议兼容；用 type: mariadb（mysql 的别名）
  maria-prod:
    type: mariadb             # 别名 -> mysql 驱动；用法同 type: mysql
    host: 10.0.0.10
    port: 3306
    database: app_db
    user: readonly
    password: ${DB_PWD_MARIA}
    allow_write: false

  # ClickHouse（原生 TCP 端口 9000）
  ck-prod:
    type: clickhouse
    host: 10.0.0.4
    port: 9000               # 原生 TCP 端口（注意不是 8123 的 HTTP 端口）
    database: analytics
    user: default
    password: ${DB_PWD_CK}
    allow_write: false
    # tls: skip-verify        # 可选：启用 TLS

  # 可写数据源（任意类型）
  dev-rw:
    type: mysql
    host: 10.0.0.3
    port: 3306
    database: app_db
    user: dev
    password: ${DB_PWD_DEV}
    allow_write: true
```

> **MySQL 说明**：`database` 为必填项（MySQL 的 schema 即 database）。8.0 默认认证插件 `caching_sha2_password` 无需额外配置即可连接。

> **连接超时**：给任意数据源加 `timeout: 10s`。dbm-cli 首次使用时会验证连接；若在超时时间内连不上则快速失败并给出清晰提示（含 host/port/凭据/网络排查方向）。默认 30s。

> **数据源描述**：给任意数据源加可选的 `description: "..."`——人类/AI 可读的职责说明（如*"生产订单库只读副本"*）。`dbm-cli config` 会展示它，让 AI 按职责而非名字选择数据源。

## 🚀 用法

```bash
dbm-cli config                                          # 查看配置概览（数据源清单、默认、超时；密码隐藏）
dbm-cli config    -o json                               # ...JSON 输出（供 AI/脚本选数据源）
dbm-cli datasources                                     # 列出已配置数据源（隐藏密码）
dbm-cli version   -d prod-ro                            # 数据库版本
dbm-cli databases -d prod-ro                            # 库 / PDB
dbm-cli schemas   -d prod-ro --like HR%                 # schema 列表
dbm-cli tables    -d prod-ro --schema HR                # 表列表
dbm-cli columns   -d prod-ro --table EMPLOYEES --schema HR
dbm-cli indexes   -d prod-ro --table EMPLOYEES
dbm-cli views     -d prod-ro --schema HR
dbm-cli table     -d prod-ro --name EMPLOYEES --schema HR --limit 20 -o json
dbm-cli query     -d prod-ro "SELECT * FROM HR.EMPLOYEES WHERE ROWNUM<=10"
dbm-cli manifest                                        # 面向 AI 的自描述 JSON
```

全局 flag：`-c/--config`、`-d/--datasource`、`-o/--output {table|json|csv|yaml|vertical}`、`--no-header`

#### 输出格式

| 格式 | 说明 | 适用场景 |
|------|------|----------|
| `table`（默认） | 带边框的对齐表格（`+---+`、`| a | b |`），正确处理 CJK 宽字符对齐 | 人类终端阅读 |
| `json` | 缩进美化的对象数组（2 空格缩进，时间转 ISO8601） | 程序解析 / AI 调用 |
| `csv` | 标准 CSV，首行表头 | 导入电子表格 / 数据交换 |
| `yaml` | YAML 对象数组 | 配置式阅读 |
| `vertical` | `\G` 风格，每条记录一段（列名: 值） | 宽表、少量行查看 |

```bash
dbm-cli tables -d prod-ro --schema HR                      # 带边框表格
dbm-cli tables -d prod-ro --schema HR -o json              # JSON
dbm-cli table  -d prod-ro --name EMP --limit 5 -o csv      # CSV
```

### 退出码与错误（面向自动化 / AI）

| 退出码 | 含义 | 处理方式 |
|--------|------|----------|
| 0 | 成功 | — |
| 1 | 运行时错误（SQL 错误、查询失败、网络问题） | 修正查询后通常可重试 |
| 2 | 配置/连接类错误（缺少配置、写守卫拒绝） | 重试前需修改配置或 `allow_write` |

所有错误以 `[dbm-cli] error: <消息>` 形式输出到 stderr，常附带一行 `[dbm-cli] hint:` 告诉你重试前具体要改什么。

## 🏗️ 架构

dbm-cli 围绕 driver 抽象构建，新增一种数据库几乎不改动现有代码。核心设计模式：

| 模式 | 落点 |
|------|------|
| Registry / Factory | `internal/driver/registry.go` —— 驱动自注册 |
| Strategy | 不同数据库的 `MetadataProvider` 实现 |
| Adapter | `internal/oracle` 把 go-ora 适配为统一 `Conn` |
| Decorator / Guard | `WriteGuard` 强制 `allow_write` |
| Facade | `pkg/dbmcli` 对外稳定 API |
| Builder | 连接串与分页 SQL 构建 |

**新增数据源（如 MySQL）**：新建 `internal/mysql/`，实现 `Driver`/`Conn`/`MetadataProvider`，在 `init()` 里 `driver.Register`，并在 `cmd/dbm-cli/main.go` 加一行 `import _ "github.com/golango-cn/dbm-cli/internal/mysql"`。CLI、输出层与 manifest 会自动识别。

## 📚 编程接口

`pkg/dbmcli` 提供稳定门面（Facade），便于嵌入其他 Go 程序：

```go
import "github.com/golango-cn/dbm-cli/pkg/dbmcli"

conn, cleanup, err := dbmcli.Open(ctx, &dbmcli.Datasource{
    Type: "oracle", Host: "10.0.0.1", Port: 1521,
    ServiceName: "ORCLPDB1", User: "ro", Password: os.Getenv("PWD"),
})
defer cleanup()
tables, _ := conn.Metadata().Tables(ctx, "HR")
```

## 📌 路线图

- [x] M0 骨架
- [x] M1 Oracle 连通（驱动、版本探测、Ping）
- [x] M2 元数据查询（多版本字典 SQL）
- [x] M3 数据查询（`table` 分页、`query` 只读路径）
- [x] M4 读写与守卫（`allow_write`、SQL 分类、危险语句确认）
- [x] M5 输出格式化
- [x] M6 Manifest（驱动自描述注入）
- [x] **MySQL 驱动**（5.6 / 5.7 / 8.0.x，已对 8.0.37 端到端验证）
- [x] **Oracle 驱动**已对 18c XE 端到端验证
- [x] **ClickHouse 驱动**（22.x+，已对 25.8 端到端验证）
- [x] **PostgreSQL 驱动**（9.6+，已对 12 / 17 端到端验证）
- [ ] M7 打磨：单元测试、跨平台发布
- [ ] SQL Server 驱动

> 四个驱动（Oracle / MySQL / ClickHouse / PostgreSQL）全部完成，通过 `go vet` + `go build`，并已对真实实例端到端验证：Oracle 11g/18c XE、MySQL 5.7/8.0.12/8.0.37、ClickHouse 25.8、PostgreSQL 12/17。

## 🤝 贡献

欢迎提 Issue 和 PR。提交前请运行 `go vet ./...` 和 `go test ./...`。

## 📄 许可

[MIT](LICENSE) © golango
