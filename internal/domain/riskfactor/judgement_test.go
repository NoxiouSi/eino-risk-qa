package riskfactor_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

func TestAggregateJudgement_AllRequiredQuestionsComplete(t *testing.T) {
	specs := []riskfactor.QuestionSpec{{QuestionKey: "name", Required: true}, {QuestionKey: "image", Required: true}}
	items := []riskfactor.QuestionJudgement{{QuestionKey: "name", Completeness: true, Reasonableness: true}, {QuestionKey: "image", Completeness: true, Reasonableness: true}}

	result := riskfactor.AggregateJudgement(specs, items, nil, "", "")

	assert.True(t, result.Completeness)
	assert.True(t, result.Reasonableness)
	assert.Empty(t, result.MissingQuestions)
	require.Len(t, result.Questions, 2)
	assert.True(t, result.Questions[0].Required)
}

func TestAggregateJudgement_MissingRequiredQuestionIsIncomplete(t *testing.T) {
	specs := []riskfactor.QuestionSpec{{QuestionKey: "name", Required: true}, {QuestionKey: "optional_note", Required: false}}

	result := riskfactor.AggregateJudgement(specs, nil, nil, "", "请补充姓名")

	assert.False(t, result.Completeness)
	assert.True(t, result.Reasonableness)
	assert.Equal(t, []string{"name"}, result.MissingQuestions)
	assert.False(t, result.Questions[1].Required)
}

func TestAggregateJudgement_CompletedUnreasonableEvidenceFailsReasonableness(t *testing.T) {
	specs := []riskfactor.QuestionSpec{{QuestionKey: "evidence", Required: true}}
	items := []riskfactor.QuestionJudgement{{QuestionKey: "evidence", Completeness: true, Reasonableness: false, Note: "存在篡改痕迹"}}

	result := riskfactor.AggregateJudgement(specs, items, nil, "", "")

	assert.True(t, result.Completeness)
	assert.False(t, result.Reasonableness)
}
