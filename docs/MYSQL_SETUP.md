# 本地 MySQL 安装与项目数据库初始化指南

本文档记录了 `eino-risk-qa` 项目在本机（macOS）搭建 MySQL 开发环境的完整过程，方便下次在新机器上自行安装、或本机环境损坏后重建。

## 环境信息（记录本次安装时的实际环境）

- 操作系统：macOS 15.7.7（`sw_vers` 输出）
- 包管理器：Homebrew 5.1.15，安装前缀 `/usr/local`
- MySQL 版本：`8.0.19`（Homebrew formula `mysql`，实际已存在 `8.0.19_1` keg）
- 数据目录：`/usr/local/var/mysql`
- 客户端/服务端二进制：`/usr/local/bin/mysql`、`/usr/local/bin/mysqld`

> 若在新机器安装，Homebrew 默认会拉取当前最新的 `mysql`（可能是 8.4.x 或 9.x），操作步骤基本一致，仅版本号不同，参见下方"安装最新版注意事项"。

## 一、安装 Homebrew（如已安装可跳过）

```bash
brew --version
```

若未安装，参考 [https://brew.sh](https://brew.sh) 官方脚本安装。

## 二、安装 MySQL

```bash
brew install mysql
```

安装完成后 Homebrew 会打印类似提示（Caveats），需要关注两点：

```
We've installed your MySQL database without a root password. To secure it run:
    mysql_secure_installation

MySQL is configured to only allow connections from localhost by default
```

即：**默认 root 无密码**，且默认只允许本机连接。

## 三、启动 MySQL 服务

使用 `brew services` 管理（推荐，随系统登录自动启动）：

```bash
brew services start mysql
```

查看服务状态：

```bash
brew services list
```

预期输出中 `mysql` 一行的 `Status` 应为 `started`：

```
Name          Status User      File
mysql         started sichwang  ~/Library/LaunchAgents/homebrew.mxcl.mysql.plist
```

如需停止/重启：

```bash
brew services stop mysql
brew services restart mysql
```

也可以不用后台服务方式，临时手动启动（前台阻塞、调试用）：

```bash
mysql.server start
mysql.server stop
```

## 四、验证 root 可无密码连接

```bash
mysql -u root -e "SELECT VERSION();"
```

预期输出版本号（本机为 `8.0.19`）。若失败提示 `Access denied`，说明之前已手动设置过 root 密码，可用 `mysql -u root -p` 输入已知密码尝试，或参考"忘记 root 密码"章节重置。

> 生产环境务必执行 `mysql_secure_installation` 加固（设置 root 密码、移除匿名用户、禁用远程 root 登录等）。本文档面向本机开发调试场景，暂不做该加固，保留 root 无密码便于本机管理，业务连接改用专用账号（见下）。

## 五、为项目创建专用数据库与账号

**不直接使用 root 账号连接业务代码**，而是创建一个专用数据库账号，权限仅限于该项目数据库，更贴近生产实践、也避免误操作其他库。

```bash
mysql -u root -e "
CREATE DATABASE IF NOT EXISTS eino_risk_qa DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
CREATE USER IF NOT EXISTS 'eino_risk_qa'@'localhost' IDENTIFIED BY 'EinoRiskQa#2026';
GRANT ALL PRIVILEGES ON eino_risk_qa.* TO 'eino_risk_qa'@'localhost';
FLUSH PRIVILEGES;
"
```

字段说明：

| 项 | 值 |
| --- | --- |
| 数据库名 | `eino_risk_qa` |
| 字符集/排序规则 | `utf8mb4` / `utf8mb4_0900_ai_ci`（与 `docs/DESIGN.md` 数据层设计一致） |
| 业务账号 | `eino_risk_qa` |
| 业务密码 | `EinoRiskQa#2026` |
| 允许连接的 Host | `localhost`（仅本机） |

> **密码策略提示**：MySQL 默认开启 `validate_password` 组件（`MEDIUM` 策略），要求密码长度 ≥8，且包含大小写字母、数字、特殊字符各至少 1 个。若创建账号时报错 `ERROR 1819 (HY000): Your password does not satisfy the current policy requirements`，需更换符合策略的密码；查看当前策略：
> ```bash
> mysql -u root -e "SHOW VARIABLES LIKE 'validate_password%';"
> ```

验证专用账号连接：

```bash
mysql -u eino_risk_qa -p'EinoRiskQa#2026' -e "SELECT DATABASE(), CURRENT_USER();" eino_risk_qa
```

预期输出：

```
DATABASE()      CURRENT_USER()
eino_risk_qa    eino_risk_qa@localhost
```

## 六、初始化项目表结构

项目的建表 SQL 位于 `migrations/0001_init_schema.up.sql`（内容与 `docs/DESIGN.md` 数据层设计章节的 DDL 保持一致，四张表：`users`/`batches`/`risk_factor_sessions`/`qa_records`，均为 `InnoDB` + `utf8mb4`，不使用外键约束，详见 DESIGN.md）。

执行方式（任选其一）：

```bash
# 方式一：直接用mysql客户端导入
mysql -u eino_risk_qa -p'EinoRiskQa#2026' eino_risk_qa < migrations/0001_init_schema.up.sql

# 方式二：项目引入 golang-migrate 后（详见 DESIGN.md 技术栈选型），用标准迁移命令
# migrate -path migrations -database "mysql://eino_risk_qa:EinoRiskQa%232026@tcp(127.0.0.1:3306)/eino_risk_qa" up
```

回滚（清空表结构，谨慎使用）：

```bash
mysql -u eino_risk_qa -p'EinoRiskQa#2026' eino_risk_qa < migrations/0001_init_schema.down.sql
```

验证建表结果：

```bash
mysql -u eino_risk_qa -p'EinoRiskQa#2026' eino_risk_qa -e "SHOW TABLES;"
```

预期输出：

```
Tables_in_eino_risk_qa
batches
qa_records
risk_factor_sessions
users
```

## 七、项目配置文件中的连接串（供 `configs/config.yaml` / Viper 配置参考）

```yaml
mysql:
  host: 127.0.0.1
  port: 3306
  user: eino_risk_qa
  password: "EinoRiskQa#2026"
  database: eino_risk_qa
  charset: utf8mb4
  parse_time: true
  loc: Local
```

对应 GORM DSN 格式示例：

```
eino_risk_qa:EinoRiskQa#2026@tcp(127.0.0.1:3306)/eino_risk_qa?charset=utf8mb4&parseTime=True&loc=Local
```

> 注意：密码中含 `#` 字符，若用在 URL/DSN 字符串中需做 URL 编码（`#` → `%23`），Go 代码里直接拼接普通 DSN 字符串（非 URL）时通常无需转义，但若使用 `database/sql` 的 DSN 或包含在 URL 中，请留意转义规则。

## 八、常用管理命令速查

```bash
# 查看服务状态
brew services list

# 启动/停止/重启
brew services start mysql
brew services stop mysql
brew services restart mysql

# 命令行登录（业务账号）
mysql -u eino_risk_qa -p eino_risk_qa

# 命令行登录（root，本机管理用）
mysql -u root

# 查看错误日志（路径含本机hostname，按实际情况调整）
tail -f /usr/local/var/mysql/*.err

# 查看数据目录
ls /usr/local/var/mysql
```

## 九、忘记 root 密码时的重置方法

```bash
brew services stop mysql
mysqld_safe --skip-grant-tables &
mysql -u root
```

在 `mysql` 交互中执行：

```sql
FLUSH PRIVILEGES;
ALTER USER 'root'@'localhost' IDENTIFIED BY '新密码';
FLUSH PRIVILEGES;
```

然后：

```bash
mysqladmin -u root -p shutdown   # 关闭 skip-grant-tables 模式启动的实例
brew services start mysql        # 恢复正常启动
```

## 十、卸载（如需完全清理重装）

```bash
brew services stop mysql
brew uninstall mysql
rm -rf /usr/local/var/mysql          # 彻底删除数据目录（会丢失所有数据，谨慎执行）
rm -rf /usr/local/etc/my.cnf         # 删除配置文件（如有自定义）
```

## 十一、安装最新版（非 8.0.19）时的注意事项

若在新机器上直接 `brew install mysql` 拉取的是较新大版本（如 8.4.x 或 9.x），整体流程一致，但需注意：

- 新版本可能默认开启更严格的认证插件或密码策略，创建账号语法不变，但若连接报 `caching_sha2_password` 相关错误，可在 GORM/Go MySQL Driver（`go-sql-driver/mysql`）连接参数中确认已使用支持该认证方式的驱动版本（`go-sql-driver/mysql` v1.6+ 默认已支持）。
- 若本机之前装过 8.0.x 又要升级到 9.x，Homebrew 提示需要先经过 8.4 中转（`brew install mysql@8.4` 启动一次做数据字典升级后再切换到最新版），避免跨大版本直接升级导致数据目录不兼容，具体命令 `brew info mysql` 会打印，按提示操作即可。
- `utf8mb4_0900_ai_ci` 排序规则从 MySQL 8.0 起支持，更高版本兼容，无需调整。
