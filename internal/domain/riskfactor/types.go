package riskfactor

// RiskFactorType 风险要素类型枚举。
type RiskFactorType string

const (
	RiskFactorTypeIdentity   RiskFactorType = "identity"
	RiskFactorTypeFundSource RiskFactorType = "fund_source"
)

// SessionStatus 会话状态机的状态集合。
type SessionStatus string

const (
	// StatusProcessing 处理中（非终态），CurrentRound 取值 0~MaxRounds。
	StatusProcessing SessionStatus = "processing"
	// StatusCleared 终态：已排除合理怀疑。
	StatusCleared SessionStatus = "cleared"
	// StatusNotCleared 终态：未排除合理怀疑（含两种终止原因）。
	StatusNotCleared SessionStatus = "not_cleared"
	// StatusLLMError 非终态、可重试，不消耗轮次。
	StatusLLMError SessionStatus = "llm_error"
)

// TerminationReason 终态 NotCleared 的终止原因。
type TerminationReason string

const (
	// TerminationReasonUnreasonable 完整但不合理。
	TerminationReasonUnreasonable TerminationReason = "unreasonable"
	// TerminationReasonMaxRoundsIncomplete 达到最大轮次仍不完整。
	TerminationReasonMaxRoundsIncomplete TerminationReason = "max_rounds_incomplete"
)

// DefaultMaxRounds 默认最大追问轮次。
const DefaultMaxRounds = 3

// ClosingMessage 到达任意终态后统一对外展示的收尾话术（领域常量）。
const ClosingMessage = "谢谢您的配合，审核结果将在3个工作日内推送给您。"
