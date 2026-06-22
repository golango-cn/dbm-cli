# dbm-cli

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Oracle](https://img.shields.io/badge/Oracle-10g%20~%2021c-F80000?logo=oracle)](https://www.oracle.com/database/)
[![MySQL](https://img.shields.io/badge/MySQL-5.7%20~%208.0-4479A1?logo=mysql&logoColor=white)](https://www.mysql.com/)
[![ClickHouse](https://img.shields.io/badge/ClickHouse-22.x%2B-FFCC01?logo=clickhouse&logoColor=black)](https://clickhouse.com/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-9.6%2B-4169E1?logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Impala](https://img.shields.io/badge/Apache%20Impala-4.x-58AEE6?logo=apache&logoColor=white)](https://impala.apache.org/)
[![SQL Server](https://img.shields.io/badge/SQL%20Server-2017%2B-CC2927?logo=microsoftsqlserver&logoColor=white)](https://www.microsoft.com/sql-server)

> A **zero-external-dependency** database CLI: a single static binary queries database metadata and data.
> Supports **Oracle** (10g–21c, pure-Go [go-ora](https://github.com/sijms/go-ora)), **MySQL / MariaDB** (5.7 / 8.0.x / MariaDB 10.x+, pure-Go [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql)), **ClickHouse** (22.x+, pure-Go [clickhouse-go/v2](https://github.com/ClickHouse/clickhouse-go)), **PostgreSQL** (9.6+, pure-Go [pgx/v5](https://github.com/jackc/pgx)), **Apache Impala** (3.x/4.x, pure-Go [impala-go](https://github.com/sclgo/impala)), and **SQL Server** (2017+, pure-Go [go-mssqldb](https://github.com/microsoft/go-mssqldb)) — all without Instant Client / CGO. Extensible to more databases through a driver interface.
> **AI-friendly**: `dbm-cli manifest` emits a self-describing JSON contract.

[中文文档](README_CN.md)

---

## ✨ Features

- 🪶 **Single binary** — all drivers are pure Go, so the compiled binary has no external native-library dependency. Copy it and run.
- 🗂️ **Multi-version Oracle** — auto-detects 10g / 11g / 12c / 18c / 19c / 21c and picks the right dictionary views per version. Restricted accounts can skip detection via `force_version`.
- 🐬 **MySQL 5.7 / 8.0.x & MariaDB** — metadata via `information_schema` (consistent across versions); supports `caching_sha2_password` (8.0 default auth) out of the box. MariaDB is protocol-compatible and usable via the `mariadb` type alias. Optional TLS (`skip-verify`) for self-signed certs.
- ⚡ **ClickHouse 22.x+** — metadata via `system.*` tables; native TCP protocol (port 9000) for low overhead. Understands ClickHouse-specific types (`Nullable(...)`, `Array(...)`) and engine-based table types.
- 🐘 **PostgreSQL 9.6+** — metadata via `information_schema` + `pg_catalog`; correct schema/database distinction; array-typed index columns parsed.
- 🐝 **Apache Impala 3.x/4.x** — metadata via `SHOW`/`DESCRIBE` (no INFORMATION_SCHEMA); HiveServer2 protocol. Understands Impala has no traditional indexes (PARTITIONED/SORTED BY instead).
- 🟦 **SQL Server 2017+** — metadata via `sys.*` catalog views; OFFSET/FETCH paging; no ODBC driver needed (pure-Go go-mssqldb).
- 🧩 **Extensible** — `Driver` + `MetadataProvider` + a registry. Adding a database means one new package and one `import _` line; core stays untouched (Open–Closed Principle).
- 🔒 **Controlled writes** — every datasource has an `allow_write` switch (read-only by default). Destructive statements (`DROP` / `TRUNCATE` / `DELETE`/`UPDATE` without `WHERE`) require an interactive confirmation.
- 🤖 **AI-friendly** — `dbm-cli manifest` outputs a JSON contract describing all commands, flags, drivers and examples, so an agent learns how to call the tool from a single read.
- 🔌 **MCP server** — `dbm-cli mcp` exposes the same capabilities (metadata, sample data, SQL) as a Model Context Protocol server over stdio, so AI clients like Claude Desktop / Cursor can call the database directly. Zero impact on existing CLI commands.
- 🎨 **Multiple output formats** — `table` / `json` / `csv` / `yaml` / `vertical`.
- 🧭 **Machine-friendly errors** — every failure is printed to stderr as `[dbm-cli] error: <reason>` with a retry hint, and exit codes distinguish *retryable* (1) from *config-required* (2) failures.

## 📦 Install

```bash
git clone https://github.com/golango-cn/dbm-cli.git
cd dbm-cli
make build          # produces ./bin/dbm-cli
# or directly
go build -o dbm-cli ./cmd/dbm-cli
```

Cross-compile static binaries for Linux / macOS / Windows (amd64 + arm64):

```bash
make dist           # outputs to ./dist/
```

## 🧪 Try it (live demo)

The repo ships a demo harness in [`demo/`](demo) that exercises every command via a Makefile. It is database-agnostic — point it at any datasource you have configured:

```bash
cd demo
# 1. build the binary into ./demo
go build -o dbm-cli ../cmd/dbm-cli
# 2. create a config (copy & edit from ../examples/config.yaml.example)
cp ../examples/config.yaml.example dbm-cli.yaml   # then edit host/user/password
# 3. initialize the demo table on your default datasource
make init
# 4. run the demos
make manifest       # supported drivers
make demo           # version/tables/columns/data on default datasource
make formats        # all output formats (table/json/csv/yaml/vertical)
make demo DS=other  # run against a different datasource
```

> The demo's `dbm-cli` binary and `dbm-cli.yaml` (which holds credentials) are gitignored. Only `Makefile` and `init-demo-data.sh` are tracked.

## ⚙️ Configuration

Copy [`examples/config.yaml.example`](examples/config.yaml.example) to `~/.dbm-cli.yaml` (the recommended default location) and edit. If you run `dbm-cli` without `-c`, it looks up the config in this order (first match wins):
`--config` → `./dbm-cli.yaml` → **`~/.dbm-cli.yaml`** → `$XDG_CONFIG_HOME/dbm-cli/config.yaml`

```yaml
default: prod-ro
datasources:
  # Oracle (use service_name OR sid)
  prod-ro:
    type: oracle
    host: 10.0.0.1
    port: 1521
    service_name: ORCLPDB1
    user: readonly
    password: ${DB_PWD_PROD} # env-var expansion; avoids plaintext secrets
    allow_write: false       # read-only by default

  # MySQL (use database = schema name)
  mysql-prod:
    type: mysql
    host: 10.0.0.2
    port: 3306
    database: app_db         # required for MySQL
    user: readonly
    password: ${DB_PWD_MYSQL}
    allow_write: false
    # tls: skip-verify        # optional: enable TLS for self-signed certs

  # MariaDB — protocol-compatible with MySQL; use type: mariadb (alias of mysql)
  maria-prod:
    type: mariadb             # alias -> mysql driver; works the same as type: mysql
    host: 10.0.0.10
    port: 3306
    database: app_db
    user: readonly
    password: ${DB_PWD_MARIA}
    allow_write: false

  # ClickHouse (native TCP port 9000)
  ck-prod:
    type: clickhouse
    host: 10.0.0.4
    port: 9000               # native TCP port (NOT 8123 which is HTTP)
    database: analytics
    user: default
    password: ${DB_PWD_CK}
    allow_write: false
    # tls: skip-verify        # optional: enable TLS

  # Writable datasource (any type)
  dev-rw:
    type: mysql
    host: 10.0.0.3
    port: 3306
    database: app_db
    user: dev
    password: ${DB_PWD_DEV}
    allow_write: true
```

> **MySQL note:** `database` is required (MySQL's notion of schema == database). 8.0's default auth plugin `caching_sha2_password` works without extra config.

> **Connection timeout:** add `timeout: 10s` to any datasource. dbm-cli verifies the connection on first use; if it can't connect within the timeout it fails fast with a clear message (host/port/credentials/network hints). Default 30s.

> **Datasource description:** add an optional `description: "..."` to any datasource — a human/AI-readable note of its role (e.g. *"prod orders DB, read-replica"*). Shown by `dbm-cli config` so an agent can pick the right datasource by purpose, not just by name.

## 🚀 Usage

```bash
dbm-cli config                                          # show config overview (datasources, default; passwords hidden)
dbm-cli config    -o json                               # ...as JSON (for AI/scripts to pick a datasource)
dbm-cli datasources                                     # list configured datasources (passwords hidden)
dbm-cli version   -d prod-ro                            # database version
dbm-cli databases -d prod-ro                            # databases / PDBs
dbm-cli schemas   -d prod-ro --like HR%                 # schemas
dbm-cli tables    -d prod-ro --schema HR                # tables
dbm-cli columns   -d prod-ro --table EMPLOYEES --schema HR
dbm-cli indexes   -d prod-ro --table EMPLOYEES
dbm-cli views     -d prod-ro --schema HR
dbm-cli table     -d prod-ro --name EMPLOYEES --schema HR --limit 20 -o json
dbm-cli query     -d prod-ro "SELECT * FROM HR.EMPLOYEES WHERE ROWNUM<=10"
dbm-cli manifest                                        # self-describing JSON for AI agents
```

Global flags: `-c/--config`, `-d/--datasource`, `-o/--output {table|json|csv|yaml|vertical}`, `--no-header`

#### Custom SQL (`query`)

`query` runs any SQL. Read-only statements execute directly; writes go through the `allow_write` guard (off by default), with a second confirmation for destructive statements (`DROP`/`TRUNCATE`, `DELETE`/`UPDATE` without `WHERE`).

**Three SQL sources** (priority: `--file` > stdin > argument):

```bash
dbm-cli query -d prod-ro "SELECT * FROM HR.EMPLOYEES WHERE ROWNUM<=10"   # argument
dbm-cli query -d prod-ro -f report.sql                                    # from file
echo "SELECT COUNT(*) FROM orders" | dbm-cli query -d prod-ro             # stdin / pipe / heredoc
```

**Parameterized queries** (SQL-injection-safe — preferred over string concatenation). Write `?` as the placeholder; it is auto-translated to each engine's native style (`?` for MySQL/ClickHouse, `$1` for PostgreSQL, `:1` for Oracle):

```bash
dbm-cli query -d prod-ro "SELECT * FROM users WHERE id=? AND status=?" --param 100 --param active
```

**Result-set guard** — `--limit` caps rows returned by read-only queries (default `1000`, `<=0` disables) to prevent an accidental `SELECT *` from pulling a huge table:

```bash
dbm-cli query -d prod-ro -f big-report.sql --limit 500
```

Flags: `--file/-f`, `--param` (repeatable, positional binding), `--limit` (default 1000), `--yes` (skip destructive confirmation).

#### Output formats

| Format | Description | Best for |
|--------|-------------|----------|
| `table` (default) | Bordered aligned table (`+---+`, `| a | b |`), CJK-width aware | human reading in a terminal |
| `json` | Pretty-printed array of objects (2-space indent, ISO8601 times) | programs / AI agents |
| `csv` | Standard CSV with a header row | spreadsheets / data exchange |
| `yaml` | YAML array of objects | config-style reading |
| `vertical` | `\G` style: one record per block (`col: value`) | wide tables, few rows |

```bash
dbm-cli tables -d prod-ro --schema HR                      # bordered table
dbm-cli tables -d prod-ro --schema HR -o json              # JSON
dbm-cli table  -d prod-ro --name EMP --limit 5 -o csv      # CSV
```

### Exit codes & errors (for automation / AI)

| Exit | Meaning | Action |
|------|---------|--------|
| 0 | success | — |
| 1 | runtime error (SQL error, query failure, network) | usually retryable after fixing the query |
| 2 | config / connection-class error (missing config, write-guard rejection) | fix config or `allow_write` before retry |

All errors go to stderr as `[dbm-cli] error: <message>`, often followed by a `[dbm-cli] hint:` line telling you exactly what to change before retrying.

## 🔌 MCP server (for AI clients)

`dbm-cli mcp` runs the tool as a **Model Context Protocol** server over stdio. AI clients (Claude Desktop, Cursor, etc.) can connect to it and call the database directly — no need to shell out to CLI commands.

It exposes **11 tools**, mirroring the CLI one-to-one:

| MCP tool | Equivalent CLI | Purpose |
|----------|----------------|---------|
| `list_datasources` | `datasources` | list configured datasources (no connection) |
| `get_version` | `version` | engine + version |
| `list_databases` | `databases` | databases / PDBs |
| `list_schemas` | `schemas` | schemas / users |
| `list_tables` | `tables` | tables in a schema |
| `describe_table` | `columns` | column definitions |
| `list_indexes` | `indexes` | table indexes |
| `list_views` | `views` | views |
| `sample_table` | `table` | paginated table data |
| `query` | `query` (read) | read-only SQL with `?` placeholders |
| `execute` | `query` (write) | write SQL, gated by `allow_write` |

**Safety is inherited, not re-implemented**: every connection goes through the same `allow_write` guard as the CLI, so a read-only datasource rejects writes identically. Since MCP has no interactive terminal, the `execute` tool is *stricter* than the CLI — destructive statements (`DROP` / `TRUNCATE` / `DELETE`/`UPDATE` without `WHERE`) require an explicit `confirm_destructive: true` argument.

### Configure in Claude Desktop

```jsonc
{
  "mcpServers": {
    "dbm-cli": {
      "command": "/path/to/dbm-cli",
      "args": ["mcp"]
      // optional: "args": ["mcp", "-c", "/path/to/dbm-cli.yaml"]
    }
  }
}
```

The server reads the same YAML config as the CLI (see [Configuration](#️-configuration)). Each tool takes an optional `datasource` argument (falls back to the configured `default`).

> The `mcp` subcommand is additive — it does not change any existing CLI command. Users who never call `dbm-cli mcp` are completely unaffected.

## 🏗️ Architecture

dbm-cli is built around a driver abstraction so that adding a database touches almost no existing code. Core patterns:

| Pattern | Where |
|---------|-------|
| Registry / Factory | `internal/driver/registry.go` — drivers self-register |
| Strategy | different `MetadataProvider` implementations per database |
| Adapter | `internal/oracle` adapts go-ora to the unified `Conn` |
| Decorator / Guard | `WriteGuard` enforces `allow_write` |
| Facade | `pkg/dbmcli` stable public API |
| Builder | connection-string and paged-SQL construction |

**Adding a datasource (e.g. MySQL):** create `internal/mysql/`, implement `Driver`/`Conn`/`MetadataProvider`, call `driver.Register` in `init()`, and add one line to `cmd/dbm-cli/main.go`: `import _ "github.com/golango-cn/dbm-cli/internal/mysql"`. The CLI, output layer and manifest pick it up automatically.

## 📚 Programmatic API

A stable Facade lives in `pkg/dbmcli` for embedding in other Go programs:

```go
import "github.com/golango-cn/dbm-cli/pkg/dbmcli"

conn, cleanup, err := dbmcli.Open(ctx, &dbmcli.Datasource{
    Type: "oracle", Host: "10.0.0.1", Port: 1521,
    ServiceName: "ORCLPDB1", User: "ro", Password: os.Getenv("PWD"),
})
defer cleanup()
tables, _ := conn.Metadata().Tables(ctx, "HR")
```

## 📌 Roadmap

- [x] M0 Skeleton
- [x] M1 Oracle connectivity (driver, version detection, ping)
- [x] M2 Metadata queries (multi-version dictionary SQL)
- [x] M3 Data queries (`table` paging, `query` read path)
- [x] M4 Writes & guard (`allow_write`, classification, destructive confirmation)
- [x] M5 Output formats
- [x] M6 Manifest (driver self-description injection)
- [x] **MySQL driver** (5.6 / 5.7 / 8.0.x, verified end-to-end against 8.0.37)
- [x] **Oracle driver** verified end-to-end against 18c XE
- [x] **ClickHouse driver** (22.x+, verified end-to-end against 25.8)
- [x] **PostgreSQL driver** (9.6+, verified end-to-end against 12 / 17)
- [x] **Apache Impala driver** (3.x/4.x, verified end-to-end against 4.5.0)
- [x] **SQL Server driver** (2017+, verified end-to-end against 2017 / 2022)
- [ ] M7 Polish: unit tests, cross-platform release

> All four drivers (Oracle, MySQL, ClickHouse, PostgreSQL) are complete, pass `go vet` + `go build`, and have been verified end-to-end against live instances: Oracle 11g/18c XE, MySQL 5.7/8.0.12/8.0.37, ClickHouse 25.8, PostgreSQL 12/17, Apache Impala 4.5.0, SQL Server 2017/2022.

## 🤝 Contributing

Issues and PRs welcome. Please run `go vet ./...` and `go test ./...` before submitting.

## 📄 License

[MIT](LICENSE) © golango
