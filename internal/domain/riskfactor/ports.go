package riskfactor

import "context"

// JudgeInput 是调用 RiskJudger 所需的完整上下文，避免端口方法签名随参数增多而不断膨胀。
type JudgeInput struct {
	SessionID      string // 多路复用标识（流式场景下用于区分事件归属），同步调用可忽略
	RiskFactorType RiskFactorType
	MainQuestion   string   // 风险要素的主问题
	History        []QAPair // 已完成的历史问答（不含本轮）
	// CurrentQuestion 本轮用户实际正在回答的问题文本：
	//   - 首轮（history 为空）时等于 MainQuestion；
	//   - 追问轮次时等于上一轮判断产生的 follow_up_question（即 RiskFactorSession.FollowUpQuestion()）。
	CurrentQuestion string
	LatestAnswer    string // 用户对 CurrentQuestion 的回答
}

// RiskJudger 领域端口：LLM 判断能力抽象。由 infra/llm 实现，domain/application 只依赖该接口。
type RiskJudger interface {
	// Judge 同步判断：输入完整上下文，输出一次结构化判断。
	Judge(ctx context.Context, input JudgeInput) (*JudgementResult, error)
	// JudgeStream 流式判断：持续产出 message_delta 事件（追问问题的真实逐字增量，或终态收尾话术），
	// channel 关闭前必发出一条 Type=StreamEventResult 或 Type=StreamEventError 的终止事件。
	JudgeStream(ctx context.Context, input JudgeInput) (<-chan JudgeStreamEvent, error)
}

// SessionRepository 领域端口：持久化能力抽象。由 infra/persistence 实现。
type SessionRepository interface {
	// Save 事务内同时持久化 session 状态与新增的 QA 记录。
	Save(ctx context.Context, session *RiskFactorSession) error
	// FindByID 按业务 session_id 加载聚合，还原完整历史。
	FindByID(ctx context.Context, sessionID string) (*RiskFactorSession, error)
	// FindByBatchID 按业务 batch_id 列出该批次下的全部会话（用于批次查询接口）。
	FindByBatchID(ctx context.Context, batchID string) ([]*RiskFactorSession, error)
}

// StreamEventType 流式事件类型。
type StreamEventType string

const (
	// StreamEventMessageDelta message（追问问题或收尾话术）的增量文本片段。
	StreamEventMessageDelta StreamEventType = "message_delta"
	// StreamEventResult 最终完整的结构化判断结果。
	StreamEventResult StreamEventType = "result"
	// StreamEventError LLM调用/解析失败。
	StreamEventError StreamEventType = "error"
)

// JudgeStreamEvent 流式判断过程中的单个事件。
type JudgeStreamEvent struct {
	SessionID    string // 多路复用标识：批量场景下用于区分事件归属的风险要素
	Type         StreamEventType
	MessageDelta string           // Type=StreamEventMessageDelta 时的文本片段
	Result       *JudgementResult // Type=StreamEventResult 时的最终结构化结果
	Err          error            // Type=StreamEventError 时的错误
}
