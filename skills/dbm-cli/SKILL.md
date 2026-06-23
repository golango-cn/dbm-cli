---
name: dbm-cli
description: Query database metadata and run SQL via dbm-cli — a zero-dependency, single-binary CLI + MCP server supporting MySQL/PostgreSQL/Oracle/SQL Server/ClickHouse/Impala. Use when the user asks to inspect a database's structure (tables, columns, indexes, views, schemas), browse sample data, or execute SQL against a configured datasource. Also use to install/configure dbm-cli as an MCP server for AI clients (Claude Code, ZCode, Cursor).
---

# dbm-cli — Database Metadata & SQL CLI + MCP server

`dbm-cli` is a **zero-dependency, single-binary** tool for querying database **metadata** and **data**, and exposing the same capabilities as an **MCP server**. It is the primary tool for an agent to explore an unknown database schema, inspect table structures, and run SQL — without writing any application code.

Supported engines: **MySQL / MariaDB** (5.7 / 8.0), **PostgreSQL** (9.6+), **Oracle** (10.2+), **SQL Server** (2017+), **ClickHouse** (22.x+), **Apache Impala** (3.x/4.x). All via pure-Go drivers (no Instant Client, no CGO, no ODBC).

> This skill is written for **agent self-service**: an AI reading this should be able to **install, configure, and use** dbm-cli end-to-end without further human help.

---

## When to use

- The user wants to **understand a database's structure** — tables, columns/types, indexes, views, schemas.
- The user wants to **browse sample rows** of a table without writing a full query.
- The user wants to **run SQL** (read or write) against a configured datasource.
- The user wants to **add a new database** to dbm-cli, or **set up dbm-cli as an MCP server** for their AI client.
- You need to **discover what datasources are configured** before connecting.

Do **not** use for: schema migrations, long-running ETL, or a database with no datasource configured (configure one in the YAML first — see [Step 2: Configure](#step-2-configure-datasources--dbm-cliyaml)).

---

## Step 1: Verify install (or install if missing)

**Check first** — dbm-cli may already be installed:

```bash
which dbm-cli && dbm-cli manifest | head -5        # installed → prints tool/version/drivers
```

- `dbm-cli manifest` is the canonical "is it working?" check: it prints self-describing JSON **without needing any database connection**. The `version` field gives the build version. (There is **no** `--version` flag.)
- `which dbm-cli` returns the binary path; common locations are `/usr/local/bin/dbm-cli` (system-wide, needs sudo) or `~/.local/bin/dbm-cli` (user, no sudo).

### If not installed: use the prebuilt binary bundled with this skill

A static, precompiled binary ships alongside this skill at **`./bin/dbm-cli`** (same directory as this `SKILL.md`). It's a self-contained Linux amd64 binary — **no Go toolchain, no build step, no external libraries needed**. Just copy it onto your PATH:

```bash
# SKILL_DIR = the directory containing this SKILL.md (also holds ./bin/dbm-cli)

# 1. install onto PATH (prefer user-writable ~/.local/bin — no sudo needed)
install -m 0755 "$SKILL_DIR/bin/dbm-cli" ~/.local/bin/dbm-cli

# 2. system-wide alternative (needs sudo):
#    sudo install -m 0755 "$SKILL_DIR/bin/dbm-cli" /usr/local/bin/dbm-cli

# 3. verify
dbm-cli manifest | head -5
```

Notes for the agent:
- The bundled binary is **static** (`CGO_ENABLED=0`) and works on any Linux amd64 without dependencies — just copy and run.
- To locate `$SKILL_DIR`: it's the directory of this very `SKILL.md` file. If you have the skill path from the harness, use it directly; otherwise ask the user where the skill was installed.
- **Do not rebuild from source** unless the user explicitly asks for a custom build (e.g. different OS/arch). If you need another platform, cross-compile from the source repo: `git clone https://github.com/golango-cn/dbm-cli.git && cd dbm-cli && make dist` (outputs to `./dist/`).
- If `which dbm-cli` finds a stale older copy without the `mcp` subcommand, overwrite it with the bundled binary, then re-verify via `dbm-cli --help | grep mcp`.
- Architecture mismatch? If the target host is not Linux amd64 (e.g. macOS, arm64), the bundled binary won't run — fall back to `make dist` cross-compile, and tell the user.

---

## Step 2: Configure datasources (`~/.dbm-cli.yaml`)

All database connections live in **one YAML file**, shared by both the CLI and the MCP server. The MCP server reads **the same file** — never put host/user/password in the MCP client's JSON config.

**File lookup order** (first match wins): `--config <path>` → `./.dbm-cli.yaml` → `./dbm-cli.yaml` → `~/.dbm-cli.yaml` → `$XDG_CONFIG_HOME/dbm-cli/config.yaml` → `~/.config/dbm-cli/config.yaml`.

**Recommended location: `~/.dbm-cli.yaml`** (home dir, works from any cwd).

### Datasource naming convention

**Always name datasources** `<ip>_<datasourcetype>_<database>_<env>`:

```
<ip>_<type>_<database>_<env>
10.10.239.152_mysql_bdmp_dev
10.10.0.7_postgresql_appdb_prod
10.10.0.10_sqlserver_appdb_prod
```

- `<ip>`: the host IP (distinguishes multiple DBs on different hosts)
- `<type>`: `mysql` / `postgresql` / `oracle` / `sqlserver` / `clickhouse` / `impala` (or `mariadb`)
- `<database>`: the database/schema name
- `<env>`: `dev` / `test` / `prod` — **encodes risk level in the name** so neither AI nor human accidentally runs writes against prod.

### Full example for every supported engine

A complete reference is committed next to this skill at **`.dbm-cli.yaml.example`**. To set up, copy and edit it:

```bash
cp ./.dbm-cli.yaml.example ~/.dbm-cli.yaml   # then edit host/user/password
```

That example contains **one block per engine**, all following the naming convention:

| type | Key fields | Notes |
|------|-----------|-------|
| `mysql` / `mariadb` | `database` (required; schema == database) | MariaDB uses `type: mariadb` (alias → mysql driver). Optional `tls: skip-verify`. |
| `postgresql` | `database` (required) | DB and schema are two layers; pass `--schema` at query time (default `public`). |
| `oracle` | `service_name` **or** `sid` (one required) | Version auto-detected 10g–21c; restricted accounts use `force_version: "11g"`. |
| `sqlserver` | `database` (required) | Default schema is `dbo`; pure-Go driver, no ODBC. |
| `clickhouse` | `database` (required; == schema) | **Port 9000** (native TCP), **not** 8123 (HTTP). |
| `impala` | `database` (required; == schema) | **Port 21050** (HiveServer2), **not** 21000. user/password optional on no-auth clusters. |

**Common fields for all types**: `description` (helps AI distinguish same-type sources), `allow_write` (default **false** = read-only, writes blocked by a guard), `timeout` (default 30s), `password` (supports `${ENV_VAR}` expansion — prefer this over inline plaintext).

Minimal MySQL example:

```yaml
default: 10.10.239.152_mysql_bdmp_dev
datasources:
  10.10.239.152_mysql_bdmp_dev:
    type: mysql
    description: "Dev MySQL (bdmp, read-only)"
    host: 10.10.239.152
    port: 3309
    database: bdmp
    user: root
    password: ${DB_PWD_152_MYSQL_BDMP}   # env var; or inline for local-only
    allow_write: false
```

### Verify the config

After writing `~/.dbm-cli.yaml`:

```bash
dbm-cli datasources        # lists sources (names, types, hosts; passwords masked)
dbm-cli version -d <name>  # connects + pings; confirms the source actually works
```

---

## Step 3: Register dbm-cli as an MCP server

`dbm-cli mcp` runs the tool as a **Model Context Protocol** server over **stdio**. It exposes the same 11 capabilities as MCP tools (see [MCP usage](#using-the-mcp-server) below). It reads the **same `~/.dbm-cli.yaml`** — so once Step 2 is done, registration is just pointing the client at the binary.

> The `mcp` subcommand is **additive**: it does not change any other CLI command. Users who never run it are unaffected.

### The registration entry (same for all clients)

```json
"dbm-cli": {
  "command": "<ABSOLUTE_PATH_TO_dbm-cli>",
  "args": ["mcp"],
  "env": {}
}
```

- Use an **absolute path** for `command` (e.g. `/home/USER/.local/bin/dbm-cli`) — MCP clients spawn the process and may not inherit a full interactive PATH.
- Optional: add `-c /path/to/.dbm-cli.yaml` to `args` if your config lives in a non-default location: `"args": ["mcp", "-c", "/path/to/.dbm-cli.yaml"]`.
- Database passwords: if your YAML uses `${ENV_VAR}` placeholders, the client process must inherit those env vars — either export them in the shell profile, or put them in the `env` object: `"env": { "DB_PWD_PROD": "..." }`.

### Claude Code (and Claude-Code-compatible clients: ZCode, etc.)

Claude Code stores MCP servers in **`~/.agents/mcp.json`** under the `mcpServers` key. This is the canonical file for ZCode too.

```json
{
  "mcpServers": {
    "dbm-cli": {
      "command": "/home/ningzi/.local/bin/dbm-cli",
      "args": ["mcp"],
      "env": {}
    }
  }
}
```

After editing, **restart the client** so it spawns the new server. Verify inside the client with its `/mcp` command — `dbm-cli` should appear with 11 tools.

> Note for the agent: when configuring ZCode specifically, the file is `~/.agents/mcp.json` (standard `mcpServers` shape) — **not** `~/.zcode/v2/config.json` (that file is for model providers only, despite ZCode being OpenCode-derived).

### Claude Desktop

Edit `claude_desktop_config.json` (macOS: `~/Library/Application Support/Claude/`; Linux/Windows: per-platform app config dir), same shape:

```json
{
  "mcpServers": {
    "dbm-cli": {
      "command": "/home/ningzi/.local/bin/dbm-cli",
      "args": ["mcp"]
    }
  }
}
```

### Verify the MCP server works (independent of any client)

Pipe a JSON-RPC handshake straight into the binary to confirm it lists tools:

```bash
{ printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"probe","version":"1"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'; sleep 0.3; } | dbm-cli mcp
```

Expect an `initialize` result naming `dbm-cli`, then a `tools/list` result with 11 tools (`describe_table`, `execute`, `get_version`, `list_databases`, `list_datasources`, `list_indexes`, `list_schemas`, `list_tables`, `list_views`, `query`, `sample_table`).

---

## Using the MCP server

When running as an MCP server, dbm-cli exposes **11 tools**, mirroring the CLI one-to-one:

| MCP tool | ≈ CLI command | Purpose |
|----------|---------------|---------|
| `list_datasources` | `datasources` | list configured sources (no connection) |
| `get_version` | `version` | engine + version |
| `list_databases` | `databases` | databases / Oracle PDBs |
| `list_schemas` | `schemas` | schemas / users |
| `list_tables` | `tables` | tables in a schema |
| `describe_table` | `columns` | column definitions |
| `list_indexes` | `indexes` | table indexes |
| `list_views` | `views` | views |
| `sample_table` | `table` | paginated table data |
| `query` | `query` (read) | read-only SQL with `?` placeholders |
| `execute` | `query` (write) | write SQL, gated by `allow_write` |

### Recommended exploration flow (read path)

```
list_datasources  →  get_version  →  list_databases / list_schemas  →  list_tables  →  describe_table  →  sample_table / query
```

Each tool takes an optional `datasource` argument (falls back to the configured `default`). Use `list_datasources` first; never guess a datasource name.

### Safety model (identical to the CLI, stricter on writes)

- Every datasource has an `allow_write` switch (**read-only by default**). The `execute` tool is rejected on read-only sources — surface this to the user rather than retrying.
- Since MCP has **no interactive terminal**, `execute` is *stricter* than the CLI: destructive statements (`DROP`, `TRUNCATE`, or `DELETE`/`UPDATE` without a `WHERE` clause) require an explicit `confirm_destructive: true` argument.
- Use the **`query` tool for reads** and the **`execute` tool for writes** — never run write SQL through `query`.
- For parameterized SQL, use `?` placeholders and pass `params` (an array); dbm-cli translates `?` to each engine's native style (`?` for MySQL/ClickHouse, `$1` for PostgreSQL, `:1` for Oracle).

---

## Using the CLI directly (when not on an MCP client)

Same capabilities, as shell commands. Use `-o json` whenever you will parse output yourself.

```bash
dbm-cli manifest                              # self-describing JSON (no DB needed)
dbm-cli datasources                           # list configured sources
dbm-cli version   -d <ds>                     # connectivity + engine/version
dbm-cli schemas   -d <ds> [-l PAT]            # list schemas (-l = --like)
dbm-cli tables    -d <ds> [-s <S>] [-l PAT]   # list tables (-s = --schema)
dbm-cli columns   -d <ds> -t <T> [-s <S>]     # table structure (read before writing SQL!) (-t = --table)
dbm-cli indexes   -d <ds> -t <T>              # indexes / primary key
dbm-cli views     -d <ds> [-s <S>]            # list views
dbm-cli table     -d <ds> -t <T> -n 10        # paginated sample rows (-t = --name, -n = --limit)
dbm-cli query     -d <ds> "SELECT * FROM t WHERE id=?" --param 100 --param active
```

### Parameterized queries (preferred — SQL-injection-safe)

Write `?` as the placeholder; dbm-cli auto-translates to each engine's native style. `--param` is repeatable and binds positionally:

```bash
dbm-cli query -d <ds> "SELECT * FROM users WHERE id=? AND status=?" --param 100 --param active
```

Numeric-only strings bind as integers; the `--param` count must equal the `?` count or the command errors.

### Read vs write, and the safety guard

- **Read-only** SQL (`SELECT`/`WITH`) runs immediately; `--limit` (default **1000**) caps returned rows. `--limit 0` disables the cap.
- **Write** SQL is blocked unless the datasource has `allow_write: true`. Even then, **destructive** statements (`DROP`/`TRUNCATE`/`DELETE`/`UPDATE` without `WHERE`) require interactive confirmation — pass `--yes` for scripted/agent use:
  ```bash
  dbm-cli query -d <ds-rw> "DELETE FROM tmp WHERE id=1" --yes
  ```
- A `write disabled` error (exit code 2) is intentional — tell the user to set `allow_write: true` if the write is truly intended; do not retry as-is.

### Output formats (`-o/--output`)

| Format | When |
|--------|------|
| `json` | **Default for agent use** — parseable, typed, ISO8601 timestamps |
| `table` | Human-readable aligned table (default; CJK-width aware) |
| `csv` | spreadsheets / data exchange |
| `vertical` | wide tables, few rows |
| `yaml` | config-style reading |

`--no-header` omits the header in `table`/`csv`.

### Exit codes

| Exit | Meaning | Agent action |
|------|---------|--------------|
| 0 | success | parse output |
| 1 | runtime error (SQL, query, network) | read error, fix query, retry |
| 2 | config/connection-class error (missing config, write-guard rejection) | fix config / `allow_write`; do not blindly retry |

All errors print to stderr as `[dbm-cli] error: <message>`, often with a `[dbm-cli] hint:` line. **Read the hint** before retrying.

---

## Agent guardrails

- **Discover before connecting.** Run `datasources`/`list_datasources` first; never guess a datasource name.
- **Respect the naming convention.** `<ip>_<type>_<database>_<env>` — the `_prod` suffix means be careful; prefer a `_dev`/`_test` source for writes.
- **Prefer `--like`/`--limit` and tool-native filters** over pulling entire catalogs.
- **Prefer parameterized `?` + `--param`** over string-concatenated values.
- **Respect `allow_write`.** A read-only rejection (exit 2) is intentional; surface it, don't retry.
- **Use `-o json`** when parsing results; `table` only for human display.
- **Read the hint line** in any error before retrying.
- When configuring an MCP client, **use the absolute binary path** and remember the client process must inherit any `${ENV_VAR}` the YAML references.
