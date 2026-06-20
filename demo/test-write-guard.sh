#!/usr/bin/env bash
# test-write-guard.sh —— 演示 dbm-cli 写守卫的拦截行为
#
# 对比 allow_write=false（game 库，应被拦截）与 allow_write=true（DS，应放行）。
# 用法：./test-write-guard.sh [数据源名]
#   数据源默认取 dbm-cli.yaml 的 default。
set -uo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
CLI="$DIR/dbm-cli"
CONFIG="$DIR/dbm-cli.yaml"
DS="${1:-$(sed -n 's/^default:[[:space:]]*//p' "$CONFIG")}"

[ -x "$CLI" ] || { echo "缺少 $CLI 二进制；请先 go build -o $CLI ../cmd/dbm-cli"; exit 1; }

# show 执行一条命令，打印其退出码。用 2>&1 合并 stderr 让拦截信息可见。
show() {
  local label="$1"; shift
  echo "$label"
  echo "  ----------------------------------------------------------------"
  "$@" 2>&1
  echo "  -> exit code=$?"
  echo ""
}

echo "==================== 写守卫测试 ===================="
echo ""

show "【场景1】只读数据源（game，allow_write=false）执行 DELETE —— 应被拦截
  期望：拒绝执行 + 提示 allow_write=false，exit code=2" \
  "$CLI" -c "$CONFIG" -d game query "DELETE FROM dbm_demo WHERE id=1" --yes

show "【场景2】只读数据源（game，allow_write=false）执行 UPDATE —— 应被拦截
  期望：拒绝执行 + 提示，exit code=2" \
  "$CLI" -c "$CONFIG" -d game query "UPDATE dbm_demo SET score=0 WHERE id=1" --yes

show "【场景3】只读数据源（game）执行无 WHERE 的 DELETE —— 同样被拦截（不会先弹确认）
  期望：直接拒绝，不会询问 y/N" \
  "$CLI" -c "$CONFIG" -d game query "DELETE FROM dbm_demo" --yes

# ---- DDL 语句同样受只读守卫拦截（建表/改表/删表/清空）----
echo "【DDL 拦截组】只读数据源（game）执行各类 DDL —— 均应被拦截，exit code=2"
echo "  原理：守卫按「只读 vs 非只读」二分类拦截，DDL 属非只读，无需逐关键字列举"
echo "  ----------------------------------------------------------------"
for SQL in \
  "CREATE TABLE t_guard_test (id INT)" \
  "ALTER TABLE dbm_demo ADD COLUMN c_guard VARCHAR(8)" \
  "DROP TABLE t_guard_test" \
  "TRUNCATE TABLE dbm_demo" \
  "CREATE INDEX idx_guard ON dbm_demo(id)"; do
  printf "  %-52s -> " "$SQL"
  "$CLI" -c "$CONFIG" -d game query "$SQL" --yes >/dev/null 2>&1
  rc=$?
  if [ "$rc" -eq 2 ]; then echo "拦截 exit=2 ✓"; else echo "未拦截 exit=$rc ✗"; fi
done
echo ""

show "【场景4】可写数据源（$DS，allow_write=true）执行 DELETE —— 应放行
  期望：OK (rows affected: N)" \
  "$CLI" -c "$CONFIG" -d "$DS" query "DELETE FROM dbm_demo WHERE id=-999" --yes

echo "【对照】只读数据源（game）执行 SELECT —— 应正常（读不受守卫限制）"
echo "  期望：返回查询结果"
echo "  ----------------------------------------------------------------"
"$CLI" -c "$CONFIG" -d game query "SELECT 1 AS ok" 2>&1 | head -4
echo ""
echo "exit code=$?"
