package persistence_test

import (
	"fmt"
	"os"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/NoxiouSi/eino-risk-qa/internal/infra/persistence"
)

// setupTestDB 连接本机测试库 eino_risk_qa_test（见 docs/MYSQL_SETUP.md），
// 并在每次调用前清空四张表，保证测试间互不干扰、可重复运行。
// 依赖真实 MySQL 实例：若本机未启动 MySQL 或测试库不存在，测试会直接失败并给出明确提示，
// 而不是静默跳过——因为持久化层的事务/乐观锁行为必须用真实数据库验证。
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := testDSN()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("connect test db %q failed: %v (see docs/MYSQL_SETUP.md to set up eino_risk_qa_test)", dsn, err)
	}

	if err := db.AutoMigrate(&persistence.RiskFactorQuestionModel{}, &persistence.AuditSkillModel{}, &persistence.QuestionSkillRefModel{}, &persistence.UploadedFileModel{}, &persistence.QuestionSubmissionModel{}, &persistence.QARecordModel{}); err != nil {
		t.Fatalf("auto migrate test models failed: %v", err)
	}

	if err := db.Exec("SET FOREIGN_KEY_CHECKS=0").Error; err != nil {
		t.Fatalf("disable fk checks failed: %v", err)
	}
	for _, table := range []string{"question_submissions", "uploaded_files", "question_skill_refs", "audit_skills", "risk_factor_questions", "qa_records", "risk_factor_sessions", "batches", "users"} {
		if err := db.Exec(fmt.Sprintf("TRUNCATE TABLE `%s`", table)).Error; err != nil {
			t.Fatalf("truncate table %s failed: %v", table, err)
		}
	}
	if err := db.Exec("SET FOREIGN_KEY_CHECKS=1").Error; err != nil {
		t.Fatalf("enable fk checks failed: %v", err)
	}

	return db
}

func testDSN() string {
	if v := os.Getenv("EINO_RISK_QA_TEST_DSN"); v != "" {
		return v
	}
	return "eino_risk_qa:EinoRiskQa#2026@tcp(127.0.0.1:3306)/eino_risk_qa_test?charset=utf8mb4&parseTime=True&loc=Local"
}
