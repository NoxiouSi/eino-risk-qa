package handler_test

import (
	"bufio"
	"bytes"
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

// newJudgementResult 是测试辅助函数，用于快速构造 fakeJudger 的预设返回值。
func newJudgementResult(completeness, reasonableness bool, followUpQuestion string) *riskfactor.JudgementResult {
	return &riskfactor.JudgementResult{
		Completeness:     completeness,
		Reasonableness:   reasonableness,
		FollowUpQuestion: followUpQuestion,
	}
}

// newTestEngine 构建一套完整的应用（fake infra依赖）+ 路由，用于 handler 层集成测试。
func newTestEngine(judger *fakeJudger, sessionRepo *fakeSessionRepository, userBatchRepo *fakeUserBatchRepository) *server.Hertz {
	sessionSvc := application.NewSessionAppService(judger, sessionRepo)
	batchSvc := application.NewBatchAppService(sessionSvc, userBatchRepo, newSequentialIDGenerator())
	userSvc := application.NewUserAppService(userBatchRepo, newFakeMainQuestionCatalog())

	batchHandler := handler.NewBatchHandler(batchSvc)
	sessionHandler := handler.NewSessionHandler(sessionSvc)
	userHandler := handler.NewUserHandler(userSvc)

	h := server.New()
	api.RegisterRoutes(h, "", batchHandler, sessionHandler, userHandler)
	return h
}

func TestBatchHandler_SubmitBatch_NonStream_Success(t *testing.T) {
	judger := newFakeJudger()
	judger.responses["我是财务经理，工作五年整"] = newJudgementResult(true, true, "")
	h := newTestEngine(judger, newFakeSessionRepository(), newFakeUserBatchRepository())

	body := `{"user":{"user_id":"u_1","name":"张三"},"risk_factors":[{"risk_factor_type":"identity","main_question":"请说明您的身份信息","answer":"我是财务经理，工作五年整"}]}`
	resp := ut.PerformRequest(h.Engine, "POST", "/api/v1/batches", &ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})

	assert.Equal(t, consts.StatusOK, resp.Code)
	var out dto.BatchResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	assert.NotEmpty(t, out.BatchID)
	require.Len(t, out.Results, 1)
	assert.Equal(t, "cleared", out.Results[0].Status)
	assert.Equal(t, "谢谢您的配合，审核结果将在3个工作日内推送给您。", out.Results[0].Message)
}

func TestBatchHandler_SubmitBatch_MissingUserID_ReturnsInvalidParam(t *testing.T) {
	h := newTestEngine(newFakeJudger(), newFakeSessionRepository(), newFakeUserBatchRepository())

	body := `{"user":{"user_id":""},"risk_factors":[{"risk_factor_type":"identity","main_question":"q","answer":"a"}]}`
	resp := ut.PerformRequest(h.Engine, "POST", "/api/v1/batches", &ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})

	assert.Equal(t, consts.StatusBadRequest, resp.Code)
	var out dto.ErrorResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	assert.Equal(t, "INVALID_PARAM", out.ErrorCode)
}

func TestBatchHandler_SubmitBatch_Stream_EmitsSSEFrames(t *testing.T) {
	judger := newFakeJudger()
	judger.responses["短"] = newJudgementResult(false, true, "请补充更多信息")
	h := newTestEngine(judger, newFakeSessionRepository(), newFakeUserBatchRepository())

	body := `{"user":{"user_id":"u_1"},"risk_factors":[{"risk_factor_type":"identity","main_question":"q","answer":"短"}],"stream":true}`
	resp := ut.PerformRequest(h.Engine, "POST", "/api/v1/batches", &ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})

	assert.Equal(t, consts.StatusOK, resp.Code)
	assert.Contains(t, resp.Header().Get("Content-Type"), "text/event-stream")

	events := parseSSEEvents(t, resp.Body.Bytes())
	assertHasEventType(t, events, "batch_created")
	assertHasEventType(t, events, "message_delta")
	assertHasEventType(t, events, "result")
	assertHasEventType(t, events, "done")
}

func TestBatchHandler_GetBatch_NotFound(t *testing.T) {
	h := newTestEngine(newFakeJudger(), newFakeSessionRepository(), newFakeUserBatchRepository())

	resp := ut.PerformRequest(h.Engine, "GET", "/api/v1/batches/batch_missing", nil)

	assert.Equal(t, consts.StatusNotFound, resp.Code)
	var out dto.ErrorResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	assert.Equal(t, "BATCH_NOT_FOUND", out.ErrorCode)
}

func TestBatchHandler_APIKeyAuth_RejectsMissingKey(t *testing.T) {
	sessionSvc := application.NewSessionAppService(newFakeJudger(), newFakeSessionRepository())
	userBatchRepo := newFakeUserBatchRepository()
	batchSvc := application.NewBatchAppService(sessionSvc, userBatchRepo, newSequentialIDGenerator())
	userSvc := application.NewUserAppService(userBatchRepo, newFakeMainQuestionCatalog())
	h := server.New()
	api.RegisterRoutes(h, "secret-key", handler.NewBatchHandler(batchSvc), handler.NewSessionHandler(sessionSvc), handler.NewUserHandler(userSvc))

	resp := ut.PerformRequest(h.Engine, "GET", "/api/v1/batches/whatever", nil)

	assert.Equal(t, consts.StatusUnauthorized, resp.Code)
}

// sseEvent 表示解析后的单个 SSE 事件帧。
type sseEvent struct {
	Event string
	Data  string
}

// parseSSEEvents 按 "event:\ndata:\n\n" 协议解析响应体中的全部事件帧。
func parseSSEEvents(t *testing.T, body []byte) []sseEvent {
	t.Helper()
	var events []sseEvent
	scanner := bufio.NewScanner(bytes.NewReader(body))
	var cur sseEvent
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			cur.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.Data = strings.TrimPrefix(line, "data: ")
		case line == "":
			if cur.Event != "" {
				events = append(events, cur)
				cur = sseEvent{}
			}
		}
	}
	return events
}

func assertHasEventType(t *testing.T, events []sseEvent, eventType string) {
	t.Helper()
	for _, e := range events {
		if e.Event == eventType {
			return
		}
	}
	t.Fatalf("expected an SSE event of type %q, got events: %+v", eventType, events)
}
