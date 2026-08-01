package application_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/application"
	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

func TestUserAppService_GetMainQuestions_Success(t *testing.T) {
	userRepo := newFakeUserBatchRepository()
	userRepo.users["u_1001"] = application.User{
		UserID:          "u_1001",
		Name:            "张三",
		RiskFactorTypes: []string{"identity", "fund_source"},
	}
	catalog := newFakeMainQuestionCatalog()
	catalog.questions["identity"] = "请说明您的身份信息及职业背景"
	catalog.questions["fund_source"] = "请说明本次资金的来源"

	svc := application.NewUserAppService(userRepo, catalog)
	result, err := svc.GetMainQuestions(context.Background(), "u_1001")

	require.NoError(t, err)
	assert.Equal(t, "u_1001", result.UserID)
	require.Len(t, result.Items, 2)
	assert.Equal(t, riskfactor.RiskFactorTypeIdentity, result.Items[0].RiskFactorType)
	assert.Equal(t, "请说明您的身份信息及职业背景", result.Items[0].MainQuestion)
	assert.Equal(t, riskfactor.RiskFactorTypeFundSource, result.Items[1].RiskFactorType)
	assert.Equal(t, "请说明本次资金的来源", result.Items[1].MainQuestion)
}

func TestUserAppService_GetMainQuestions_UserNotFound(t *testing.T) {
	svc := application.NewUserAppService(newFakeUserBatchRepository(), newFakeMainQuestionCatalog())

	_, err := svc.GetMainQuestions(context.Background(), "u_missing")

	assert.ErrorIs(t, err, application.ErrUserNotFound)
}

func TestUserAppService_GetMainQuestions_EmptyRiskFactorTypes_ReturnsEmptyItems(t *testing.T) {
	userRepo := newFakeUserBatchRepository()
	userRepo.users["u_no_risk"] = application.User{UserID: "u_no_risk", RiskFactorTypes: nil}

	svc := application.NewUserAppService(userRepo, newFakeMainQuestionCatalog())
	result, err := svc.GetMainQuestions(context.Background(), "u_no_risk")

	require.NoError(t, err)
	assert.Empty(t, result.Items)
}

// TestUserAppService_GetMainQuestions_SkipsTypeMissingFromCatalog 验证用户配置了某个风险要素类型，
// 但该类型在 risk_factor_main_questions 映射表中缺失主问题时，该类型被跳过而不是返回半成品项，
// 且不影响其他已正确配置的类型正常返回。
func TestUserAppService_GetMainQuestions_SkipsTypeMissingFromCatalog(t *testing.T) {
	userRepo := newFakeUserBatchRepository()
	userRepo.users["u_partial"] = application.User{UserID: "u_partial", RiskFactorTypes: []string{"identity", "unknown_type"}}
	catalog := newFakeMainQuestionCatalog()
	catalog.questions["identity"] = "请说明您的身份信息及职业背景"

	svc := application.NewUserAppService(userRepo, catalog)
	result, err := svc.GetMainQuestions(context.Background(), "u_partial")

	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, riskfactor.RiskFactorTypeIdentity, result.Items[0].RiskFactorType)
}
