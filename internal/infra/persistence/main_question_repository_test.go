package persistence_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/infra/persistence"
)

// 以下测试使用独立的 test_type_a/test_type_b 而非现有 identity/fund_source，
// 避免污染本地联调所依赖的 seed 数据，并在测试结束后清理。

func TestGORMMainQuestionRepository_FindMainQuestions(t *testing.T) {
	db := setupTestDB(t)
	repo := persistence.NewGORMMainQuestionRepository(db)
	ctx := context.Background()

	require.NoError(t, db.Exec(
		"INSERT INTO risk_factor_main_questions (risk_factor_type, main_question) VALUES (?, ?) "+
			"ON DUPLICATE KEY UPDATE main_question = VALUES(main_question)",
		"test_type_a", "测试主问题A").Error)
	require.NoError(t, db.Exec(
		"INSERT INTO risk_factor_main_questions (risk_factor_type, main_question) VALUES (?, ?) "+
			"ON DUPLICATE KEY UPDATE main_question = VALUES(main_question)",
		"test_type_b", "测试主问题B").Error)
	t.Cleanup(func() {
		db.Exec("DELETE FROM risk_factor_main_questions WHERE risk_factor_type IN (?, ?)", "test_type_a", "test_type_b")
	})

	result, err := repo.FindMainQuestions(ctx, []string{"test_type_a", "test_type_b", "test_type_missing"})

	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"test_type_a": "测试主问题A",
		"test_type_b": "测试主问题B",
	}, result)
}

func TestGORMMainQuestionRepository_FindMainQuestions_EmptyInput(t *testing.T) {
	db := setupTestDB(t)
	repo := persistence.NewGORMMainQuestionRepository(db)

	result, err := repo.FindMainQuestions(context.Background(), nil)

	require.NoError(t, err)
	assert.Empty(t, result)
}
