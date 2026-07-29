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
  `follow_up_question` TEXT DEFAULT NULL COMMENT '当前待回答的追问问题文本，Processing状态下有效，用于跨HTTP请求保持UserMessage()推导所需状态',
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
