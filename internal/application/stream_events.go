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

// StreamEvent 是 SessionAppService/BatchAppService 流式用例统一产出的事件。
type StreamEvent struct {
	SessionID string
	Type      StreamEventType
	BatchID   string        // Type=BatchCreated 时携带新创建的批次ID，供前端后续查询该批次
	Content   string        // Type=MessageDelta 时的增量文本
	Result    SessionResult // Type=Result 时的最终结果
	ErrorCode string        // Type=Error 时的错误码
	Message   string        // Type=Error 时的错误信息
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
