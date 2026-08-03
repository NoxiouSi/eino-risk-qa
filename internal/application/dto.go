package application

import (
	"time"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

// SkillSpec 是问题引用的审核标准快照。
type SkillSpec struct {
	SkillKey string
	Name     string
	RuleText string
}

// QuestionNode 是统一问题配置树中的节点。
type QuestionNode struct {
	ID             uint64
	RiskFactorType string
	QuestionKey    string
	ParentID       *uint64
	QuestionText   string
	AnswerType     string
	Required       bool
	MinSubmitCount int
	MaxSubmitCount int
	SortOrder      int
	Skills         []SkillSpec
	Children       []QuestionNode
}

// QuestionTree 是某一风险要素的 group 主问题及可回答子问题。
type QuestionTree struct {
	RiskFactorType string
	Root           QuestionNode
}

// QuestionAnswerInput 是 API 提交的单个结构化答案。
type QuestionAnswerInput struct {
	QuestionKey string
	Text        string
	FileIDs     []string
}

// RiskFactorInput 批量提交中单个风险要素的输入。
type RiskFactorInput struct {
	RiskFactorType riskfactor.RiskFactorType
	Answers        []QuestionAnswerInput
	// 兼容内部测试构造；API 不再接收这两个旧字段。
	MainQuestion string
	Answer       string
}

// SubmitBatchInput 批量首轮提交用例的输入。
type SubmitBatchInput struct {
	UserID      string
	UserName    string
	RiskFactors []RiskFactorInput
}

// SessionResult 单个风险要素会话的当前状态快照，供 api 层组装响应。
type SessionResult struct {
	SessionID           string
	RiskFactorType      riskfactor.RiskFactorType
	MainQuestion        string
	Questions           []QuestionNode
	Status              riskfactor.SessionStatus
	CurrentRound        int
	MaxRounds           int
	Message             string
	Cleared             *bool
	TerminationReason   *riskfactor.TerminationReason
	ExtractedInfo       map[string]interface{}
	MissingQuestionKeys []string
	QuestionJudgements  []riskfactor.QuestionJudgement
	History             []riskfactor.QAPair
	Error               *ResultError
}

// ResultError 单个风险要素处理失败时的错误信息（不影响批量中其他要素）。
type ResultError struct {
	Code    string
	Message string
}

// BatchResult 批量首轮提交用例的输出。
type BatchResult struct {
	BatchID   string
	UserID    string
	UserName  string
	CreatedAt time.Time
	Results   []SessionResult
}

// MainQuestionItem 单个风险要素的问题树展示数据。
type MainQuestionItem struct {
	RiskFactorType riskfactor.RiskFactorType
	MainQuestion   string
	Questions      []QuestionNode
}

// MainQuestionsResult UserAppService.GetMainQuestions 用例的输出：按用户配置的
// RiskFactorTypes 顺序组装的主问题列表。
type MainQuestionsResult struct {
	UserID string
	Items  []MainQuestionItem
}
