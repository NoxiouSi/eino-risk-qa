package middleware

import (
	"context"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/NoxiouSi/eino-risk-qa/internal/api/dto"
	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

// APIKeyHeader 鉴权请求头名称。
const APIKeyHeader = "X-API-Key"

// APIKeyAuth 返回一个校验 X-API-Key 请求头的中间件；apiKey 为空字符串时视为不启用鉴权
// （便于本地开发调试，生产环境应始终配置非空 apiKey）。
func APIKeyAuth(apiKey string) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if apiKey == "" {
			c.Next(ctx)
			return
		}
		if string(c.GetHeader(APIKeyHeader)) != apiKey {
			logging.FromContext(ctx).Warn("api key auth failed", "path", string(c.Path()), "client_ip", c.ClientIP())
			c.JSON(http.StatusUnauthorized, dto.ErrorResponse{
				ErrorCode: "UNAUTHORIZED",
				Message:   "missing or invalid " + APIKeyHeader,
			})
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}
