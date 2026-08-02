package handler_test

import (
	"context"
	"encoding/json"
	"strings"
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
	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

func newSessionTestEngine(judger *fakeJudger, sessionRepo *fakeSessionRepository) *server.Hertz {
	sessionSvc := application.NewSessionAppService(judger, sessionRepo)
	userBatchRepo := newFakeUserBatchRepository()
	batchSvc := application.NewBatchAppService(sessionSvc, userBatchRepo, newSequentialIDGenerator())
	userSvc := application.NewUserAppService(userBatchRepo, newFakeMainQuestionCatalog())
	h := server.New()
	api.RegisterRoutes(h, "", handler.NewBatchHandler(batchSvc), handler.NewSessionHandler(sessionSvc), handler.NewUserHandler(userSvc))
	return h
}

func jsonHeader() ut.Header {
	return ut.Header{Key: "Content-Type", Value: "application/json"}
}

func TestSessionHandler_SubmitFollowUp_NonStream_CompletesSession(t *testing.T) {
	judger := newFakeJudger()
	judger.responses["财务经理"] = newJudgementResult(false, true, "任职时间是？")
	judger.responses["2020年至今"] = newJudgementResult(true, true, "")
	repo := newFakeSessionRepository()
	sessionSvc := application.NewSessionAppService(judger, repo)
	sessionSvc.SubmitInitial(context.Background(), "sess_1", "batch_1", "user_1", riskfactor.RiskFactorTypeIdentity, "主问题", "财务经理")

	h := newSessionTestEngine(judger, repo)

	body := `{"answer":"2020年至今"}`
	resp := ut.PerformRequest(h.Engine, "POST", "/api/v1/sessions/sess_1/answers", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, jsonHeader())

	assert.Equal(t, consts.StatusOK, resp.Code)
	var out dto.SessionResult
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	assert.Equal(t, "cleared", out.Status)
	assert.Equal(t, riskfactor.SessionCompletedMessage, out.Message)
}

func TestSessionHandler_SubmitFollowUp_SessionNotFound(t *testing.T) {
	h := newSessionTestEngine(newFakeJudger(), newFakeSessionRepository())

	body := `{"answer":"任意"}`
	resp := ut.PerformRequest(h.Engine, "POST", "/api/v1/sessions/sess_missing/answers", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, jsonHeader())

	assert.Equal(t, consts.StatusNotFound, resp.Code)
	var out dto.ErrorResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	assert.Equal(t, "SESSION_NOT_FOUND", out.ErrorCode)
}

func TestSessionHandler_SubmitFollowUp_OnClearedSession_ReturnsConflict(t *testing.T) {
	judger := newFakeJudger()
	judger.responses["完整且合理的详细回答内容"] = newJudgementResult(true, true, "")
	repo := newFakeSessionRepository()
	sessionSvc := application.NewSessionAppService(judger, repo)
	sessionSvc.SubmitInitial(context.Background(), "sess_2", "batch_1", "user_1", riskfactor.RiskFactorTypeIdentity, "主问题", "完整且合理的详细回答内容")

	h := newSessionTestEngine(judger, repo)
	body := `{"answer":"追问回答"}`
	resp := ut.PerformRequest(h.Engine, "POST", "/api/v1/sessions/sess_2/answers", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, jsonHeader())

	assert.Equal(t, consts.StatusConflict, resp.Code)
	var out dto.ErrorResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	assert.Equal(t, "SESSION_NOT_PROCESSING", out.ErrorCode)
}

func TestSessionHandler_SubmitFollowUp_EmptyAnswer_InvalidParam(t *testing.T) {
	h := newSessionTestEngine(newFakeJudger(), newFakeSessionRepository())

	body := `{"answer":""}`
	resp := ut.PerformRequest(h.Engine, "POST", "/api/v1/sessions/sess_1/answers", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, jsonHeader())

	assert.Equal(t, consts.StatusBadRequest, resp.Code)
}

func TestSessionHandler_SubmitFollowUp_Stream_EmitsSSEFrames(t *testing.T) {
	judger := newFakeJudger()
	judger.responses["财务经理"] = newJudgementResult(false, true, "任职时间是？")
	judger.responses["2020年至今"] = newJudgementResult(true, true, "")
	repo := newFakeSessionRepository()
	sessionSvc := application.NewSessionAppService(judger, repo)
	sessionSvc.SubmitInitial(context.Background(), "sess_3", "batch_1", "user_1", riskfactor.RiskFactorTypeIdentity, "主问题", "财务经理")

	h := newSessionTestEngine(judger, repo)
	body := `{"answer":"2020年至今","stream":true}`
	resp := ut.PerformRequest(h.Engine, "POST", "/api/v1/sessions/sess_3/answers", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, jsonHeader())

	assert.Equal(t, consts.StatusOK, resp.Code)
	assert.Contains(t, resp.Header().Get("Content-Type"), "text/event-stream")
	events := parseSSEEvents(t, resp.Body.Bytes())
	assertNotHasEventType(t, events, "message_delta")
	assertHasEventType(t, events, "result")
	assertHasEventType(t, events, "done")
}

func TestSessionHandler_GetSession_ReturnsFullDetailWithHistory(t *testing.T) {
	judger := newFakeJudger()
	judger.responses["财务经理"] = newJudgementResult(false, true, "任职时间是？")
	repo := newFakeSessionRepository()
	sessionSvc := application.NewSessionAppService(judger, repo)
	sessionSvc.SubmitInitial(context.Background(), "sess_4", "batch_1", "user_1", riskfactor.RiskFactorTypeIdentity, "主问题", "财务经理")

	h := newSessionTestEngine(judger, repo)
	resp := ut.PerformRequest(h.Engine, "GET", "/api/v1/sessions/sess_4", nil)

	assert.Equal(t, consts.StatusOK, resp.Code)
	var out dto.SessionDetail
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	assert.Equal(t, "processing", out.Status)
	assert.Equal(t, "任职时间是？", out.Message)
	require.Len(t, out.History, 1)
	assert.Equal(t, "财务经理", out.History[0].Answer)
}
