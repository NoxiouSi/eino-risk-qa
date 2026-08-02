CREATE TABLE `risk_factor_main_questions` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `risk_factor_type` VARCHAR(32) NOT NULL,
  `main_question` TEXT NOT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_risk_factor_type` (`risk_factor_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='风险要素类型-主问题映射表';

INSERT INTO `risk_factor_main_questions` (`risk_factor_type`, `main_question`, `created_at`, `updated_at`)
SELECT `risk_factor_type`, `question_text`, `created_at`, `updated_at`
FROM `risk_factor_questions`
WHERE `answer_type` = 'group' AND `risk_factor_type` IN ('identity', 'fund_source');

ALTER TABLE `qa_records` DROP COLUMN `question_judgements`;
DROP TABLE IF EXISTS `question_submissions`;
DROP TABLE IF EXISTS `uploaded_files`;
DROP TABLE IF EXISTS `question_skill_refs`;
DROP TABLE IF EXISTS `audit_skills`;
DROP TABLE IF EXISTS `risk_factor_questions`;

UPDATE `users`
SET `risk_factor_types` = TRIM(BOTH ',' FROM REPLACE(CONCAT(',', `risk_factor_types`, ','), ',transaction_scene,', ','))
WHERE FIND_IN_SET('transaction_scene', `risk_factor_types`) > 0;
