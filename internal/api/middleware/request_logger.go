package middleware

import (
	"context"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"

	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

// RequestIDContextKey 供其他 handler 通过 c.Get 取出本次请求的 request_id。
const RequestIDContextKey = "request_id"

// RequestLogger 是关键路径日志的入口中间件：为每个请求生成唯一 request_id 并注入
// RequestContext / context.Context（通过 logging.WithRequestID），请求结束后记录
// method/path/status/耗时/客户端IP，便于串联同一请求在各层产生的日志。
func RequestLogger() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		requestID := uuid.NewString()
		c.Set(RequestIDContextKey, requestID)

		start := time.Now()
		logCtx := logging.WithRequestID(ctx, requestID)

		logging.FromContext(logCtx).Info("http request start",
			"method", string(c.Method()),
			"path", string(c.Path()),
			"client_ip", c.ClientIP(),
		)

		c.Next(logCtx)

		logging.FromContext(logCtx).Info("http request done",
			"method", string(c.Method()),
			"path", string(c.Path()),
			"status", c.Response.StatusCode(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}
