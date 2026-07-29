package handler

import (
	"github.com/cloudwego/hertz/pkg/app"

	"github.com/NoxiouSi/eino-risk-qa/internal/api/dto"
)

// 错误码常量，与 docs/DESIGN.md 中"错误码与HTTP状态码映射表"保持一致。
const (
	CodeInvalidParam         = "INVALID_PARAM"
	CodeUnauthorized         = "UNAUTHORIZED"
	CodeSessionNotFound      = "SESSION_NOT_FOUND"
	CodeBatchNotFound        = "BATCH_NOT_FOUND"
	CodeSessionNotProcessing = "SESSION_NOT_PROCESSING"
	CodeLLMJudgeFailed       = "LLM_JUDGE_FAILED"
	CodeInternalError        = "INTERNAL_ERROR"
)

// requestIDContextKey 中间件（若引入）可通过该 key 向 RequestContext 注入 request_id。
const requestIDContextKey = "request_id"

// writeError 写出统一的错误响应结构体（error_code/message/request_id）。
func writeError(c *app.RequestContext, statusCode int, code, message string) {
	c.JSON(statusCode, dto.ErrorResponse{
		ErrorCode: code,
		Message:   message,
		RequestID: requestIDFrom(c),
	})
}

// requestIDFrom 从上下文中取出 request_id（若中间件已注入），否则返回空字符串。
func requestIDFrom(c *app.RequestContext) string {
	if v, ok := c.Get(requestIDContextKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
