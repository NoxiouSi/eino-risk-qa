-- 新增：users 表增加 risk_factor_types 列（逗号分隔字符串，记录该用户拥有哪些风险要素类型，
-- 取值与 risk_factor_type 枚举一致，如 "identity,fund_source"），供 GET /api/v1/users/{user_id}/main-questions
-- 查询该用户应回答的风险项列表。默认空字符串，不影响现有 EnsureUser 幂等 upsert 逻辑。
ALTER TABLE `users`
  ADD COLUMN `risk_factor_types` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '该用户拥有的风险要素类型列表，逗号分隔，如 identity,fund_source' AFTER `name`;

-- 新增：风险要素类型 -> 主问题 的全局固定映射表。所有用户共用同一套主问题文案，
-- 不按用户区分（与 users.risk_factor_types 配合，决定"该用户需要回答哪些主问题"）。
CREATE TABLE `risk_factor_main_questions` (
  `id`               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `risk_factor_type` VARCHAR(32) NOT NULL COMMENT '风险要素类型标识，如 identity/fund_source',
  `main_question`    TEXT NOT NULL COMMENT '该风险要素类型对应的主问题内容',
  `created_at`       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_risk_factor_type` (`risk_factor_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='风险要素类型-主问题映射表';

-- Seed：identity/fund_source 两个现有风险要素类型的默认主问题文案（与此前前端调试页默认值保持一致）。
INSERT INTO `risk_factor_main_questions` (`risk_factor_type`, `main_question`) VALUES
  ('identity', '请说明您的身份信息及职业背景'),
  ('fund_source', '请说明本次资金的来源')
ON DUPLICATE KEY UPDATE `main_question` = VALUES(`main_question`);

-- Seed：本机联调用的调试用户，拥有 identity + fund_source 两个风险项，避免每次都要手工插库。
INSERT INTO `users` (`user_id`, `name`, `risk_factor_types`) VALUES
  ('u_1001', '张三', 'identity,fund_source')
ON DUPLICATE KEY UPDATE `risk_factor_types` = 'identity,fund_source';
