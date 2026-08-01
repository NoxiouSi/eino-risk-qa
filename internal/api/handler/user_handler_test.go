package handler_test

import (
	"encoding/json"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/api"
	"github.com/NoxiouSi/eino-risk-qa/internal/api/dto"
	"github.com/NoxiouSi/eino-risk-qa/internal/api/handler"
	"github.com/NoxiouSi/eino-risk-qa/internal/application"
)

// newUserTestEngine 构建仅含用户主问题查询能力所需依赖的测试引擎。
func newUserTestEngine(userBatchRepo *fakeUserBatchRepository, catalog *fakeMainQuestionCatalog) *server.Hertz {
	sessionSvc := application.NewSessionAppService(newFakeJudger(), newFakeSessionRepository())
	batchSvc := application.NewBatchAppService(sessionSvc, userBatchRepo, newSequentialIDGenerator())
	userSvc := application.NewUserAppService(userBatchRepo, catalog)

	h := server.New()
	api.RegisterRoutes(h, "", handler.NewBatchHandler(batchSvc), handler.NewSessionHandler(sessionSvc), handler.NewUserHandler(userSvc))
	return h
}

func TestUserHandler_GetMainQuestions_Success(t *testing.T) {
	userRepo := newFakeUserBatchRepository()
	userRepo.users["u_1001"] = application.User{UserID: "u_1001", Name: "张三", RiskFactorTypes: []string{"identity", "fund_source"}}
	catalog := newFakeMainQuestionCatalog()
	catalog.questions["identity"] = "请说明您的身份信息及职业背景"
	catalog.questions["fund_source"] = "请说明本次资金的来源"

	h := newUserTestEngine(userRepo, catalog)
	resp := ut.PerformRequest(h.Engine, "GET", "/api/v1/users/u_1001/main-questions", nil)

	assert.Equal(t, consts.StatusOK, resp.Code)
	var out dto.MainQuestionsResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	assert.Equal(t, "u_1001", out.UserID)
	require.Len(t, out.Items, 2)
	assert.Equal(t, "identity", out.Items[0].RiskFactorType)
	assert.Equal(t, "请说明您的身份信息及职业背景", out.Items[0].MainQuestion)
	assert.Equal(t, "fund_source", out.Items[1].RiskFactorType)
}

func TestUserHandler_GetMainQuestions_UserNotFound(t *testing.T) {
	h := newUserTestEngine(newFakeUserBatchRepository(), newFakeMainQuestionCatalog())

	resp := ut.PerformRequest(h.Engine, "GET", "/api/v1/users/u_missing/main-questions", nil)

	assert.Equal(t, consts.StatusNotFound, resp.Code)
	var out dto.ErrorResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	assert.Equal(t, "USER_NOT_FOUND", out.ErrorCode)
}
