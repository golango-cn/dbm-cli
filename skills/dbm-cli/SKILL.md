---
name: dbm-cli
description: Query database metadata and run SQL via the dbm-cli command-line tool. Use when the user asks to inspect a database's structure (tables, columns, indexes, views, schemas), browse sample data, or execute SQL against an Oracle/MySQL/PostgreSQL/ClickHouse database reachable by a configured datasource.
---

# dbm-cli — Database Metadata & SQL CLI

`dbm-cli` is a zero-dependency, single-binary CLI for querying database **metadata** and **data**. It is the primary tool for an agent to explore an unknown database schema, inspect table structures, and run SQL — without writing any application code.

Supported engines: **Oracle** (10.2+), **MySQL / MariaDB** (5.7 / 8.0), **PostgreSQL** (9.6+), **ClickHouse** (22.x+). All via pure-Go drivers (no Instant Client, no CGO).

## When to use

- The user wants to **understand a database's structure** — what tables exist, what columns/types a table has, its indexes or views.
- The user wants to **browse sample rows** of a table without writing a full query.
- The user wants to **run a SQL query** (read or write) against a configured datasource.
- You need to **discover what datasources are configured** before connecting.

Do **not** use this for: schema migrations, long-running ETL, or connecting to a database that has no `dbm-cli` datasource configured (configure one in the YAML first — see "Configuring a datasource").

## First step: discover what's available

Before running any database command, learn the environment. This needs **no database connection**:

```bash
dbm-cli manifest          # full self-describing JSON: commands, flags, drivers, datasources, examples
dbm-cli datasources       # list configured datasources (names, types, hosts; passwords hidden)
dbm-cli config -o json    # config overview as JSON (good for picking a datasource programmatically)
```

The `-d <datasource>` flag selects a datasource; if omitted, the configured `default` is used. When multiple datasources of the same type exist, their `description` field tells them apart.

## Standard schema-exploration workflow

When asked to explore or understand a database, follow this drill-down order. Always use `-o json` when you will parse the output yourself:

```bash
dbm-cli version   -d <ds>                     # 1. confirm connectivity + learn engine/version
dbm-cli schemas   -d <ds>                     # 2. list schemas (Oracle: users; MySQL: databases)
dbm-cli tables    -d <ds> --schema <S>        # 3. list tables in a schema
dbm-cli columns   -d <ds> --table <T> --schema <S>   # 4. inspect a table's columns (type, nullable, comment)
dbm-cli indexes   -d <ds> --table <T>         # 5. inspect indexes / primary key
dbm-cli views     -d <ds> --schema <S>        # 6. list views
dbm-cli table     -d <ds> --name <T> --limit 10 -o json   # 7. peek at sample rows
```

Tips:
- `tables` and `schemas` accept `--like <PATTERN>` (SQL `LIKE`, e.g. `USER%`) to narrow results — use it instead of pulling everything and filtering in your head.
- `columns` is the richest single command: it returns position, data type, length/precision/scale, nullability, default value, and comment. Read it before writing any SQL against an unfamiliar table.
- `table` paginates with `--limit`/`--offset`/`--order` and works across all engines (handles dialect differences like Oracle's `ROWNUM` internally).

## Running custom SQL (`query`)

`query` runs any SQL. Three input sources, by priority:

| Priority | Source | Example |
|----------|--------|---------|
| 1 | `--file` / `-f` | `dbm-cli query -d <ds> -f report.sql` |
| 2 | stdin (when piped) | `echo "SELECT 1" \| dbm-cli query -d <ds>` |
| 3 | command argument | `dbm-cli query -d <ds> "SELECT 1"` |

### Parameterized queries (preferred — SQL-injection-safe)

Always prefer parameter binding over string-interpolating values into SQL. Write `?` as the placeholder; `dbm-cli` translates it to each engine's native style automatically (`?` for MySQL/ClickHouse, `$1` for PostgreSQL, `:1` for Oracle):

```bash
dbm-cli query -d <ds> "SELECT * FROM users WHERE id=? AND status=?" --param 100 --param active
```

- `--param` is **repeatable**; values bind **positionally** to `?` in order.
- The number of `--param` values must equal the number of `?` placeholders, or the command errors.
- Numeric-only strings are bound as integers; everything else as strings.

### Read vs write, and the safety guard

- **Read-only** statements (`SELECT`, `WITH`) execute immediately. `--limit` (default **1000**) caps returned rows to protect against an accidental `SELECT *` on a huge table. Set `--limit 0` to disable the cap.
- **Write** statements (DML/DDL/transaction) are blocked unless the datasource has `allow_write: true`. Even then, **destructive** statements (`DROP`, `TRUNCATE`, or `DELETE`/`UPDATE` without a `WHERE` clause) require interactive confirmation — pass `--yes` to skip it (for scripted/agent use).

```bash
dbm-cli query -d dev-rw "DELETE FROM tmp WHERE id=1" --yes   # pre-approved destructive write
```

If a write fails with `write disabled`, that datasource is read-only by design — do not retry; tell the user to enable `allow_write` in config if the write is truly intended.

## Output formats

Global flag `-o/--output` controls rendering. Choose by consumer:

| Format | When to use |
|--------|-------------|
| `json` | **Default for agent use** — parseable, typed values, ISO8601 timestamps |
| `table` | Human-readable aligned table (default; CJK-width aware) |
| `csv` | Export to spreadsheets / data exchange |
| `vertical` | Wide tables with few rows (one record per block, `col: value`) |
| `yaml` | Config-style reading |

`--no-header` omits the header row in `table`/`csv`.

## Exit codes & error handling

| Exit | Meaning | Agent action |
|------|---------|--------------|
| 0 | success | parse output |
| 1 | runtime error (SQL error, query failure, network) | read the error, fix the query/SQL, retry |
| 2 | config/connection-class error (missing config, write-guard rejection) | fix config or datasource; do not blindly retry the same command |

All errors print to stderr as `[dbm-cli] error: <message>`, often with a `[dbm-cli] hint:` line describing what to change. **Read the hint** before retrying.

## Configuring a datasource

Datasources live in a YAML file found by: `--config <path>` → `./dbm-cli.yaml` → `~/.dbm-cli.yaml` → `$XDG_CONFIG_HOME/dbm-cli/config.yaml`. Minimal example:

```yaml
default: prod-ro
datasources:
  prod-ro:                       # read-only production
    type: oracle
    description: "Prod read replica (HR schema)"
    host: db.prod.example.com
    port: 1521
    service_name: PROD
    user: app_ro
    password: ${DB_PWD_PROD}     # env var expansion; never inline secrets
    allow_write: false           # default; safe
  dev-rw:                        # writable dev
    type: postgresql
    host: 127.0.0.1
    port: 5432
    database: appdb
    user: dev
    password: ${DB_PWD_DEV}
    allow_write: true
```

Key fields: `type` (oracle/mysql/mariadb/postgresql/clickhouse), `host`, `port`, `user`, `password` (supports `${ENV_VAR}`), `allow_write` (default false), `timeout` (default 30s), `description` (helps you pick the right datasource).

## Reference: all commands

| Command | Purpose | Key flags |
|---------|---------|-----------|
| `manifest` | Self-describing JSON (no DB connection) | — |
| `config` | Config overview (masked) | `-o json` |
| `datasources` | List datasources | — |
| `version` | Database version | `-d` |
| `databases` | List databases / Oracle PDBs | `-d` |
| `schemas` | List schemas/users | `--like` |
| `tables` | List tables | `--schema`, `--like` |
| `columns` | Table structure | `--table` (req), `--schema` |
| `indexes` | Table indexes | `--table` (req), `--schema` |
| `views` | List views | `--schema` |
| `table` | Paged table-data read | `--name` (req), `--schema`, `--limit`, `--offset`, `--order` |
| `query` | Run any SQL | `--file/-f`, `--param`, `--limit`, `--yes` |

Global flags (all commands): `-c/--config`, `-d/--datasource`, `-o/--output`, `--no-header`.

## Agent guardrails

- **Discover before connecting.** Run `datasources`/`manifest` first; never guess a datasource name.
- **Prefer `--like` and `--limit`** over pulling entire catalogs — narrow early.
- **Prefer parameterized `?` + `--param`** over string-concatenated values in SQL.
- **Respect `allow_write`.** A read-only rejection (exit 2) is intentional; surface it to the user rather than retrying.
- **Use `-o json`** when you will parse results; use `table` only when showing output to the human.
- **Read the hint line** in any error before retrying — it usually names the exact fix.
