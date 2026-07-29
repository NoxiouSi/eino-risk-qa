package application

import (
	"time"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

// RiskFactorInput 批量提交中单个风险要素的输入。
type RiskFactorInput struct {
	RiskFactorType riskfactor.RiskFactorType
	MainQuestion   string
	Answer         string
}

// SubmitBatchInput 批量首轮提交用例的输入。
type SubmitBatchInput struct {
	UserID      string
	UserName    string
	RiskFactors []RiskFactorInput
}

// SessionResult 单个风险要素会话的当前状态快照，供 api 层组装响应。
type SessionResult struct {
	SessionID         string
	RiskFactorType    riskfactor.RiskFactorType
	MainQuestion      string
	Status            riskfactor.SessionStatus
	CurrentRound      int
	MaxRounds         int
	Message           string
	Cleared           *bool
	TerminationReason *riskfactor.TerminationReason
	ExtractedInfo     map[string]interface{}
	History           []riskfactor.QAPair
	Error             *ResultError
}

// ResultError 单个风险要素处理失败时的错误信息（不影响批量中其他要素）。
type ResultError struct {
	Code    string
	Message string
}

// BatchResult 批量首轮提交用例的输出。
type BatchResult struct {
	BatchID   string
	CreatedAt time.Time
	Results   []SessionResult
}
