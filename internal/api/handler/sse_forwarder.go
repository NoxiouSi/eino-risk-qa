package handler

import (
	"io"
	"sync"

	"github.com/NoxiouSi/eino-risk-qa/internal/api/dto"
	"github.com/NoxiouSi/eino-risk-qa/internal/api/sse"
	"github.com/NoxiouSi/eino-risk-qa/internal/application"
)

// sseForwarder 实现 application.StreamEmitter：将应用层产出的 StreamEvent 转换为
// SSE 文本帧写入底层 io.Writer。批量场景下多个风险要素的 goroutine 会并发调用 Emit，
// 通过内部 mutex 保证每一帧的 "event:\ndata:\n\n" 不会被其他帧的写入打断/交错。
type sseForwarder struct {
	mu sync.Mutex
	w  io.Writer
}

// newSSEForwarder 创建一个新的转发器实例。
func newSSEForwarder(w io.Writer) *sseForwarder {
	return &sseForwarder{w: w}
}

var _ application.StreamEmitter = (*sseForwarder)(nil)

// Emit 实现 application.StreamEmitter。
func (f *sseForwarder) Emit(event application.StreamEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch event.Type {
	case application.StreamEventBatchCreated:
		_ = sse.WriteEvent(f.w, "batch_created", map[string]string{"batch_id": event.BatchID})
	case application.StreamEventMessageDelta:
		_ = sse.WriteEvent(f.w, "message_delta", dto.SSEMessageDeltaPayload{
			SessionID: event.SessionID,
			Content:   event.Content,
		})
	case application.StreamEventResult:
		r := event.Result
		var terminationReason *string
		if r.TerminationReason != nil {
			v := string(*r.TerminationReason)
			terminationReason = &v
		}
		_ = sse.WriteEvent(f.w, "result", dto.SSEResultPayload{
			SessionID:         r.SessionID,
			RiskFactorType:    string(r.RiskFactorType),
			Status:            string(r.Status),
			CurrentRound:      r.CurrentRound,
			Message:           r.Message,
			Cleared:           r.Cleared,
			TerminationReason: terminationReason,
			ExtractedInfo:     nonNilMap(r.ExtractedInfo),
		})
	case application.StreamEventDone:
		_ = sse.WriteEvent(f.w, "done", dto.SSEDonePayload{SessionID: event.SessionID})
	case application.StreamEventError:
		_ = sse.WriteEvent(f.w, "error", dto.SSEErrorPayload{
			SessionID: event.SessionID,
			ErrorCode: event.ErrorCode,
			Message:   event.Message,
		})
	}
}
