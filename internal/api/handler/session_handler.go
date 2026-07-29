package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/NoxiouSi/eino-risk-qa/internal/api/dto"
	"github.com/NoxiouSi/eino-risk-qa/internal/application"
	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

// SessionHandler 处理 /api/v1/sessions/* 相关的两个接口：追问回答提交、会话详情查询。
type SessionHandler struct {
	sessionSvc *application.SessionAppService
}

// NewSessionHandler 创建 Handler 实例。
func NewSessionHandler(sessionSvc *application.SessionAppService) *SessionHandler {
	return &SessionHandler{sessionSvc: sessionSvc}
}

// SubmitFollowUp 处理 POST /api/v1/sessions/{session_id}/answers。
func (h *SessionHandler) SubmitFollowUp(ctx context.Context, c *app.RequestContext) {
	log := logging.FromContext(ctx)
	sessionID := c.Param("session_id")

	var req dto.FollowUpAnswerRequest
	if err := c.BindAndValidate(&req); err != nil {
		log.Warn("submit follow-up: invalid request body", "session_id", sessionID, "error", err.Error())
		writeError(c, http.StatusBadRequest, CodeInvalidParam, "invalid request body")
		return
	}
	if req.Answer == "" {
		log.Warn("submit follow-up: empty answer", "session_id", sessionID)
		writeError(c, http.StatusBadRequest, CodeInvalidParam, "answer is required")
		return
	}
	log.Info("submit follow-up: accepted", "session_id", sessionID, "stream", req.Stream)

	if !req.Stream {
		result, err := h.sessionSvc.SubmitFollowUp(ctx, sessionID, req.Answer)
		if err != nil {
			log.Warn("submit follow-up: failed", "session_id", sessionID, "error", err.Error())
			writeSubmitFollowUpError(c, err)
			return
		}
		log.Info("submit follow-up: succeeded", "session_id", sessionID, "status", string(result.Status))
		c.JSON(consts.StatusOK, toSessionResultDTO(result))
		return
	}

	pr, pw := io.Pipe()
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.SetStatusCode(consts.StatusOK)
	c.Response.SetBodyStream(pr, -1)

	go func() {
		defer pw.Close()
		log.Info("submit follow-up (stream): started", "session_id", sessionID)
		h.sessionSvc.SubmitFollowUpStream(ctx, sessionID, req.Answer, newSSEForwarder(pw))
		log.Info("submit follow-up (stream): finished", "session_id", sessionID)
	}()
}

// GetSession 处理 GET /api/v1/sessions/{session_id}。
func (h *SessionHandler) GetSession(ctx context.Context, c *app.RequestContext) {
	log := logging.FromContext(ctx)
	sessionID := c.Param("session_id")
	log.Info("get session: received", "session_id", sessionID)

	result, err := h.sessionSvc.GetSession(ctx, sessionID)
	if err != nil {
		if application.IsNotFound(err) {
			log.Warn("get session: not found", "session_id", sessionID)
			writeError(c, http.StatusNotFound, CodeSessionNotFound, "session not found")
			return
		}
		log.Error("get session: application service failed", "session_id", sessionID, "error", err.Error())
		writeError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	log.Info("get session: succeeded", "session_id", sessionID, "status", string(result.Status))
	c.JSON(consts.StatusOK, toSessionDetailDTO(result))
}

func writeSubmitFollowUpError(c *app.RequestContext, err error) {
	switch {
	case application.IsNotFound(err):
		writeError(c, http.StatusNotFound, CodeSessionNotFound, "session not found")
	case isSessionNotProcessing(err):
		writeError(c, http.StatusConflict, CodeSessionNotProcessing, err.Error())
	default:
		writeError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
	}
}

func isSessionNotProcessing(err error) bool {
	return errors.Is(err, riskfactor.ErrSessionNotProcessing)
}
