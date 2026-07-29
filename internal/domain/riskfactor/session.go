package riskfactor

import (
	"errors"
	"time"
)

// 领域层错误：与状态机规则相关的业务错误，供 application/api 层做错误码转换。
var (
	// ErrSessionNotProcessing 仅 Processing 状态的 session 才允许提交追问回答。
	ErrSessionNotProcessing = errors.New("session is not in processing status")
	// ErrInvalidJudgement 传入的判断结果不满足领域规则（如 nil）。
	ErrInvalidJudgement = errors.New("invalid judgement result")
	// ErrSessionNotFound 是 SessionRepository 端口契约的一部分：FindByID/FindByBatchID 找不到记录时
	// 应返回（或用 fmt.Errorf("...: %w", ErrSessionNotFound) 包装）该哨兵错误，使 application 层
	// 能通过 errors.Is 判断"未找到"语义，而不必反向依赖 infra/persistence 的具体错误类型。
	ErrSessionNotFound = errors.New("session not found")
)

// RiskFactorSession 聚合根：封装状态机全部规则（转移条件、轮次校验、终态判定、
// 提取信息合并、对外展示文案 UserMessage() 推导），是"核心领域知识"的载体。
type RiskFactorSession struct {
	ID             string
	BatchID        string
	UserID         string
	RiskFactorType RiskFactorType
	MainQuestion   string

	Status            SessionStatus
	CurrentRound      int
	MaxRounds         int
	TerminationReason *TerminationReason

	// Completeness/Reasonableness 为最新一轮的判断快照。
	Completeness   *bool
	Reasonableness *bool

	ExtractedInfo map[string]interface{}
	History       []QAPair

	// followUpQuestion 当前待回答的追问问题文本（仅 Status==Processing 时有意义）。
	// 不导出，避免外部绕过状态机直接篡改，只能通过领域方法间接读取（UserMessage）。
	followUpQuestion string

	// version 乐观锁版本号：由持久化层在 FindByID 还原聚合时赋值，Save 时用于检测并发冲突。
	// 不导出，领域方法不会修改它——它不是业务规则的一部分，只是"该聚合被加载时的存储快照版本"，
	// 通过 Version() 只读暴露给 infra 层使用。
	version int

	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewRiskFactorSession 创建一个新的、处于 Processing 状态（round=0）的会话聚合，
// 尚未收到首轮判断结果，History/ExtractedInfo 均为空。
func NewRiskFactorSession(id, batchID, userID string, riskFactorType RiskFactorType, mainQuestion string) *RiskFactorSession {
	now := time.Now()
	return &RiskFactorSession{
		ID:             id,
		BatchID:        batchID,
		UserID:         userID,
		RiskFactorType: riskFactorType,
		MainQuestion:   mainQuestion,
		Status:         StatusProcessing,
		CurrentRound:   0,
		MaxRounds:      DefaultMaxRounds,
		ExtractedInfo:  map[string]interface{}{},
		History:        []QAPair{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// ReconstructParams 用于从持久化存储还原聚合的全部字段（供 infra/persistence 层的 mapper 使用）。
// 这是聚合根与外部存储之间唯一允许的"整体赋值"入口，避免外部代码绕过状态机方法直接篡改内部字段。
type ReconstructParams struct {
	ID                string
	BatchID           string
	UserID            string
	RiskFactorType    RiskFactorType
	MainQuestion      string
	Status            SessionStatus
	CurrentRound      int
	MaxRounds         int
	TerminationReason *TerminationReason
	Completeness      *bool
	Reasonableness    *bool
	ExtractedInfo     map[string]interface{}
	History           []QAPair
	FollowUpQuestion  string
	Version           int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ReconstructRiskFactorSession 从持久化数据还原聚合根，不触发任何状态机校验（假定传入数据本身合法）。
func ReconstructRiskFactorSession(p ReconstructParams) *RiskFactorSession {
	extracted := p.ExtractedInfo
	if extracted == nil {
		extracted = map[string]interface{}{}
	}
	history := p.History
	if history == nil {
		history = []QAPair{}
	}
	return &RiskFactorSession{
		ID:                p.ID,
		BatchID:           p.BatchID,
		UserID:            p.UserID,
		RiskFactorType:    p.RiskFactorType,
		MainQuestion:      p.MainQuestion,
		Status:            p.Status,
		CurrentRound:      p.CurrentRound,
		MaxRounds:         p.MaxRounds,
		TerminationReason: p.TerminationReason,
		Completeness:      p.Completeness,
		Reasonableness:    p.Reasonableness,
		ExtractedInfo:     extracted,
		History:           history,
		followUpQuestion:  p.FollowUpQuestion,
		version:           p.Version,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}
}

// FollowUpQuestion 只读访问当前待回答的追问问题（供 persistence mapper 落库时读取，
// 不允许外部通过此方法反向设置）。
func (s *RiskFactorSession) FollowUpQuestion() string {
	return s.followUpQuestion
}

// Version 只读访问乐观锁版本号（供 persistence 层做并发冲突检测使用）。
func (s *RiskFactorSession) Version() int {
	return s.version
}

// SubmitInitialAnswer 首轮主问题回答提交，内部调用 judgement 驱动状态迁移。
// 允许在 Status 为 Processing（刚创建、尚未有任何判断）或 LLMError（重试首轮）时调用。
func (s *RiskFactorSession) SubmitInitialAnswer(answer string, judgement *JudgementResult) error {
	if judgement == nil {
		return ErrInvalidJudgement
	}
	if s.Status != StatusProcessing && s.Status != StatusLLMError {
		return ErrSessionNotProcessing
	}
	return s.applyJudgement(s.MainQuestion, answer, judgement)
}

// SubmitFollowUpAnswer 追问回答提交，仅 Processing 状态允许调用，内部执行完整性驱动的
// 追问循环与结论合成规则。
func (s *RiskFactorSession) SubmitFollowUpAnswer(answer string, judgement *JudgementResult) error {
	if judgement == nil {
		return ErrInvalidJudgement
	}
	if s.Status != StatusProcessing {
		return ErrSessionNotProcessing
	}
	question := s.followUpQuestion
	return s.applyJudgement(question, answer, judgement)
}

// MarkLLMError 将会话置为 LLMError（非终态、可重试，不消耗轮次）。
func (s *RiskFactorSession) MarkLLMError() {
	s.Status = StatusLLMError
	s.UpdatedAt = time.Now()
}

// UserMessage 对外展示文案推导（核心领域规则）：
// Status==Processing 时返回最新一轮的 follow_up_question；
// 到达任意终态（Cleared/NotCleared）时统一返回领域常量 ClosingMessage。
// 其他状态（如 LLMError）返回空字符串，由上层根据 error 字段单独处理。
func (s *RiskFactorSession) UserMessage() string {
	switch s.Status {
	case StatusProcessing:
		return s.followUpQuestion
	case StatusCleared, StatusNotCleared:
		return ClosingMessage
	default:
		return ""
	}
}

// applyJudgement 是状态机的核心：记录本轮问答、合并提取信息、根据 completeness/reasonableness
// 推导下一个状态（Cleared / NotCleared+reason / 继续Processing并记录下一轮追问问题）。
//
// 规则（与 docs/DESIGN.md 保持一致）：
//   - completeness=true  & reasonableness=true  -> Cleared
//   - completeness=true  & reasonableness=false -> NotCleared(reason=unreasonable)
//   - completeness=false & round已达MaxRounds    -> NotCleared(reason=max_rounds_incomplete)
//   - completeness=false & round<MaxRounds       -> 继续 Processing，round+1，记录 follow_up_question
func (s *RiskFactorSession) applyJudgement(question, answer string, judgement *JudgementResult) error {
	qa := QAPair{
		Round:          s.CurrentRound,
		Question:       question,
		Answer:         answer,
		Completeness:   judgement.Completeness,
		Reasonableness: judgement.Reasonableness,
		CreatedAt:      time.Now(),
	}
	s.History = append(s.History, qa)
	s.ExtractedInfo = judgement.MergeInto(s.ExtractedInfo)

	completeness := judgement.Completeness
	reasonableness := judgement.Reasonableness
	s.Completeness = &completeness
	s.Reasonableness = &reasonableness

	switch {
	case judgement.Completeness && judgement.Reasonableness:
		s.Status = StatusCleared
		s.TerminationReason = nil
		s.followUpQuestion = ""
	case judgement.Completeness && !judgement.Reasonableness:
		s.Status = StatusNotCleared
		reason := TerminationReasonUnreasonable
		s.TerminationReason = &reason
		s.followUpQuestion = ""
	case !judgement.Completeness && s.CurrentRound >= s.MaxRounds:
		s.Status = StatusNotCleared
		reason := TerminationReasonMaxRoundsIncomplete
		s.TerminationReason = &reason
		s.followUpQuestion = ""
	default: // !judgement.Completeness && s.CurrentRound < s.MaxRounds
		s.Status = StatusProcessing
		s.CurrentRound++
		s.followUpQuestion = judgement.FollowUpQuestion
	}

	s.UpdatedAt = time.Now()
	return nil
}
