SET NAMES utf8mb4;

CREATE TABLE `risk_factor_questions` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `risk_factor_type` VARCHAR(32) NOT NULL COMMENT '风险要素类型',
  `question_key` VARCHAR(64) NOT NULL COMMENT '风险要素内稳定唯一标识',
  `parent_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '父问题ID，业务关联不设外键',
  `question_text` TEXT NOT NULL COMMENT '问题标题或提问文案',
  `answer_type` VARCHAR(16) NOT NULL COMMENT 'group/text/image/file',
  `required` TINYINT(1) NOT NULL DEFAULT 1,
  `min_submit_count` INT UNSIGNED NOT NULL DEFAULT 1,
  `sort_order` INT NOT NULL DEFAULT 0,
  `enabled` TINYINT(1) NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_risk_factor_question_key` (`risk_factor_type`, `question_key`),
  KEY `idx_risk_factor_question_tree` (`risk_factor_type`, `parent_id`, `enabled`, `sort_order`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='统一风险要素问题配置表';

CREATE TABLE `audit_skills` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `skill_key` VARCHAR(64) NOT NULL,
  `name` VARCHAR(128) NOT NULL,
  `rule_text` TEXT NOT NULL,
  `evidence_type` VARCHAR(16) NOT NULL COMMENT 'text/image/file/any',
  `enabled` TINYINT(1) NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_audit_skill_key` (`skill_key`),
  KEY `idx_audit_skill_enabled` (`enabled`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='可复用审核Skill';

CREATE TABLE `question_skill_refs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `question_id` BIGINT UNSIGNED NOT NULL COMMENT '逻辑关联risk_factor_questions.id',
  `skill_id` BIGINT UNSIGNED NOT NULL COMMENT '逻辑关联audit_skills.id',
  `sort_order` INT NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_question_skill` (`question_id`, `skill_id`),
  KEY `idx_question_skill_order` (`question_id`, `sort_order`),
  KEY `idx_question_skill_skill` (`skill_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='问题与审核Skill多对多引用';

CREATE TABLE `uploaded_files` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `file_id` VARCHAR(64) NOT NULL,
  `user_id` VARCHAR(64) NOT NULL,
  `risk_factor_type` VARCHAR(32) NOT NULL,
  `question_key` VARCHAR(64) NOT NULL,
  `original_name` VARCHAR(255) NOT NULL,
  `stored_path` VARCHAR(512) NOT NULL COMMENT '受控相对路径',
  `mime_type` VARCHAR(128) NOT NULL,
  `size_bytes` BIGINT UNSIGNED NOT NULL,
  `sha256` CHAR(64) NOT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_uploaded_file_id` (`file_id`),
  KEY `idx_uploaded_file_owner` (`user_id`, `risk_factor_type`, `question_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='上传文件元数据';

CREATE TABLE `question_submissions` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `submission_id` VARCHAR(64) NOT NULL,
  `session_id` VARCHAR(64) NOT NULL,
  `round` INT NOT NULL,
  `risk_factor_type` VARCHAR(32) NOT NULL,
  `question_key` VARCHAR(64) NOT NULL,
  `value_type` VARCHAR(16) NOT NULL COMMENT 'text/image/file',
  `text_value` TEXT DEFAULT NULL,
  `file_id` VARCHAR(64) DEFAULT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_question_submission_id` (`submission_id`),
  KEY `idx_question_submission_round` (`session_id`, `round`, `question_key`),
  KEY `idx_question_submission_file` (`file_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='逐问题结构化提交记录';

ALTER TABLE `qa_records`
  ADD COLUMN `question_judgements` JSON DEFAULT NULL COMMENT '本轮逐问题判断快照' AFTER `reasonableness`;

INSERT INTO `risk_factor_questions`
  (`risk_factor_type`, `question_key`, `parent_id`, `question_text`, `answer_type`, `required`, `min_submit_count`, `sort_order`, `enabled`)
SELECT `risk_factor_type`, CONCAT(`risk_factor_type`, '_main'), NULL, `main_question`, 'group', 1, 0, 0, 1
FROM `risk_factor_main_questions`;

INSERT INTO `risk_factor_questions`
  (`risk_factor_type`, `question_key`, `parent_id`, `question_text`, `answer_type`, `required`, `min_submit_count`, `sort_order`, `enabled`)
VALUES ('transaction_scene', 'transaction_scene_main', NULL, '请提交能够证明本次交易场景真实存在的信息和材料', 'group', 1, 0, 0, 1);

INSERT INTO `risk_factor_questions`
  (`risk_factor_type`, `question_key`, `parent_id`, `question_text`, `answer_type`, `required`, `min_submit_count`, `sort_order`, `enabled`)
SELECT 'identity', 'real_name', id, '姓名', 'text', 1, 1, 10, 1 FROM `risk_factor_questions` WHERE `risk_factor_type`='identity' AND `answer_type`='group'
UNION ALL
SELECT 'identity', 'id_card_number', id, '身份证号', 'text', 1, 1, 20, 1 FROM `risk_factor_questions` WHERE `risk_factor_type`='identity' AND `answer_type`='group'
UNION ALL
SELECT 'identity', 'id_card_image', id, '身份证图片', 'image', 1, 1, 30, 1 FROM `risk_factor_questions` WHERE `risk_factor_type`='identity' AND `answer_type`='group'
UNION ALL
SELECT 'fund_source', 'fund_source_description', id, '资金来源说明', 'text', 1, 1, 10, 1 FROM `risk_factor_questions` WHERE `risk_factor_type`='fund_source' AND `answer_type`='group'
UNION ALL
SELECT 'fund_source', 'fund_source_evidence', id, '银行流水、收入证明等资金来源证明材料', 'image', 1, 1, 20, 1 FROM `risk_factor_questions` WHERE `risk_factor_type`='fund_source' AND `answer_type`='group'
UNION ALL
SELECT 'transaction_scene', 'transaction_description', id, '交易内容说明', 'text', 1, 1, 10, 1 FROM `risk_factor_questions` WHERE `risk_factor_type`='transaction_scene' AND `answer_type`='group'
UNION ALL
SELECT 'transaction_scene', 'transaction_evidence', id, '订单截图、商户门头、商品照片或截图等交易证明材料', 'image', 1, 1, 20, 1 FROM `risk_factor_questions` WHERE `risk_factor_type`='transaction_scene' AND `answer_type`='group';

INSERT INTO `audit_skills` (`skill_key`, `name`, `rule_text`, `evidence_type`) VALUES
  ('real_name_validation', '真实姓名校验', '姓名应为清晰、完整的真实姓名，不得包含明显占位符、无意义字符或自相矛盾内容。', 'text'),
  ('cn_id_card_validation', '中国身份证号校验', '身份证号应为合法的18位中国居民身份证号码，格式、出生日期和校验位应有效。', 'text'),
  ('fund_source_description_validation', '资金来源说明校验', '资金来源说明应明确具体来源、取得方式及与本次资金的关系，不得仅使用含糊或无法核实的描述。', 'text'),
  ('transaction_description_validation', '交易说明校验', '交易说明应包含交易对象、内容、用途等足以识别真实交易场景的信息，且前后无明显矛盾。', 'text'),
  ('id_card_image_recognition', '身份证图片识别', '图片必须能够识别为真实身份证件照片，证件主体、姓名和证件号码区域应清晰完整。', 'image'),
  ('fund_source_document_recognition', '资金来源材料识别', '图片必须能够识别为银行流水、收入证明或其他可核实的资金来源证明材料。', 'image'),
  ('transaction_evidence_recognition', '交易材料识别', '图片必须能够识别为订单、商户门头、商品照片或截图等与所述交易相关的证据。', 'image'),
  ('image_tamper_detection', '图片篡改检测', '检查图片是否存在明显拼接、涂改、覆盖、局部重绘或其他编辑/P图痕迹；存在可疑痕迹时判定不合理。', 'image'),
  ('identity_consistency', '身份字段一致性', '身份证图片中可识别的姓名和身份证号应与文本提交内容一致。', 'image');

INSERT INTO `question_skill_refs` (`question_id`, `skill_id`, `sort_order`)
SELECT q.id, s.id, x.sort_order
FROM (
  SELECT 'real_name' question_key, 'real_name_validation' skill_key, 10 sort_order UNION ALL
  SELECT 'id_card_number', 'cn_id_card_validation', 10 UNION ALL
  SELECT 'id_card_image', 'id_card_image_recognition', 10 UNION ALL
  SELECT 'id_card_image', 'image_tamper_detection', 20 UNION ALL
  SELECT 'id_card_image', 'identity_consistency', 30 UNION ALL
  SELECT 'fund_source_description', 'fund_source_description_validation', 10 UNION ALL
  SELECT 'fund_source_evidence', 'fund_source_document_recognition', 10 UNION ALL
  SELECT 'fund_source_evidence', 'image_tamper_detection', 20 UNION ALL
  SELECT 'transaction_description', 'transaction_description_validation', 10 UNION ALL
  SELECT 'transaction_evidence', 'transaction_evidence_recognition', 10 UNION ALL
  SELECT 'transaction_evidence', 'image_tamper_detection', 20
) x
JOIN `risk_factor_questions` q ON q.question_key = x.question_key
JOIN `audit_skills` s ON s.skill_key = x.skill_key;

UPDATE `users`
SET `risk_factor_types` = CASE
  WHEN `risk_factor_types` = '' THEN 'transaction_scene'
  WHEN FIND_IN_SET('transaction_scene', `risk_factor_types`) = 0 THEN CONCAT(`risk_factor_types`, ',transaction_scene')
  ELSE `risk_factor_types`
END
WHERE `user_id` = 'u_1001';

DROP TABLE `risk_factor_main_questions`;
