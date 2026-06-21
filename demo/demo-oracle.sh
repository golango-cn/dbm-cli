#!/usr/bin/env bash
# demo-oracle.sh —— Oracle 命令清单（每行可单独拷贝执行）
# 数据源 oracle18c：Oracle 18c XE，service XEPDB1，用户 APPUSER，在 dbm-cli.yaml 中定义。
# 约定：在 demo/ 目录下执行（配置文件 dbm-cli.yaml 与二进制 dbm-cli 均在本目录）。
# 注意：Oracle 元数据按 schema 隔离，表名通常为大写，故用大写 DBM_DEMO / APPUSER。
# 下面每一条都是独立的命令，挑任意一条拷贝即可直接运行。
echo "==================== Oracle 演示（数据源: oracle18c）===================="

# 查数据库版本
./dbm-cli -d oracle18c version

# 列出库/PDB（CDB 架构下返回各 PDB）
./dbm-cli -d oracle18c databases

# 列出 schema(user)
./dbm-cli -d oracle18c schemas

# 列出当前用户表
./dbm-cli -d oracle18c tables

# 指定 schema 列出表（APPUSER）
./dbm-cli -d oracle18c tables --schema APPUSER

# 表名模糊匹配 DBM%
./dbm-cli -d oracle18c tables --like "DBM%"

# 查看表结构（列定义）
./dbm-cli -d oracle18c columns --table DBM_DEMO

# 查看表索引
./dbm-cli -d oracle18c indexes --table DBM_DEMO

# 列出视图
./dbm-cli -d oracle18c views

# 分页查表数据（前 5 行，按 id 排序）
./dbm-cli -d oracle18c table --name DBM_DEMO --limit 5 --order id

# 自定义只读查询（按部门聚合）
./dbm-cli -d oracle18c query "SELECT dept, count(*) AS cnt, round(avg(score),1) AS avg_score FROM dbm_demo GROUP BY dept ORDER BY cnt DESC"

# 参数绑定查询（? 占位符，dbm-cli 自动转 Oracle 的 :1）
./dbm-cli -d oracle18c query "SELECT * FROM dbm_demo WHERE dept=? AND score>=?" --param Engineering --param 85

# 输出格式 json
./dbm-cli -d oracle18c table --name DBM_DEMO --limit 2 --order id -o json

echo "==================== Oracle 演示结束 ===================="
