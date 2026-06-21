#!/usr/bin/env bash
# demo-sqlserver.sh —— Microsoft SQL Server 命令清单（每行可单独拷贝执行）
# 数据源 mssql2022：SQL Server 2022，库 appdb，在 dbm-cli.yaml 中定义。
# 另有 mssql2017（SQL Server 2017，端口 14317）可替换。
# 约定：在 demo/ 目录下执行（配置文件 dbm-cli.yaml 与二进制 dbm-cli 均在本目录）。
echo "==================== SQL Server 演示（数据源: mssql2022）===================="

# 查数据库版本（@@VERSION，含产品年份与引擎版本号）
./dbm-cli -d mssql2022 version

# 列出实例下的数据库
./dbm-cli -d mssql2022 databases

# 列出当前库的 schema（默认 dbo）
./dbm-cli -d mssql2022 schemas

# 列出表（dbo schema）
./dbm-cli -d mssql2022 tables

# 查看表结构（列定义；SQL Server 的类型如 int/datetimeoffset/nvarchar）
./dbm-cli -d mssql2022 columns --table dbm_demo

# 查看表索引（含主键 PK）
./dbm-cli -d mssql2022 indexes --table dbm_demo

# 分页查表数据（OFFSET ... FETCH NEXT，支持跳页）
./dbm-cli -d mssql2022 table --name dbm_demo --limit 5 --order id

# 分页跳页（第 2 页）
./dbm-cli -d mssql2022 table --name dbm_demo --limit 2 --offset 2 --order id

# 自定义只读查询（按部门聚合）
./dbm-cli -d mssql2022 query "SELECT dept, count(*) AS cnt, round(avg(score),1) AS avg_score FROM dbm_demo GROUP BY dept ORDER BY cnt DESC"

# 条件筛选
./dbm-cli -d mssql2022 query "SELECT id, name, score FROM dbm_demo WHERE score >= 85 ORDER BY score DESC"

# JSON 输出（供程序/AI 解析）
./dbm-cli -d mssql2022 query "SELECT id, name, dept, score FROM dbm_demo ORDER BY id" -o json

# CSV 输出
./dbm-cli -d mssql2022 query "SELECT id, name FROM dbm_demo ORDER BY id" -o csv

# 切换到 2017 版本
# ./dbm-cli -d mssql2017 version
# ./dbm-cli -d mssql2017 tables
# ./dbm-cli -d mssql2017 table --name dbm_demo --limit 5
