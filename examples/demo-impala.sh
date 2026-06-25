#!/usr/bin/env bash
# demo-impala.sh —— Apache Impala 命令清单（每行可单独拷贝执行）
# 数据源 impala45：Impala 4.5.0，库 test_db，在 dbm-cli.yaml 中定义。
# 约定：在 examples/ 目录下执行（配置文件 dbm-cli.yaml 与二进制 dbm-cli 均在本目录）。
# 下面每一条都是独立的命令，挑任意一条拷贝即可直接运行。
echo "==================== Impala 演示（数据源: impala45）===================="

# 查数据库版本（Impala 的 version() 返回 impalad 版本描述）
./dbm-cli -d impala45 version

# 列出数据库（SHOW DATABASES）
./dbm-cli -d impala45 databases

# 列出 schema（Impala 下即 database）
./dbm-cli -d impala45 schemas

# 列出表（SHOW TABLES IN test_db）
./dbm-cli -d impala45 tables

# 查看表结构（DESCRIBE；Impala 无 INFORMATION_SCHEMA，故走 SHOW/DESCRIBE）
./dbm-cli -d impala45 columns --table dbm_demo

# 查看表索引（Impala 无传统索引，会返回说明提示用 PARTITIONED/SORTED BY）
./dbm-cli -d impala45 indexes --table dbm_demo

# 查表数据（Impala 仅支持 LIMIT，不支持 OFFSET 跳页）
./dbm-cli -d impala45 table --name dbm_demo --limit 5

# 按分数排序取前 3
./dbm-cli -d impala45 table --name dbm_demo --limit 3 --order score

# 自定义只读查询（按部门聚合统计）
./dbm-cli -d impala45 query "SELECT dept, count(*) AS cnt, round(avg(score),1) AS avg_score FROM dbm_demo GROUP BY dept ORDER BY cnt DESC"

# 按条件筛选（分数 >= 85）
./dbm-cli -d impala45 query "SELECT id, name, score FROM dbm_demo WHERE score >= 85 ORDER BY score DESC"

# JSON 输出（供程序/AI 解析）
./dbm-cli -d impala45 query "SELECT id, name, dept, score FROM dbm_demo ORDER BY id" -o json

# CSV 输出
./dbm-cli -d impala45 query "SELECT id, name FROM dbm_demo ORDER BY id" -o csv

# 注意：Impala 不支持 OFFSET 分页，以下会报清晰错误提示改用 query
# ./dbm-cli -d impala45 table --name dbm_demo --limit 5 --offset 10
