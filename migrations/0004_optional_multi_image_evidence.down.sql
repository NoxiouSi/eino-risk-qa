SET NAMES utf8mb4;

UPDATE `risk_factor_questions`
SET `required` = 1,
    `min_submit_count` = 1
WHERE `question_key` IN ('fund_source_evidence', 'transaction_evidence');

ALTER TABLE `risk_factor_questions`
  DROP COLUMN `max_submit_count`;
