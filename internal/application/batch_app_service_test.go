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

func newBatchAppServiceForTest(judger *fakeJudger, sessionRepo *fakeSessionRepository) (*application.BatchAppService, *fakeUserBatchRepository) {
	sessionSvc := application.NewSessionAppService(judger, sessionRepo)
	userBatchRepo := newFakeUserBatchRepository()
	batchSvc := application.NewBatchAppService(sessionSvc, userBatchRepo, newSequentialIDGenerator())
	return batchSvc, userBatchRepo
}

func TestBatchAppService_SubmitBatch_MultipleRiskFactors_EachIndependent(t *testing.T) {
	judger := newFakeJudger()
	judger.responses["我是财务经理，工作年限5年整"] = &riskfactor.JudgementResult{Completeness: true, Reasonableness: true}
	judger.responses["短"] = &riskfactor.JudgementResult{Completeness: false, Reasonableness: true, FollowUpQuestion: "请补充更多信息"}
	batchSvc, _ := newBatchAppServiceForTest(judger, newFakeSessionRepository())

	result, err := batchSvc.SubmitBatch(context.Background(), application.SubmitBatchInput{
		UserID:   "user_1",
		UserName: "张三",
		RiskFactors: []application.RiskFactorInput{
			{RiskFactorType: riskfactor.RiskFactorTypeIdentity, MainQuestion: "身份问题", Answer: "我是财务经理，工作年限5年整"},
			{RiskFactorType: riskfactor.RiskFactorTypeFundSource, MainQuestion: "资金来源问题", Answer: "短"},
		},
	})

	require.NoError(t, err)
	assert.NotEmpty(t, result.BatchID)
	require.Len(t, result.Results, 2)

	// 顺序应与输入 RiskFactors 一一对应
	assert.Equal(t, riskfactor.RiskFactorTypeIdentity, result.Results[0].RiskFactorType)
	assert.Equal(t, riskfactor.StatusCleared, result.Results[0].Status)

	assert.Equal(t, riskfactor.RiskFactorTypeFundSource, result.Results[1].RiskFactorType)
	assert.Equal(t, riskfactor.StatusProcessing, result.Results[1].Status)
	assert.Equal(t, "请补充更多信息", result.Results[1].Message)
}

func TestBatchAppService_SubmitBatch_OneFactorFails_DoesNotAffectOthers(t *testing.T) {
	judger := newFakeJudger()
	judger.responses["正常且完整合理的回答内容示例"] = &riskfactor.JudgementResult{Completeness: true, Reasonableness: true}
	judger.errs["会触发失败的回答"] = errors.New("llm unavailable")
	batchSvc, _ := newBatchAppServiceForTest(judger, newFakeSessionRepository())

	result, err := batchSvc.SubmitBatch(context.Background(), application.SubmitBatchInput{
		UserID: "user_1",
		RiskFactors: []application.RiskFactorInput{
			{RiskFactorType: riskfactor.RiskFactorTypeIdentity, MainQuestion: "问题A", Answer: "会触发失败的回答"},
			{RiskFactorType: riskfactor.RiskFactorTypeFundSource, MainQuestion: "问题B", Answer: "正常且完整合理的回答内容示例"},
		},
	})

	require.NoError(t, err) // 整批请求本身不应因单要素失败而返回 error
	require.Len(t, result.Results, 2)
	assert.Equal(t, riskfactor.StatusLLMError, result.Results[0].Status)
	require.NotNil(t, result.Results[0].Error)
	assert.Equal(t, riskfactor.StatusCleared, result.Results[1].Status) // 另一个要素正常完成，不受影响
}

func TestBatchAppService_GetBatch_ReturnsAllSessionsInBatch(t *testing.T) {
	judger := newFakeJudger()
	judger.responses["答案1完整且合理，内容足够详细充分"] = &riskfactor.JudgementResult{Completeness: true, Reasonableness: true}
	judger.responses["答案2完整且合理，内容足够详细充分"] = &riskfactor.JudgementResult{Completeness: true, Reasonableness: true}
	batchSvc, _ := newBatchAppServiceForTest(judger, newFakeSessionRepository())

	submitted, err := batchSvc.SubmitBatch(context.Background(), application.SubmitBatchInput{
		UserID: "user_1",
		RiskFactors: []application.RiskFactorInput{
			{RiskFactorType: riskfactor.RiskFactorTypeIdentity, MainQuestion: "问题A", Answer: "答案1完整且合理，内容足够详细充分"},
			{RiskFactorType: riskfactor.RiskFactorTypeFundSource, MainQuestion: "问题B", Answer: "答案2完整且合理，内容足够详细充分"},
		},
	})
	require.NoError(t, err)

	queried, err := batchSvc.GetBatch(context.Background(), submitted.BatchID)

	require.NoError(t, err)
	assert.Equal(t, submitted.BatchID, queried.BatchID)
	assert.Len(t, queried.Results, 2)
}

func TestBatchAppService_GetBatch_UnknownBatch_ReturnsErrBatchNotFound(t *testing.T) {
	batchSvc, _ := newBatchAppServiceForTest(newFakeJudger(), newFakeSessionRepository())

	_, err := batchSvc.GetBatch(context.Background(), "batch_does_not_exist")

	assert.ErrorIs(t, err, application.ErrBatchNotFound)
}

func TestBatchAppService_SubmitBatch_EnsuresUserAndCreatesBatchRecord(t *testing.T) {
	judger := newFakeJudger()
	judger.responses["示例回答内容足够长以判定完整"] = &riskfactor.JudgementResult{Completeness: true, Reasonableness: true}
	batchSvc, userBatchRepo := newBatchAppServiceForTest(judger, newFakeSessionRepository())

	result, err := batchSvc.SubmitBatch(context.Background(), application.SubmitBatchInput{
		UserID:   "user_42",
		UserName: "李四",
		RiskFactors: []application.RiskFactorInput{
			{RiskFactorType: riskfactor.RiskFactorTypeIdentity, MainQuestion: "问题", Answer: "示例回答内容足够长以判定完整"},
		},
	})

	require.NoError(t, err)
	b, err := userBatchRepo.FindBatch(context.Background(), result.BatchID)
	require.NoError(t, err)
	assert.Equal(t, "user_42", b.UserID)
}
