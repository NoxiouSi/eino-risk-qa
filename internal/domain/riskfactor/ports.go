package riskfactor

import "context"

// QuestionSpec 是判断器所需的可回答问题及其全部审核规则。
type QuestionSpec struct {
	QuestionKey    string
	QuestionText   string
	AnswerType     string
	Required       bool
	MinSubmitCount int
	MaxSubmitCount int
	Rules          []string
}

// QuestionAnswer 是单个问题的结构化文本或文件答案。
type QuestionAnswer struct {
	QuestionKey string
	ValueType   string
	Text        string
	ImagePaths  []string
	FileIDs     []string
}

// JudgeInput 是调用 RiskJudger 所需的完整上下文。
type JudgeInput struct {
	SessionID       string
	RiskFactorType  RiskFactorType
	MainQuestion    string
	History         []QAPair
	CurrentQuestion string
	LatestAnswer    string
	Questions       []QuestionSpec
	Answers         []QuestionAnswer
}

// RiskJudger 领域端口：LLM 判断能力抽象。由 infra/llm 实现，domain/application 只依赖该接口。
type RiskJudger interface {
	// Judge 同步判断：输入完整上下文，输出一次结构化判断。
	Judge(ctx context.Context, input JudgeInput) (*JudgementResult, error)
	// JudgeStream 流式判断：仅在需要补充资料时持续产出追问文本的 message_delta；
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
	// StreamEventMessageDelta 需要补充资料时，追问文案的增量文本片段。
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
