package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

func TestSetup_ChangesGlobalLoggerLevel(t *testing.T) {
	logging.Setup("debug")
	assert.True(t, logging.L.Enabled(context.Background(), slog.LevelDebug))

	logging.Setup("warn")
	assert.False(t, logging.L.Enabled(context.Background(), slog.LevelInfo))
	assert.True(t, logging.L.Enabled(context.Background(), slog.LevelWarn))

	// 恢复默认级别，避免影响其他测试用例的日志输出行为。
	logging.Setup("info")
}

func TestWithRequestID_And_RequestIDFrom_RoundTrip(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", logging.RequestIDFrom(ctx))

	ctx = logging.WithRequestID(ctx, "req-123")
	assert.Equal(t, "req-123", logging.RequestIDFrom(ctx))
}

func TestFromContext_AttachesRequestIDField(t *testing.T) {
	var buf bytes.Buffer
	original := logging.L
	logging.L = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	defer func() { logging.L = original }()

	ctx := logging.WithRequestID(context.Background(), "req-abc")
	logging.FromContext(ctx).Info("hello")

	var entry map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "req-abc", entry["request_id"])
	assert.Equal(t, "hello", entry["msg"])
}

func TestFromContext_WithoutRequestID_NoExtraField(t *testing.T) {
	var buf bytes.Buffer
	original := logging.L
	logging.L = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	defer func() { logging.L = original }()

	logging.FromContext(context.Background()).Info("hello")

	var entry map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	_, hasRequestID := entry["request_id"]
	assert.False(t, hasRequestID)
}
