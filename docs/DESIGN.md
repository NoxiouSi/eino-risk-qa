# 风险要素合理怀疑排除服务 — 技术设计方案

## 用户需求

基于 eino 框架（Go, CloudWeGo）构建一个"风险要素合理怀疑排除"服务，整体遵循领域驱动设计（DDD）。批量提交一个用户名下多个风险要素（如身份、资金来源），每个要素含主问题与用户回答；LLM 对每个要素分别判断回答的**完整性**与**合理性**两个独立维度。若信息不完整，则针对缺失点生成追问，等待用户通过专门接口提交追问回答，每要素最多追问 3 次；一旦完整性满足即结束追问循环（不再因合理性问题继续追问），最终结合两个维度给出"是否排除合理怀疑"的结论、终止原因及提取到的结构化信息。

## 产品概览

面向风控/尽调场景：批量提交一个用户名下多个风险要素（如身份、资金来源），每个要素含主问题与用户回答；LLM 对每个要素分别判断回答的**完整性**（信息是否已全部覆盖）与**合理性**（内容是否可信、无矛盾）。若信息不完整，则针对缺失点生成追问，等待用户通过专门接口提交追问回答，每要素最多追问 3 次；一旦完整性满足即结束追问循环（不再因合理性问题继续追问），最终结合两个维度给出"是否排除合理怀疑"的结论、终止原因及提取到的结构化信息。本次仅提供后端 API 服务，并明确定义标准化的 HTTP 接口契约、数据层设计。

## 核心功能

- 批量提交用户 + 多个风险要素（主问题+回答），各要素独立并发首轮判断
- 单个风险要素追问回答提交接口（按 session 定位，仅 Processing 状态可提交）
- 批次/会话状态与历史问答查询接口
- 完整性驱动追问循环（≤3轮），合理性仅参与终态结论合成、不驱动追问
- 结论合成规则：完整+合理→Cleared；完整+不合理→NotCleared(unreasonable)；达上限仍不完整→NotCleared(max_rounds_incomplete)；不完整且未达上限→继续追问
- 提取信息跨轮次累积合并（同名字段以最新轮次为准）
- 统一的 API 契约：响应包装格式、错误码体系、字段命名规范、鉴权方式
- 全过程持久化，业务状态机作为领域核心知识内聚在领域层，LLM调用与Prompt模板经依赖倒置下沉到基础设施层

## 技术栈

- 语言：Go 1.21+
- LLM框架：CloudWeGo `eino` + `eino-ext`（ChatModel Provider适配，如openai兼容/ark）
- Web框架：Hertz（CloudWeGo同生态，高性能、原生流式支持，errgroup并发调用友好）
- ORM：GORM + golang-migrate（正式迁移脚本）
- 数据库：MySQL 8.x
- 配置：Viper
- 并发：golang.org/x/sync/errgroup

## 架构风格：DDD + 端口与适配器（Hexagonal）

核心原则：**领域层零框架依赖，业务规则（状态机）内聚在领域层；LLM调用与Prompt构建通过依赖倒置下沉为基础设施层，实现领域层定义的端口接口**。

### 分层说明

- **domain（领域层）**：`internal/domain/riskfactor/`。不 import eino、不 import gorm。包含：
  - 聚合根 `RiskFactorSession`：封装状态机全部规则（转移条件、轮次校验、终态判定、提取信息合并），是"核心领域知识"载体，提供领域方法驱动状态迁移，不对外暴露可绕过规则的字段直接赋值。
  - 值对象：`JudgementResult`（Completeness/Reasonableness/FollowUpQuestion/ExtractedInfo/ReasoningSummary）、`QAPair`、`RiskFactorType`、`SessionStatus`、`TerminationReason` 枚举。
  - 端口（domain定义，infra实现）：`RiskJudger`（LLM判断能力抽象）、`SessionRepository`（持久化能力抽象）。
  - 领域事件（可选，供审计）：`SessionCleared`/`SessionNotCleared`/`FollowUpRequested`。
- **application（应用层）**：`internal/application/`。仅编排：加载聚合→调用`RiskJudger`端口获取判断→调用聚合领域方法完成状态迁移→通过`SessionRepository`端口落库，事务边界在此层控制。不包含业务规则本身。含 `BatchAppService`（批量创建+errgroup并发调度首轮判断）、`SessionAppService`（首次提交/追问提交两个用例）。
- **infra（基础设施层）**：
  - `internal/infra/llm/`：ChatModel工厂（按provider分发eino-ext组件）、Prompt/ChatTemplate构建（系统提示词+风险要素类型+历史问答拼装）、结构化输出Tool Schema定义与解析重试、`JudgerAdapter`实现`domain.RiskJudger`。
  - `internal/infra/persistence/`：GORM实体定义（与domain聚合做双向映射，不直接复用domain结构体）、实现`domain.SessionRepository`（含乐观锁+事务）。
- **api（接口层）**：`internal/api/`。Hertz路由、Handler、DTO，调用application层用例，不下沉业务逻辑。
- **config/main**：配置加载 + `cmd/server/main.go` 手动依赖组装（infra实现→注入application→注入api handler），无需额外DI框架。

依赖方向：api → application → domain(端口接口) ← infra(实现端口)。infra依赖domain定义的接口类型，domain完全不依赖infra，实现依赖倒置。

## 判断维度拆分与结论合成（核心业务规则，实现于domain聚合根）

- 每轮LLM判断同时输出 `completeness`（完整性）与`reasonableness`（合理性）两个独立bool，以及`follow_up_question`（针对完整性缺口生成）、`extracted_info`（结构化，跨轮次以字段级合并、同名取最新轮次）。
- 追问循环仅由`completeness`驱动：completeness=false 且 round<3 → 生成追问、round+1、状态仍为Processing；completeness=true → 立即终止追问循环，不再看reasonableness决定是否继续追问。
- 终态结论合成规则（domain聚合根内实现）：
  - completeness=true & reasonableness=true → Cleared（cleared=true）
  - completeness=true & reasonableness=false → NotCleared（cleared=false，reason=unreasonable）
  - completeness=false & round已达3 → NotCleared（cleared=false，reason=max_rounds_incomplete）
  - completeness=false & round<3 → 非终态，继续Processing，携带follow_up_question

## 状态机设计（domain层聚合根RiskFactorSession内实现，非分散在application/api）

状态集合：Processing（含round 0~3）、Cleared（终态）、NotCleared（终态，含reason: unreasonable/max_rounds_incomplete）、LLMError（非终态、可重试、不消耗轮次）。
事件：SubmitInitialAnswer、SubmitFollowUpAnswer、LLM判断返回（携带completeness+reasonableness）、LLM调用/解析失败。

```mermaid
stateDiagram-v2
    [*] --> Processing: SubmitInitialAnswer(round=0)
    Processing --> LLMError: LLM调用/解析失败
    LLMError --> Processing: 重试同一轮(不增加round)
    Processing --> Cleared: completeness=true & reasonableness=true
    Processing --> NotCleared_Unreasonable: completeness=true & reasonableness=false
    Processing --> NotCleared_MaxRounds: completeness=false & round==3
    Processing --> Processing: completeness=false & round<3 (round+=1, 记录follow_up_question, 等待SubmitFollowUpAnswer)
    NotCleared_Unreasonable --> [*]
    NotCleared_MaxRounds --> [*]
    Cleared --> [*]

    state NotCleared_Unreasonable {
      note: cleared=false, reason=unreasonable
    }
    state NotCleared_MaxRounds {
      note: cleared=false, reason=max_rounds_incomplete
    }
```

仅 Processing 状态的session允许提交SubmitFollowUpAnswer；终态session再提交追问请求返回明确业务错误。状态、round、reason、completeness/reasonableness快照等字段在infra/persistence GORM实体中有对应映射，但状态迁移判断逻辑只存在于domain层，infra/application只是读写字段，不做业务判断。

## Implementation Notes

- 日志：不打印用户回答原文全文，仅记录session_id/batch_id/round/status/耗时。
- LLM调用失败/解析失败：进入LLMError，允许同轮重试（不消耗round），最终仍失败需在响应中如实返回错误。
- 批量接口单要素失败不应导致整批失败，返回每要素独立status/error。
- "新增QA记录"与"更新session状态/轮次"须在同一事务内完成（application层控制事务边界，通过SessionRepository端口的事务方法或Unit of Work模式）。
- extracted_info合并逻辑（跨轮次字段级合并）应实现为domain值对象上的方法（如`JudgementResult.MergeInto(existing map)`），保持规则内聚。

## 架构图

```mermaid
graph TB
    Client[客户端/风控业务系统]
    subgraph API层
        R1[POST /api/v1/batches 批量首轮提交]
        R2[POST /api/v1/sessions/id/answers 追问回答提交]
        R3[GET /api/v1/batches/id 批次查询]
        R4[GET /api/v1/sessions/id 会话详情查询]
    end
    subgraph Application层 用例编排
        BAS[BatchAppService 并发调度]
        SAS[SessionAppService 首轮/追问用例, 事务边界]
    end
    subgraph Domain层 核心领域知识
        AGG[RiskFactorSession 聚合根: 状态机+轮次+结论合成规则]
        VO[值对象: JudgementResult/QAPair/SessionStatus]
        P1[[端口: RiskJudger]]
        P2[[端口: SessionRepository]]
    end
    subgraph Infra层 端口实现 依赖倒置
        LLMA[JudgerAdapter: ChatModel工厂+Prompt模板+Tool Schema解析]
        PERSA[GORM Repository: 实体映射+乐观锁+事务]
    end
    DB[(MySQL: users/batches/risk_factor_sessions/qa_records)]

    Client --> R1 --> BAS
    Client --> R2 --> SAS
    Client --> R3 --> SAS
    Client --> R4 --> SAS
    BAS --> SAS
    SAS --> AGG
    AGG --> VO
    SAS -.调用.-> P1
    SAS -.调用.-> P2
    LLMA -.实现.-> P1
    PERSA -.实现.-> P2
    PERSA --> DB
```

## API 接口文档（契约设计）

### 统一规范

- **统一响应包装**：采用"HTTP 状态码表达调用层面的成功/失败 + 业务对象直接作为响应体"的方案（不额外套 code/message/data 包装），理由：本服务是纯后端API、无需兼容遗留统一网关协议；HTTP状态码本身已能表达是否成功（2xx/4xx/5xx），业务对象直接返回可减少一层嵌套解析，前端/调用方按状态码分流处理更直观；批量接口内单项失败通过响应体内每项独立的 `status`/`error` 字段表达，而非依赖外层 code。若后续需接入统一网关规范，可在api层加一层薄包装，不影响application/domain。
- **错误响应统一结构**（4xx/5xx 场景）：

```json
{
  "error_code": "SESSION_NOT_PROCESSING",
  "message": "session当前状态为cleared，不允许提交追问回答",
  "request_id": "req-xxxx"
}
```

- **字段命名规范**：全部使用 snake_case，与Go结构体 `json:"xxx_yyy"` tag 一一对应；时间字段统一 ISO8601 字符串（如 `2026-07-23T10:00:00Z`）；枚举字段使用小写下划线字符串（如 `risk_factor_type: "identity"` / `"fund_source"`；`status: "processing"|"cleared"|"not_cleared"|"llm_error"`；`termination_reason: "unreasonable"|"max_rounds_incomplete"`）。
- **鉴权方式**：当前阶段服务定位为内部风控系统间调用，无需完整用户登录体系，采用简单 API Key 鉴权：请求头 `X-API-Key: <key>`，由 api 层中间件统一校验，未通过返回 `401 UNAUTHORIZED`；后续如需更细粒度权限控制，可在此中间件位置扩展，不影响下层设计。

### 错误码与 HTTP 状态码映射表

| error_code | HTTP状态码 | 场景 |
| --- | --- | --- |
| INVALID_PARAM | 400 | 请求参数校验失败（缺失字段、枚举值非法等） |
| UNAUTHORIZED | 401 | API Key 缺失或无效 |
| SESSION_NOT_FOUND | 404 | session_id 不存在 |
| BATCH_NOT_FOUND | 404 | batch_id 不存在 |
| SESSION_NOT_PROCESSING | 409 | 对非Processing状态的session提交追问回答 |
| LLM_JUDGE_FAILED | 502 | LLM调用/结构化输出解析最终失败（重试后仍失败） |
| INTERNAL_ERROR | 500 | 未预期的内部错误（如DB写入异常） |

### 1. POST /api/v1/batches — 批量首轮提交

请求体：

```json
{
  "user": { "user_id": "u_1001", "name": "张三" },
  "risk_factors": [
    {
      "risk_factor_type": "identity",
      "main_question": "请说明您的身份信息及职业背景",
      "answer": "我是XX公司的财务经理..."
    },
    {
      "risk_factor_type": "fund_source",
      "main_question": "请说明本次资金的来源",
      "answer": "资金来源于..."
    }
  ]
}
```

字段说明：`user.user_id`必填；`risk_factors`数组，每项`risk_factor_type`（枚举：identity/fund_source等，可扩展）、`main_question`、`answer`均必填，为空则返回`INVALID_PARAM`。

响应体（HTTP 200，单要素失败不影响整批）：

```json
{
  "batch_id": "batch_20260723_001",
  "created_at": "2026-07-23T10:00:00Z",
  "results": [
    {
      "session_id": "sess_abc123",
      "risk_factor_type": "identity",
      "status": "processing",
      "current_round": 1,
      "follow_up_question": "您提到的职业背景中，具体的任职时间是？",
      "cleared": null,
      "termination_reason": null,
      "extracted_info": {"occupation": "财务经理"},
      "error": null
    },
    {
      "session_id": "sess_def456",
      "risk_factor_type": "fund_source",
      "status": "cleared",
      "current_round": 0,
      "follow_up_question": null,
      "cleared": true,
      "termination_reason": null,
      "extracted_info": {"source": "工资收入"},
      "error": null
    }
  ]
}
```

说明：`status=llm_error`时`error`字段携带`error_code`+`message`，其余字段可为空；该项失败不影响其他风险要素的正常返回。

### 2. POST /api/v1/sessions/{session_id}/answers — 追问回答提交

路径参数：`session_id`

请求体：

```json
{ "answer": "任职时间为2020年至今" }
```

响应体（HTTP 200）：

```json
{
  "session_id": "sess_abc123",
  "status": "cleared",
  "current_round": 1,
  "follow_up_question": null,
  "cleared": true,
  "termination_reason": null,
  "extracted_info": {"occupation": "财务经理", "tenure": "2020年至今"},
  "error": null
}
```

校验规则：`session_id`不存在 → `404 SESSION_NOT_FOUND`；session非`processing`状态提交 → `409 SESSION_NOT_PROCESSING`；`answer`为空 → `400 INVALID_PARAM`。

### 3. GET /api/v1/batches/{batch_id} — 批次查询

响应体（HTTP 200）：

```json
{
  "batch_id": "batch_20260723_001",
  "created_at": "2026-07-23T10:00:00Z",
  "sessions": [
    {
      "session_id": "sess_abc123",
      "risk_factor_type": "identity",
      "main_question": "请说明您的身份信息及职业背景",
      "status": "cleared",
      "current_round": 1,
      "max_rounds": 3,
      "cleared": true,
      "termination_reason": null,
      "extracted_info": {"occupation": "财务经理", "tenure": "2020年至今"},
      "history": [
        {"round": 0, "question": "请说明您的身份信息及职业背景", "answer": "我是XX公司的财务经理...", "completeness": false, "reasonableness": true},
        {"round": 1, "question": "您提到的职业背景中，具体的任职时间是？", "answer": "任职时间为2020年至今", "completeness": true, "reasonableness": true}
      ]
    }
  ]
}
```

`batch_id`不存在 → `404 BATCH_NOT_FOUND`。

### 4. GET /api/v1/sessions/{session_id} — 会话详情查询

响应体结构同上"sessions"数组中单项，直接返回该session完整详情（含`history`数组）。`session_id`不存在 → `404 SESSION_NOT_FOUND`。

## 数据层设计

### ER 关系图

```mermaid
erDiagram
    users ||--o{ batches : "发起"
    batches ||--o{ risk_factor_sessions : "包含"
    risk_factor_sessions ||--o{ qa_records : "包含"

    users {
        bigint id PK
        varchar user_id "业务唯一键"
        varchar name
        datetime created_at
        datetime updated_at
    }
    batches {
        bigint id PK
        varchar batch_id "业务唯一键"
        varchar user_id "逻辑关联users.user_id"
        datetime created_at
    }
    risk_factor_sessions {
        bigint id PK
        varchar session_id "业务唯一键"
        varchar batch_id "逻辑关联batches.batch_id"
        varchar user_id
        varchar risk_factor_type
        text main_question
        varchar status
        int current_round
        int max_rounds
        tinyint completeness
        tinyint reasonableness
        varchar termination_reason
        tinyint cleared
        json extracted_info
        int version "乐观锁"
        datetime created_at
        datetime updated_at
    }
    qa_records {
        bigint id PK
        varchar session_id "逻辑关联risk_factor_sessions.session_id"
        int round
        text question
        text answer
        tinyint completeness
        tinyint reasonableness
        json extracted_info_delta
        datetime created_at
    }
```

### 外键约束设计决策

**不使用数据库外键约束**，仅通过业务标识字段（`user_id`/`batch_id`/`session_id`，均为应用层生成的字符串ID，如UUID或自定义前缀+时间戳）+ 索引维护引用完整性，理由：

- 避免分布式/多实例部署下因外键约束产生的锁竞争，批量接口内并发写多个 `risk_factor_sessions`/`qa_records` 时可获得更好的写入吞吐；
- 便于后续分库分表、数据归档迁移，不受外键强约束限制；
- 引用完整性由 application 层（事务内先写父表再写子表）与领域规则保证，数据库侧通过索引保障查询性能。

### 字符集与引擎

统一使用 `utf8mb4` 字符集（支持中文及emoji）、`InnoDB` 引擎（支持事务，与"QA记录写入+session状态更新同事务"的设计吻合）。

### 建表 DDL（`migrations/0001_init_schema.up.sql`）

执行顺序：`users` → `batches` → `risk_factor_sessions` → `qa_records`（无外键约束，此顺序仅为可读性考虑，非强制依赖）。

```sql
CREATE TABLE `users` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id` VARCHAR(64) NOT NULL COMMENT '业务用户唯一标识',
  `name` VARCHAR(128) DEFAULT NULL COMMENT '用户姓名',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='用户表';

CREATE TABLE `batches` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `batch_id` VARCHAR(64) NOT NULL COMMENT '业务批次唯一标识',
  `user_id` VARCHAR(64) NOT NULL COMMENT '逻辑关联users.user_id，不设外键',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_batch_id` (`batch_id`),
  KEY `idx_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='批次表';

CREATE TABLE `risk_factor_sessions` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `session_id` VARCHAR(64) NOT NULL COMMENT '业务会话唯一标识',
  `batch_id` VARCHAR(64) NOT NULL COMMENT '逻辑关联batches.batch_id，不设外键',
  `user_id` VARCHAR(64) NOT NULL COMMENT '便于按用户查询',
  `risk_factor_type` VARCHAR(32) NOT NULL COMMENT '风险要素类型：identity/fund_source等',
  `main_question` TEXT NOT NULL COMMENT '主问题内容',
  `status` VARCHAR(32) NOT NULL DEFAULT 'processing' COMMENT 'processing/cleared/not_cleared/llm_error',
  `current_round` INT NOT NULL DEFAULT 0 COMMENT '当前已完成轮次，0~3',
  `max_rounds` INT NOT NULL DEFAULT 3 COMMENT '最大追问轮次',
  `completeness` TINYINT(1) DEFAULT NULL COMMENT '最新一轮完整性判断快照',
  `reasonableness` TINYINT(1) DEFAULT NULL COMMENT '最新一轮合理性判断快照',
  `termination_reason` VARCHAR(32) DEFAULT NULL COMMENT 'unreasonable/max_rounds_incomplete，终态才有值',
  `cleared` TINYINT(1) DEFAULT NULL COMMENT '是否排除合理怀疑，终态才有值',
  `extracted_info` JSON DEFAULT NULL COMMENT '跨轮次累积合并后的结构化提取信息',
  `version` INT NOT NULL DEFAULT 0 COMMENT '乐观锁版本号',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_session_id` (`session_id`),
  KEY `idx_batch_id` (`batch_id`),
  KEY `idx_user_id` (`user_id`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='风险要素会话表';

CREATE TABLE `qa_records` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `session_id` VARCHAR(64) NOT NULL COMMENT '逻辑关联risk_factor_sessions.session_id，不设外键',
  `round` INT NOT NULL COMMENT '该轮次序号，0为主问题，1~3为追问',
  `question` TEXT NOT NULL COMMENT '本轮问题内容（主问题或追问）',
  `answer` TEXT NOT NULL COMMENT '用户本轮回答',
  `completeness` TINYINT(1) DEFAULT NULL COMMENT '本轮完整性判断快照',
  `reasonableness` TINYINT(1) DEFAULT NULL COMMENT '本轮合理性判断快照',
  `extracted_info_delta` JSON DEFAULT NULL COMMENT '本轮新提取到的增量信息，用于审计追溯',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_session_round` (`session_id`, `round`),
  KEY `idx_session_id` (`session_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='问答记录表';
```

### 回滚脚本（`migrations/0001_init_schema.down.sql`）

按依赖反序删除：

```sql
DROP TABLE IF EXISTS `qa_records`;
DROP TABLE IF EXISTS `risk_factor_sessions`;
DROP TABLE IF EXISTS `batches`;
DROP TABLE IF EXISTS `users`;
```

## Directory Structure Summary

全新Go项目，按DDD分层组织：domain（领域规则）、application（用例编排）、infra（LLM与持久化适配器，依赖倒置实现domain端口）、api（Hertz接口层，按上述接口文档实现）、config、迁移脚本。

```
eino-risk-qa/
├── cmd/
│   └── server/
│       └── main.go                          # 依赖组装：加载配置→构造infra实现(JudgerAdapter/GORMSessionRepository)→注入application service→注入api handler→注册路由(含API Key中间件)→启动Hertz
├── internal/
│   ├── domain/
│   │   └── riskfactor/
│   │       ├── session.go                   # 聚合根RiskFactorSession：状态机全部规则(SubmitInitialAnswer/SubmitFollowUpAnswer方法)，round校验，终态判定与结论合成(Cleared/NotCleared+reason)
│   │       ├── judgement.go                 # 值对象JudgementResult(Completeness/Reasonableness/FollowUpQuestion/ExtractedInfo/ReasoningSummary)及ExtractedInfo跨轮次合并方法MergeInto
│   │       ├── qa_pair.go                   # 值对象QAPair(问题/回答/所属轮次/completeness/reasonableness判断快照)
│   │       ├── types.go                     # SessionStatus、TerminationReason、RiskFactorType枚举定义
│   │       ├── events.go                    # 领域事件定义(SessionCleared/SessionNotCleared/FollowUpRequested)，用于审计扩展
│   │       └── ports.go                     # 端口接口定义：RiskJudger(Judge方法)、SessionRepository(Save/FindByID/事务方法)，domain层核心抽象，infra实现，application依赖注入使用
│   ├── application/
│   │   ├── batch_app_service.go             # BatchAppService：创建batch，errgroup并发调度各风险要素调用SessionAppService.SubmitInitial
│   │   └── session_app_service.go           # SessionAppService：SubmitInitial/SubmitFollowUp两个用例，加载聚合→调用RiskJudger端口→调用聚合领域方法→SessionRepository端口持久化，事务边界控制
│   ├── infra/
│   │   ├── llm/
│   │   │   ├── factory.go                   # ChatModel工厂：按config.Provider(openai/ark)分发eino-ext组件，返回ToolCallingChatModel接口
│   │   │   ├── prompt.go                    # Prompt/ChatTemplate构建：系统提示词、风险要素类型、历史问答拼装、完整性/合理性判断指引
│   │   │   ├── schema.go                    # 结构化输出Tool Schema定义(completeness/reasonableness/follow_up_question/extracted_info/reasoning_summary)
│   │   │   └── judger_adapter.go            # JudgerAdapter实现domain.RiskJudger端口：组合factory+prompt+schema，含重试与解析失败处理
│   │   └── persistence/
│   │       ├── models.go                    # GORM实体：UserModel/BatchModel/RiskFactorSessionModel(含completeness/reasonableness快照、reason、round、version乐观锁字段)/QARecordModel
│   │       ├── session_repository.go        # GORMSessionRepository实现domain.SessionRepository：Save(事务内同时写session状态与QA记录)、FindByID，乐观锁条件更新
│   │       └── mapper.go                    # domain聚合 <-> GORM实体的双向映射转换函数，避免domain直接依赖gorm tag
│   ├── api/
│   │   ├── router.go                        # Hertz路由注册：/api/v1/batches、/api/v1/sessions/:id/answers、/api/v1/batches/:id、/api/v1/sessions/:id，挂载API Key鉴权中间件
│   │   ├── middleware/
│   │   │   └── auth.go                      # API Key校验中间件，读取X-API-Key头，未通过返回401 UNAUTHORIZED
│   │   ├── handler/
│   │   │   ├── batch_handler.go             # 批量提交/批次查询接口处理，调用BatchAppService，按文档组装results数组响应
│   │   │   └── session_handler.go           # 追问提交/会话查询接口处理，调用SessionAppService，统一错误码转换(404/409/400/502/500)
│   │   └── dto/
│   │       ├── batch_dto.go                 # 批量提交请求/响应结构体(BatchRequest/BatchResponse/SessionResult)，json tag对应接口文档字段
│   │       └── session_dto.go               # 追问提交请求/响应、查询响应结构体(SessionDetailResponse含history)，含completeness/reasonableness/reason字段
│   └── config/
│       └── config.go                        # Viper配置：Server/MySQL DSN/LLM Provider鉴权/最大追问轮次/API Key值
├── migrations/
│   ├── 0001_init_schema.up.sql              # 建表：users/batches/risk_factor_sessions(含completeness/reasonableness/reason/round/version字段)/qa_records
│   └── 0001_init_schema.down.sql            # 回滚脚本
├── configs/
│   └── config.yaml                          # 默认配置样例(含api_key占位)
├── go.mod / go.sum                          # 依赖：eino、eino-ext、hertz、gorm、mysql driver、viper、errgroup
└── README.md                                # 项目说明、DDD分层说明、完整API接口文档(含请求/响应示例、错误码表)、启动方式
```

## Key Code Structures

```go
// internal/domain/riskfactor/ports.go — 依赖倒置的核心端口，domain定义、infra实现
type RiskJudger interface {
    Judge(ctx context.Context, riskFactorType RiskFactorType, mainQuestion string, history []QAPair, latestAnswer string) (*JudgementResult, error)
}

type SessionRepository interface {
    Save(ctx context.Context, session *RiskFactorSession) error // 事务内同时持久化session状态与新增QA记录
    FindByID(ctx context.Context, sessionID string) (*RiskFactorSession, error)
}
```

```go
// internal/domain/riskfactor/judgement.go — 判断结果值对象，完整性驱动追问、合理性参与结论合成
type JudgementResult struct {
    Completeness     bool
    Reasonableness   bool
    FollowUpQuestion string                 // 仅Completeness=false时有效，针对完整性缺口生成
    ExtractedInfo    map[string]interface{}
    ReasoningSummary string
}

// MergeInto 将本轮提取信息与历史累积信息合并，同名字段以最新轮次为准
func (j *JudgementResult) MergeInto(existing map[string]interface{}) map[string]interface{}
```

```go
// internal/domain/riskfactor/session.go — 聚合根，状态机核心领域方法签名
type RiskFactorSession struct {
    ID, BatchID, UserID string
    RiskFactorType       RiskFactorType
    MainQuestion         string
    Status                SessionStatus // Processing | Cleared | NotCleared | LLMError
    CurrentRound          int           // 0~3
    MaxRounds             int           // 默认3
    TerminationReason     *TerminationReason // unreasonable | max_rounds_incomplete
    ExtractedInfo         map[string]interface{}
    History               []QAPair
}

// SubmitInitialAnswer 首轮主问题回答提交，内部调用judgement驱动状态迁移
func (s *RiskFactorSession) SubmitInitialAnswer(answer string, judgement *JudgementResult) error
// SubmitFollowUpAnswer 追问回答提交，仅Processing状态允许调用，内部执行完整性驱动的追问循环与结论合成规则
func (s *RiskFactorSession) SubmitFollowUpAnswer(answer string, judgement *JudgementResult) error
```
