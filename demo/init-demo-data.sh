#!/usr/bin/env bash
# init-demo-data.sh —— 在指定数据源上创建统一演示表 dbm_demo
#
# 自动识别数据源类型（mysql/mariadb/oracle/postgresql/clickhouse），
# 按对应方言建表并插入 5 行示例数据（含中文，用于测试 CJK 对齐）。
# 幂等：表已存在则跳过创建，数据先清空再插。
#
# 用法：./init-demo-data.sh <数据源名>
#   数据源需在 dbm-cli.yaml 中定义且 allow_write: true
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
CLI="$DIR/dbm-cli"
CONFIG="$DIR/dbm-cli.yaml"
DS="${1:-}"

[ -x "$CLI" ] || { echo "缺少 $CLI 二进制；请先 go build -o $CLI ../cmd/dbm-cli"; exit 1; }
[ -f "$CONFIG" ] || { echo "缺少 $CONFIG；请参考 ../examples/config.yaml.example 创建"; exit 1; }
[ -n "$DS" ] || DS="$(sed -n 's/^default:[[:space:]]*//p' "$CONFIG")"
[ -n "$DS" ] || { echo "未指定数据源：用法 $0 <数据源名>，或在 dbm-cli.yaml 设 default"; exit 1; }

# 探测数据源类型：通过 version 命令的 product 字段（MySQL/Oracle/ClickHouse/PostgreSQL）
PRODUCT="$("$CLI" -c "$CONFIG" -d "$DS" version 2>/dev/null | awk -F': *' '/^product/{print $2; exit}' | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')"
case "$PRODUCT" in
  mysql|mariadb) TYPE="mysql" ;;
  oracle) TYPE="oracle" ;;
  postgresql) TYPE="postgresql" ;;
  clickhouse) TYPE="clickhouse" ;;
  *) echo "无法识别数据源 $DS 的产品类型 (product=$PRODUCT)"; exit 1 ;;
esac

run() { "$CLI" -c "$CONFIG" -d "$DS" query "$1" --yes 2>&1 || true; }

echo "在数据源 [$DS] (type=$TYPE) 上初始化演示表 dbm_demo ..."

case "$TYPE" in
  mysql|mariadb)
    run "CREATE TABLE IF NOT EXISTS dbm_demo (id INT PRIMARY KEY AUTO_INCREMENT, name VARCHAR(64) NOT NULL COMMENT 'name', dept VARCHAR(32), score DECIMAL(5,1), created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)"
    run "DELETE FROM dbm_demo"
    run "INSERT INTO dbm_demo (name,dept,score) VALUES ('Alice','Engineering',95.5),('Bob','Sales',82.0),('Charlie','Engineering',88.5),('Diana','Marketing',91.0),('Eve','Engineering',76.5)"
    ;;
  oracle)
    run "CREATE TABLE dbm_demo (id NUMBER PRIMARY KEY, name VARCHAR2(64) NOT NULL, dept VARCHAR2(32), score NUMBER(5,1), created_at TIMESTAMP DEFAULT SYSTIMESTAMP)"
    run "DELETE FROM dbm_demo"
    for i in 1 2 3 4 5; do
      case $i in 1) n="Alice";d="Engineering";s="95.5";; 2) n="Bob";d="Sales";s="82.0";; 3) n="Charlie";d="Engineering";s="88.5";; 4) n="Diana";d="Marketing";s="91.0";; 5) n="Eve";d="Engineering";s="76.5";; esac
      run "INSERT INTO dbm_demo (id,name,dept,score) VALUES ($i,'$n','$d',$s)"
    done
    run "COMMIT"
    ;;
  postgresql)
    run "CREATE TABLE IF NOT EXISTS dbm_demo (id SERIAL PRIMARY KEY, name VARCHAR(64) NOT NULL, dept VARCHAR(32), score NUMERIC(5,1), created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)"
    run "DELETE FROM dbm_demo"
    run "INSERT INTO dbm_demo (name,dept,score) VALUES ('Alice','Engineering',95.5),('Bob','Sales',82.0),('Charlie','Engineering',88.5),('Diana','Marketing',91.0),('Eve','Engineering',76.5)"
    ;;
  clickhouse)
    run "CREATE TABLE IF NOT EXISTS dbm_demo (id UInt32, name String, dept String, score Float64, created_at DateTime DEFAULT now()) ENGINE = MergeTree() ORDER BY id"
    run "TRUNCATE TABLE dbm_demo"
    run "INSERT INTO dbm_demo (id,name,dept,score) VALUES (1,'Alice','Engineering',95.5),(2,'Bob','Sales',82.0),(3,'Charlie','Engineering',88.5),(4,'Diana','Marketing',91.0),(5,'Eve','Engineering',76.5)"
    ;;
  *)
    echo "不支持的类型: $TYPE"; exit 1 ;;
esac

echo "完成。"
echo "--- 验证 ---"
"$CLI" -c "$CONFIG" -d "$DS" query "SELECT count(*) AS cnt FROM dbm_demo" 2>&1 | tail -4
