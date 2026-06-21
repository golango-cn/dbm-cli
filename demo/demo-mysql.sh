#!/usr/bin/env bash
# demo-mysql.sh —— MySQL 命令清单（每行可单独拷贝执行）
# 数据源 mysql837：MySQL 8.0.37，库 appdb，在 dbm-cli.yaml 中定义。
# 约定：在 demo/ 目录下执行（配置文件 dbm-cli.yaml 与二进制 dbm-cli 均在本目录）。
# 下面每一条都是独立的命令，挑任意一条拷贝即可直接运行。
echo "==================== MySQL 演示（数据源: mysql837）===================="

# 查数据库版本
./dbm-cli -d mysql837 version

# 列出数据库
./dbm-cli -d mysql837 databases

# 列出 schema（MySQL 下即库）
./dbm-cli -d mysql837 schemas

# 列出表
./dbm-cli -d mysql837 tables

# 表名模糊匹配 dbm%
./dbm-cli -d mysql837 tables --like "dbm%"

# 查看表结构（列定义）
./dbm-cli -d mysql837 columns --table dbm_demo

# 查看表索引
./dbm-cli -d mysql837 indexes --table dbm_demo

# 列出视图
./dbm-cli -d mysql837 views

# 分页查表数据（前 5 行，按 id 排序）
./dbm-cli -d mysql837 table --name dbm_demo --limit 5 --order id

# 自定义只读查询（按部门聚合）
./dbm-cli -d mysql837 query "SELECT dept, count(*) AS cnt, round(avg(score),1) AS avg_score FROM dbm_demo GROUP BY dept ORDER BY cnt DESC"

# 参数绑定查询（? 占位符 + --param，防注入）
./dbm-cli -d mysql837 query "SELECT * FROM dbm_demo WHERE dept=? AND score>=?" --param Engineering --param 85

# 输出格式 json
./dbm-cli -d mysql837 table --name dbm_demo --limit 2 --order id -o json

# 输出格式 csv
./dbm-cli -d mysql837 table --name dbm_demo --limit 2 --order id -o csv

echo "==================== MySQL 演示结束 ===================="
