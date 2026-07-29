package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/application"
	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

func TestSessionAppService_SubmitInitial_CompleteAndReasonable_ReturnsCleared(t *testing.T) {
	judger := newFakeJudger()
	judger.responses["我是财务经理，工作年限5年"] = &riskfactor.JudgementResult{
		Completeness: true, Reasonableness: true, ExtractedInfo: map[string]interface{}{"occupation": "财务经理"},
	}
	repo := newFakeSessionRepository()
	svc := application.NewSessionAppService(judger, repo)

	result := svc.SubmitInitial(context.Background(), "sess_1", "batch_1", "user_1",
		riskfactor.RiskFactorTypeIdentity, "请说明您的身份信息及职业背景", "我是财务经理，工作年限5年")

	assert.Equal(t, riskfactor.StatusCleared, result.Status)
	assert.Equal(t, riskfactor.ClosingMessage, result.Message)
	require.NotNil(t, result.Cleared)
	assert.True(t, *result.Cleared)
	assert.Nil(t, result.Error)

	saved, err := repo.FindByID(context.Background(), "sess_1")
	require.NoError(t, err)
	assert.Equal(t, riskfactor.StatusCleared, saved.Status)
}

func TestSessionAppService_SubmitInitial_Incomplete_ReturnsProcessingWithMessage(t *testing.T) {
	judger := newFakeJudger()
	judger.responses["财务经理"] = &riskfactor.JudgementResult{
		Completeness: false, Reasonableness: true, FollowUpQuestion: "任职时间是？",
	}
	svc := application.NewSessionAppService(judger, newFakeSessionRepository())

	result := svc.SubmitInitial(context.Background(), "sess_2", "batch_1", "user_1",
		riskfactor.RiskFactorTypeIdentity, "请说明您的身份信息及职业背景", "财务经理")

	assert.Equal(t, riskfactor.StatusProcessing, result.Status)
	assert.Equal(t, "任职时间是？", result.Message)
	assert.Nil(t, result.Cleared)
}

func TestSessionAppService_SubmitInitial_LLMFailure_ReturnsLLMErrorResult_NotError(t *testing.T) {
	judger := newFakeJudger()
	judger.errs["回答"] = errors.New("llm timeout")
	svc := application.NewSessionAppService(judger, newFakeSessionRepository())

	result := svc.SubmitInitial(context.Background(), "sess_3", "batch_1", "user_1",
		riskfactor.RiskFactorTypeIdentity, "主问题", "回答")

	assert.Equal(t, riskfactor.StatusLLMError, result.Status)
	require.NotNil(t, result.Error)
	assert.Equal(t, "LLM_JUDGE_FAILED", result.Error.Code)
}

func TestSessionAppService_SubmitFollowUp_CompletesAfterFollowUp(t *testing.T) {
	judger := newFakeJudger()
	judger.responses["财务经理"] = &riskfactor.JudgementResult{Completeness: false, Reasonableness: true, FollowUpQuestion: "任职时间是？"}
	judger.responses["2020年至今"] = &riskfactor.JudgementResult{Completeness: true, Reasonableness: true, ExtractedInfo: map[string]interface{}{"tenure": "2020年至今"}}
	repo := newFakeSessionRepository()
	svc := application.NewSessionAppService(judger, repo)

	svc.SubmitInitial(context.Background(), "sess_4", "batch_1", "user_1", riskfactor.RiskFactorTypeIdentity, "主问题", "财务经理")

	result, err := svc.SubmitFollowUp(context.Background(), "sess_4", "2020年至今")

	require.NoError(t, err)
	assert.Equal(t, riskfactor.StatusCleared, result.Status)
	assert.Equal(t, riskfactor.ClosingMessage, result.Message)
}

func TestSessionAppService_SubmitFollowUp_SessionNotFound(t *testing.T) {
	svc := application.NewSessionAppService(newFakeJudger(), newFakeSessionRepository())

	_, err := svc.SubmitFollowUp(context.Background(), "sess_does_not_exist", "任意回答")

	assert.True(t, application.IsNotFound(err))
}

func TestSessionAppService_SubmitFollowUp_OnClearedSession_ReturnsNotProcessingError(t *testing.T) {
	judger := newFakeJudger()
	judger.responses["完整且合理的详细回答内容"] = &riskfactor.JudgementResult{Completeness: true, Reasonableness: true}
	repo := newFakeSessionRepository()
	svc := application.NewSessionAppService(judger, repo)
	svc.SubmitInitial(context.Background(), "sess_5", "batch_1", "user_1", riskfactor.RiskFactorTypeIdentity, "主问题", "完整且合理的详细回答内容")

	_, err := svc.SubmitFollowUp(context.Background(), "sess_5", "追问回答")

	assert.ErrorIs(t, err, riskfactor.ErrSessionNotProcessing)
}

func TestSessionAppService_GetSession(t *testing.T) {
	judger := newFakeJudger()
	repo := newFakeSessionRepository()
	svc := application.NewSessionAppService(judger, repo)
	svc.SubmitInitial(context.Background(), "sess_6", "batch_1", "user_1", riskfactor.RiskFactorTypeIdentity, "主问题", "回答")

	result, err := svc.GetSession(context.Background(), "sess_6")

	require.NoError(t, err)
	assert.Equal(t, "sess_6", result.SessionID)
}
