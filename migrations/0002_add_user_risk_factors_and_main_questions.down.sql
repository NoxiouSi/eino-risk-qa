-- 回滚：按依赖反序删除 0002 迁移引入的表结构变更。
DROP TABLE IF EXISTS `risk_factor_main_questions`;

ALTER TABLE `users`
  DROP COLUMN `risk_factor_types`;
