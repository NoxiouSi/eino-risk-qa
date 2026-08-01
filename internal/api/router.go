package api

import (
	"github.com/cloudwego/hertz/pkg/app/server"

	"github.com/NoxiouSi/eino-risk-qa/internal/api/handler"
	"github.com/NoxiouSi/eino-risk-qa/internal/api/middleware"
)

// RegisterRoutes 注册 /api/v1 下的全部路由，并挂载请求日志中间件与 API Key 鉴权中间件。
func RegisterRoutes(h *server.Hertz, apiKey string, batchHandler *handler.BatchHandler, sessionHandler *handler.SessionHandler, userHandler *handler.UserHandler) {
	v1 := h.Group("/api/v1", middleware.RequestLogger(), middleware.APIKeyAuth(apiKey))

	v1.POST("/batches", batchHandler.SubmitBatch)
	v1.GET("/batches/:batch_id", batchHandler.GetBatch)
	v1.POST("/sessions/:session_id/answers", sessionHandler.SubmitFollowUp)
	v1.GET("/sessions/:session_id", sessionHandler.GetSession)
	v1.GET("/users/:user_id/main-questions", userHandler.GetMainQuestions)
}
