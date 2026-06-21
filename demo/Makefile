# dbm-cli 演示 Makefile
#
# 本目录是 dbm-cli 的快速体验环境。你需要：
#   1. 把编译好的二进制放到本目录，名为 dbm-cli
#   2. 创建 dbm-cli.yaml 配置文件（参考 ../examples/config.yaml.example）
#   3. make init   初始化统一演示表 dbm_demo
#   4. make help   查看可用目标
#
# 注意：dbm-cli（二进制）和 dbm-cli.yaml（含密码）已被 .gitignore 忽略，不会入库。
#       本 Makefile 不绑定任何具体库/表名，对任意配置都通用。

CLI    := $(CURDIR)/dbm-cli
CONFIG := $(CURDIR)/dbm-cli.yaml

# 演示用的数据源名（在 dbm-cli.yaml 里定义）。
# 可通过命令行覆盖：make demo DS=mydb
DS     ?= $(shell sed -n 's/^default:[[:space:]]*//p' $(CONFIG) 2>/dev/null)

# 统一演示表（由 init 目标创建）
TBL    := dbm_demo

.PHONY: help all manifest init demo meta data formats \
        mysql ck oracle postgres

## help: 显示本说明
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //'

## mysql: 跑 MySQL 演示脚本（version/databases/schemas/tables/columns/indexes/views/table/query/输出格式）
##   命令全部写死在 demo-mysql.sh，数据源 mysql837（MySQL 8.0.37）
mysql:
	@./demo-mysql.sh

## ck: 跑 ClickHouse 演示脚本（version/databases/tables/columns/indexes/views/table/query/输出格式）
##   命令全部写死在 demo-clickhouse.sh，数据源 ck25（ClickHouse 25.8）
ck:
	@./demo-clickhouse.sh

## oracle: 跑 Oracle 演示脚本（version/databases(PDB)/schemas/tables/columns/indexes/views/table/query）
##   命令全部写死在 demo-oracle.sh，数据源 oracle18c（Oracle 18c XE）
oracle:
	@./demo-oracle.sh

## postgres: 跑 PostgreSQL 演示脚本（version/databases/schemas/tables/columns/indexes/views/table/query）
##   命令全部写死在 demo-postgresql.sh，数据源 pg17（PostgreSQL 17.10）
postgres:
	@./demo-postgresql.sh

## manifest: 支持的驱动清单
manifest:
	@echo "==================== 支持的驱动 ===================="
	@$(CLI) -c $(CONFIG) manifest | python3 -c "import sys,json;d=json.load(sys.stdin);[print(f\"  {x['name']:12} aliases={x.get('aliases',[])} {x['versions']}\") for x in d['supported_drivers']]"

## init: 在数据源 $(DS) 上创建统一演示表 dbm_demo（需该数据源 allow_write: true）
init:
	@echo "==================== 初始化演示表（数据源: $(DS)）===================="
	@./init-demo-data.sh $(DS)

## demo: 查询演示表版本/表/结构/数据（默认数据源，用 DS= 覆盖）
demo:
	@echo "==================== 演示（数据源: $(DS)）===================="
	@echo "--- 版本 ---"
	@$(CLI) -c $(CONFIG) -d $(DS) version
	@echo "--- 表列表 ---"
	@$(CLI) -c $(CONFIG) -d $(DS) tables
	@echo "--- $(TBL) 表结构 ---"
	@$(CLI) -c $(CONFIG) -d $(DS) columns --table $(TBL)
	@echo "--- $(TBL) 数据 ---"
	@$(CLI) -c $(CONFIG) -d $(DS) table --name $(TBL) --limit 5 --order id

## meta: 演示元数据查询（databases/schemas/views）
meta:
	@echo "==================== 元数据查询（数据源: $(DS)）===================="
	@echo "--- databases ---"
	@$(CLI) -c $(CONFIG) -d $(DS) databases
	@echo "--- schemas ---"
	@$(CLI) -c $(CONFIG) -d $(DS) schemas

## data: 演示数据查询的多种输出格式
data:
	@echo "==================== 输出格式（数据源: $(DS)）===================="
	@echo "--- table（默认带边框）---"
	@$(CLI) -c $(CONFIG) -d $(DS) table --name $(TBL) --limit 3 --order id
	@echo "--- json ---"
	@$(CLI) -c $(CONFIG) -d $(DS) table --name $(TBL) --limit 2 --order id -o json
	@echo "--- csv ---"
	@$(CLI) -c $(CONFIG) -d $(DS) table --name $(TBL) --limit 2 --order id -o csv

## formats: 展示全部输出格式（table/json/csv/yaml/vertical）
formats:
	@for fmt in table json csv yaml vertical; do \
	  echo "--- $$fmt ---"; \
	  $(CLI) -c $(CONFIG) -d $(DS) query "SELECT * FROM $(TBL) LIMIT 1" -o $$fmt 2>&1 | head -6; \
	done

## test_write: 演示写守卫——只读库拦截 DELETE/UPDATE，可写库放行
##   对比 allow_write=false（game 库，应被拦截）与 allow_write=true（DS，应放行）
##   实际逻辑在 test-write-guard.sh（Makefile 的 $ 转义难以可靠捕获 $?，改用独立脚本）
test_write:
	@./test-write-guard.sh $(DS)

## all: manifest + 各数据库类型演示(mysql/ck/oracle/postgres) + meta + data
##   注：all 不含 demo/init（demo 依赖通用 DS、init 会写库），避免覆盖式副作用。
all: manifest mysql ck oracle postgres meta data
