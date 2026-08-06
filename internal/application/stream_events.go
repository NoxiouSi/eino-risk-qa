package application

// StreamEventType 应用层流式事件类型，与 api 层的 SSE 事件名一一对应，
// 但不依赖 api/dto 包（保持依赖方向：api -> application）。
type StreamEventType string

const (
	StreamEventBatchCreated StreamEventType = "batch_created"
	StreamEventMessageDelta StreamEventType = "message_delta"
	StreamEventResult       StreamEventType = "result"
	StreamEventDone         StreamEventType = "done"
	StreamEventError        StreamEventType = "error"
)

// BatchSessionInfo 描述批次中一个会话的标识信息，随 batch_created 事件一次性发送给前端，
// 使前端能在任何 message_delta 到达之前就预先创建所有卡片，后续严格按 session_id 匹配。
type BatchSessionInfo struct {
	SessionID      string
	RiskFactorType string
}

// StreamEvent 是 SessionAppService/BatchAppService 流式用例统一产出的事件。
type StreamEvent struct {
	SessionID string
	Type      StreamEventType
	BatchID   string           // Type=BatchCreated 时携带新创建的批次ID
	Sessions  []BatchSessionInfo // Type=BatchCreated 时携带该批次下所有会话的标识清单
	Content   string           // Type=MessageDelta 时的增量文本
	Result    SessionResult    // Type=Result 时的最终结果
	ErrorCode string           // Type=Error 时的错误码
	Message   string           // Type=Error 时的错误信息
}

// StreamEmitter 由调用方（通常是 api 层）实现，用于消费流式事件（如写出 SSE 帧）。
// 使用接口而非裸 channel，便于批量场景下多个并发 goroutine 安全地写入同一底层输出
// （具体实现自行处理并发写入的同步问题）。
type StreamEmitter interface {
	Emit(event StreamEvent)
}

// StreamEmitterFunc 是 StreamEmitter 的函数式适配器。
type StreamEmitterFunc func(event StreamEvent)

// Emit 实现 StreamEmitter。
func (f StreamEmitterFunc) Emit(event StreamEvent) { f(event) }
