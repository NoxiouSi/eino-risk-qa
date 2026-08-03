SET NAMES utf8mb4;

ALTER TABLE `risk_factor_questions`
  ADD COLUMN `max_submit_count` INT UNSIGNED NOT NULL DEFAULT 1 AFTER `min_submit_count`;

UPDATE `risk_factor_questions`
SET `required` = 0,
    `min_submit_count` = 1,
    `max_submit_count` = 5
WHERE `question_key` IN ('fund_source_evidence', 'transaction_evidence');
