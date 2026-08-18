# dbm-cli 连接 impala12 失败问题诊断报告

> 供优化 dbm-cli MCP 的 agent 使用。本文档详述问题现象、复现步骤、根因分析与修复建议。
>
> **脱敏说明**：文中 `<IMPALA_HOST>` / `<LDAP_USER>` / `<LDAP_PASSWORD>` 为占位符,原文的 IP、用户名、密码已移除。
>
> **状态更新（2026-08-18）**:该问题已修复——dbm-cli 新增 `auth_mech: LDAP` 配置(对应 Cloudera AuthMech=3),详见仓库 README「Impala 认证方式」一节。

## 一、环境信息

| 项 | 值 |
|---|---|
| dbm-cli 路径 | `/app/bin/dbm-cli` |
| dbm-cli 版本 | `dev`(manifest version 字段) |
| 配置文件 | `/root/.dbm-cli.yaml`(软链到 `/app/.dbm-cli.yaml`) |
| 目标数据源 | impala12(平台内部数据源) |
| impala host:port | `<IMPALA_HOST>:21050` |
| 平台 authType | DEFAULT(authCreden 脱敏为 ***) |
| 平台 serviceName | `ods;AuthMech=3;EXEC_TIME_LIMIT_S=0` |

## 二、问题现象

dbm-cli 连接 impala12 数据源时,任何命令(version / tables / query)均报错:

```
[dbm-cli] error: cannot connect to datasource "<IMPALA_HOST>_impala_ods_prod":
impala: USE ods: driver: bad connection
```

- 报错来自 impala-go 驱动层,发生在 `USE ods` 这一步(即连接建立后切换 database 时失败)。
- 同一配置下,ClickHouse 和 Oracle 数据源可正常连接(本次未实测,但配置结构相同)。

## 三、复现步骤

```bash
# 1. 确认数据源已配置
dbm-cli datasources   # 能看到 <IMPALA_HOST>_impala_ods_prod

# 2. 任意命令触发报错
dbm-cli version --datasource <IMPALA_HOST>_impala_ods_prod
# 期望:返回 impala 版本
# 实际:impala: USE ods: driver: bad connection

dbm-cli tables --datasource <IMPALA_HOST>_impala_ods_prod
# 同样报错
```

## 四、诊断过程与根因

### 4.1 排除网络问题

```bash
# TCP 端口可达
timeout 5 bash -c 'cat < /dev/null > /dev/tcp/<IMPALA_HOST>/21050' && echo "TCP 21050 可达"
# 输出:TCP 21050 可达
```

**TCP 21050 可达**,排除网络/防火墙问题。

### 4.2 平台 impala12 真实连接参数

通过 `dp-cli datasource search` 拿到的 impala12 完整参数:

```json
{
  "sourceName": "impala12",
  "sourceType": "impala",
  "host": "<IMPALA_HOST>",
  "port": "21050",
  "sourceUrl": "jdbc:impala://<IMPALA_HOST>:21050/ods;AuthMech=3;EXEC_TIME_LIMIT_S=0",
  "parameter": {
    "authType": "DEFAULT",
    "connMode": "JDBC",
    "connParams": {},
    "host": "<IMPALA_HOST>",
    "port": "21050",
    "serviceName": "ods;AuthMech=3;EXEC_TIME_LIMIT_S=0"
  }
}
```

**关键矛盾点**:
- 平台 JDBC URL: `jdbc:impala://<IMPALA_HOST>:21050/ods;AuthMech=3;EXEC_TIME_LIMIT_S=0`
- 平台 `authType=DEFAULT`,但 serviceName 里嵌了 `AuthMech=3`(LDAP)
- 平台 **没有配置 user/password**(authCreden 被脱敏为 ***)

### 4.3 dbm-cli 现有 impala 配置

```yaml
<IMPALA_HOST>_impala_ods_prod:
  type: impala
  host: <IMPALA_HOST>
  port: 21050
  database: ods
  user: <LDAP_USER>        # ← 平台未提供,疑似凭空填写
  password: <LDAP_PASSWORD>        # ← 平台未提供,疑似凭空填写
  allow_write: false
  timeout: 15s
```

### 4.4 dbm-cli config_schema 缺失 impala 认证字段

通过 `dbm-cli manifest` 拿到的 `config_schema` **完全没有 impala 专用的认证字段**:

```
datasources.<name>.allow_write
datasources.<name>.description
datasources.<name>.fetch_size
datasources.<name>.force_version
datasources.<name>.host
datasources.<name>.max_open_conns
datasources.<name>.password
datasources.<name>.port
datasources.<name>.service_name    # ← Oracle 专用
datasources.<name>.sid             # ← Oracle 专用
datasources.<name>.timeout
datasources.<name>.type
datasources.<name>.user
```

**没有** `auth_mech` / `use_ldap` / `use_ssl` / `http_path` / `transport` 等 impala/HiveServer2 认证相关字段。

### 4.5 二进制中存在的认证能力

`strings /app/bin/dbm-cli` 显示驱动**内部确实支持**多种认证:

```
UseLDAP / UseKerberos / authMechanism / AuthMechanisms
sasl / sasl.plain / sasl.Client / SASLContinue
LDAP / KERBEROS / NOSASL / PLAIN
flow_plain_allowed / block_plain_allowed
```

**说明驱动代码里有 LDAP/Kerberos/SASL 能力,但 config_schema 没有暴露配置入口** —— 这是不一致点。

## 五、根因结论

报错 `impala: USE ods: driver: bad connection` 发生在连接建立后执行 `USE ods` 时。结合以下证据:

1. **TCP 可达** → 不是网络问题
2. **平台 impala12 要求 `AuthMech=3`(LDAP 认证)**,JDBC URL 明确写了 `AuthMech=3`
3. **dbm-cli config_schema 没有 `auth_mech` 字段** → 无法配置 LDAP 认证
4. **现有配置 `user: <LDAP_USER> / password: <LDAP_PASSWORD>` 是凭空填的**,平台 authType=DEFAULT 但 serviceName 嵌了 AuthMech=3
5. **驱动二进制里有 `UseLDAP`/`authMechanism` 等字符串** → 驱动代码支持,但配置层未暴露

**最可能的根因**:dbm-cli 的 impala 驱动默认用了 NOSASL 或 PLAIN 认证,而 impala12 服务端要求 LDAP(AuthMech=3),认证握手失败导致 `USE ods` 时连接被服务端断开,报 `bad connection`。

## 六、修复建议

### 建议 1:在 config_schema 暴露 impala 认证字段(优先)

在 `config_schema` 增加 impala 专用字段,让用户能配置认证方式:

```yaml
<IMPALA_HOST>_impala_ods_prod:
  type: impala
  host: <IMPALA_HOST>
  port: 21050
  database: ods
  auth_mech: LDAP        # ← 新增:NOSASL / PLAIN / LDAP / KERBEROS
  user: <LDAP_USER>
  password: <LDAP_PASSWORD>
  use_ssl: false         # ← 新增
  http_path: ""          # ← 新增(可选)
```

### 建议 2:测试无认证连接

如果 impala12 实际是 `AuthMech=0`(NOSASL),则应**移除 user/password**,让驱动走 NOSASL:

```yaml
<IMPALA_HOST>_impala_ods_prod:
  type: impala
  host: <IMPALA_HOST>
  port: 21050
  database: ods
  # 不填 user/password,走 NOSASL
```

### 建议 3:增加调试日志

`dbm-cli` 连接失败时只报 `bad connection`,无法定位是认证还是网络问题。建议在驱动层增加 `--debug` 或环境变量(如 `DBM_DEBUG=1`)输出握手细节,包括:
- 实际使用的 auth mechanism
- 服务端返回的 auth mechanisms
- SASL 握手错误

## 七、给优化 agent 的验证清单

修复后请按以下顺序验证:

```bash
# 1. 无认证配置(走 NOSASL)
dbm-cli version --datasource <IMPALA_HOST>_impala_ods_prod

# 2. LDAP 认证配置(若 config_schema 已支持 auth_mech)
dbm-cli version --datasource <IMPALA_HOST>_impala_ods_prod

# 3. 通用查询
dbm-cli tables --datasource <IMPALA_HOST>_impala_ods_prod
dbm-cli query --datasource <IMPALA_HOST>_impala_ods_prod "SELECT 1"
```

**预期**:命令成功返回,不再报 `bad connection`。

## 八、参考:平台 impala12 完整连接参数

| 字段 | 平台值 | dbm-cli 现有配置 | 一致性 |
|------|--------|------------------|--------|
| host | <IMPALA_HOST> | <IMPALA_HOST> | ✅ |
| port | 21050 | 21050 | ✅ |
| database | ods | ods | ✅ |
| authType | DEFAULT | — | ❌ 未配置 |
| serviceName | `ods;AuthMech=3;EXEC_TIME_LIMIT_S=0` | — | ❌ 未配置 |
| authCreden | ***(脱敏) | admin | ❌ 凭空填写 |
| AuthMech | 3 (LDAP) | 未配置 | ❌ |

---

**结论**:dbm-cli 的 impala 驱动**未正确配置 LDAP 认证**(AuthMech=3),导致连接 impala12 时认证握手失败。需要在 config_schema 暴露 `auth_mech` 字段,或修复默认认证逻辑。
