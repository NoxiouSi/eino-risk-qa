package persistence_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/infra/persistence"
)

func TestGORMMainQuestionRepository_FindQuestionTrees(t *testing.T) {
	db := setupTestDB(t)
	repo := persistence.NewGORMMainQuestionRepository(db)
	ctx := context.Background()

	root := persistence.RiskFactorQuestionModel{RiskFactorType: "test_type_a", QuestionKey: "test_main", QuestionText: "测试主问题A", AnswerType: "group", Required: true, Enabled: true}
	require.NoError(t, db.Create(&root).Error)
	child := persistence.RiskFactorQuestionModel{RiskFactorType: "test_type_a", QuestionKey: "test_text", ParentID: &root.ID, QuestionText: "测试文本", AnswerType: "text", Required: true, MinSubmitCount: 1, MaxSubmitCount: 3, SortOrder: 10, Enabled: true}
	require.NoError(t, db.Create(&child).Error)
	skill := persistence.AuditSkillModel{SkillKey: "test_rule", Name: "测试规则", RuleText: "必须清晰具体", EvidenceType: "text", Enabled: true}
	require.NoError(t, db.Create(&skill).Error)
	require.NoError(t, db.Create(&persistence.QuestionSkillRefModel{QuestionID: child.ID, SkillID: skill.ID, SortOrder: 10}).Error)

	result, err := repo.FindQuestionTrees(ctx, []string{"test_type_a", "missing"})
	require.NoError(t, err)
	require.Contains(t, result, "test_type_a")
	assert.Equal(t, "测试主问题A", result["test_type_a"].Root.QuestionText)
	require.Len(t, result["test_type_a"].Root.Children, 1)
	assert.Equal(t, "test_text", result["test_type_a"].Root.Children[0].QuestionKey)
	assert.Equal(t, 3, result["test_type_a"].Root.Children[0].MaxSubmitCount)
	require.Len(t, result["test_type_a"].Root.Children[0].Skills, 1)
	assert.Equal(t, "test_rule", result["test_type_a"].Root.Children[0].Skills[0].SkillKey)
}

func TestGORMMainQuestionRepository_FindQuestionTrees_EmptyInput(t *testing.T) {
	repo := persistence.NewGORMMainQuestionRepository(setupTestDB(t))
	result, err := repo.FindQuestionTrees(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}
