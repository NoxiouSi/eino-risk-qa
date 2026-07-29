// Package logging 提供全项目统一的结构化日志封装，基于标准库 log/slog。
//
// 设计原则：
//   - domain 层保持零日志依赖（领域纯净性），日志仅在 application/infra/api 层记录；
//   - 所有关键路径日志均带上下文标识（session_id/batch_id/request_id等），便于问题定位；
//   - 默认输出到 stdout，JSON 格式，便于本地开发查看与生产环境采集。
package logging

import (
	"context"
	"log/slog"
	"os"
)

// L 是全局默认 Logger 实例，Setup 未被调用时使用兜底配置（Info 级别、JSON 格式、输出到 stdout），
// 保证即使忘记调用 Setup（如单元测试中），日志调用也不会 panic。
var L = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

// Setup 根据配置的级别字符串（debug/info/warn/error，大小写不敏感，默认 info）初始化全局 Logger，
// 应在 main() 启动最早期调用一次。
func Setup(level string) {
	var lvl slog.Level
	switch level {
	case "debug", "DEBUG":
		lvl = slog.LevelDebug
	case "warn", "WARN", "warning", "WARNING":
		lvl = slog.LevelWarn
	case "error", "ERROR":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	L = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}

// contextKey 避免与其他包的 context key 冲突。
type contextKey string

const requestIDKey contextKey = "request_id"

// WithRequestID 将 request_id 存入 context，供下游日志调用统一带上该字段。
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFrom 从 context 中取出 request_id，不存在时返回空字符串。
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// FromContext 返回一个自动带上 request_id（若存在）字段的 Logger，
// 调用方在此基础上继续 .With(...) 追加业务字段（session_id/batch_id等）。
func FromContext(ctx context.Context) *slog.Logger {
	if rid := RequestIDFrom(ctx); rid != "" {
		return L.With("request_id", rid)
	}
	return L
}
