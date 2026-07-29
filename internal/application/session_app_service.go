package application

import (
	"context"
	"errors"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

// SessionAppService 编排单个风险要素会话的首轮提交与追问提交两个用例：
// 加载/新建聚合 -> 调用 RiskJudger 端口获取判断 -> 调用聚合领域方法完成状态迁移 ->
// 通过 SessionRepository 端口持久化，事务边界在此层控制（不包含业务规则本身）。
type SessionAppService struct {
	judger riskfactor.RiskJudger
	repo   riskfactor.SessionRepository
}

// NewSessionAppService 创建应用服务实例。
func NewSessionAppService(judger riskfactor.RiskJudger, repo riskfactor.SessionRepository) *SessionAppService {
	return &SessionAppService{judger: judger, repo: repo}
}

// SubmitInitial 首轮主问题回答提交：创建新的聚合、调用 LLM 判断、驱动状态迁移、持久化。
// LLM 调用/持久化失败时不返回 error 中断整批（由调用方 BatchAppService 决定），而是返回一个
// Status=LLMError 的 SessionResult，附带 error 详情，交由上层决定如何呈现。
func (s *SessionAppService) SubmitInitial(ctx context.Context, sessionID, batchID, userID string, riskFactorType riskfactor.RiskFactorType, mainQuestion, answer string) SessionResult {
	log := logging.FromContext(ctx).With("session_id", sessionID, "batch_id", batchID, "risk_factor_type", string(riskFactorType))
	log.Info("submit initial: start")
	session := riskfactor.NewRiskFactorSession(sessionID, batchID, userID, riskFactorType, mainQuestion)

	judgement, err := s.judger.Judge(ctx, riskfactor.JudgeInput{
		SessionID:       sessionID,
		RiskFactorType:  riskFactorType,
		MainQuestion:    mainQuestion,
		History:         nil,
		CurrentQuestion: mainQuestion,
		LatestAnswer:    answer,
	})
	if err != nil {
		log.Error("submit initial: judge failed", "error", err.Error())
		return llmErrorResult(sessionID, riskFactorType, mainQuestion, err)
	}

	result := s.applyJudgementAndSave(ctx, session, answer, judgement)
	log.Info("submit initial: done", "status", string(result.Status))
	return result
}

// SubmitFollowUp 追问回答提交：加载已存在的聚合、调用 LLM 判断、驱动状态迁移、持久化。
//
// 错误语义（供 api 层做错误码转换）：
//   - errors.Is(err, riskfactor.ErrSessionNotFound)      -> 404 SESSION_NOT_FOUND
//   - errors.Is(err, riskfactor.ErrSessionNotProcessing) -> 409 SESSION_NOT_PROCESSING
//   - 其他 error                                          -> 500 INTERNAL_ERROR
//
// LLM 调用失败时不返回 error，而是返回 Status=LLMError 的 SessionResult（与批量提交一致的处理方式）。
func (s *SessionAppService) SubmitFollowUp(ctx context.Context, sessionID, answer string) (SessionResult, error) {
	log := logging.FromContext(ctx).With("session_id", sessionID)
	log.Info("submit follow-up: start")

	session, err := s.repo.FindByID(ctx, sessionID)
	if err != nil {
		log.Warn("submit follow-up: session not found", "error", err.Error())
		return SessionResult{}, err
	}
	if session.Status != riskfactor.StatusProcessing {
		log.Warn("submit follow-up: session not processing", "status", string(session.Status))
		return SessionResult{}, riskfactor.ErrSessionNotProcessing
	}

	judgement, err := s.judger.Judge(ctx, riskfactor.JudgeInput{
		SessionID:       sessionID,
		RiskFactorType:  session.RiskFactorType,
		MainQuestion:    session.MainQuestion,
		History:         session.History,
		CurrentQuestion: session.FollowUpQuestion(),
		LatestAnswer:    answer,
	})
	if err != nil {
		log.Error("submit follow-up: judge failed", "error", err.Error())
		return llmErrorResult(sessionID, session.RiskFactorType, session.MainQuestion, err), nil
	}

	result := s.applyJudgementAndSave(ctx, session, answer, judgement)
	log.Info("submit follow-up: done", "status", string(result.Status))
	return result, nil
}

// SubmitInitialStream 首轮提交的流式变体：转发 RiskJudger.JudgeStream 产出的 message_delta 事件，
// 收到最终判断结果后，执行与 SubmitInitial 完全一致的状态迁移与持久化逻辑，最后发出 Result + Done 事件。
// LLM/状态迁移/持久化任一环节失败，发出 Error + Done 事件（不返回 Go error，语义与同步路径一致）。
func (s *SessionAppService) SubmitInitialStream(ctx context.Context, sessionID, batchID, userID string, riskFactorType riskfactor.RiskFactorType, mainQuestion, answer string, emitter StreamEmitter) {
	log := logging.FromContext(ctx).With("session_id", sessionID, "batch_id", batchID, "risk_factor_type", string(riskFactorType))
	log.Info("submit initial (stream): start")
	session := riskfactor.NewRiskFactorSession(sessionID, batchID, userID, riskFactorType, mainQuestion)

	events, err := s.judger.JudgeStream(ctx, riskfactor.JudgeInput{
		SessionID:       sessionID,
		RiskFactorType:  riskFactorType,
		MainQuestion:    mainQuestion,
		CurrentQuestion: mainQuestion,
		LatestAnswer:    answer,
	})
	if err != nil {
		log.Error("submit initial (stream): judge stream failed", "error", err.Error())
		emitLLMError(emitter, sessionID, err)
		return
	}

	judgement := consumeJudgeStream(sessionID, events, emitter)
	if judgement == nil {
		log.Warn("submit initial (stream): judge stream terminated with error")
		return // 已发出 Error+Done
	}

	result := s.applyJudgementAndSave(ctx, session, answer, judgement)
	log.Info("submit initial (stream): done", "status", string(result.Status))
	emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventResult, Result: result})
	emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventDone})
}

// SubmitFollowUpStream 追问提交的流式变体，行为与 SubmitInitialStream 对称。
// 若 session 不存在或状态不允许追问，发出 Error + Done 事件。
func (s *SessionAppService) SubmitFollowUpStream(ctx context.Context, sessionID, answer string, emitter StreamEmitter) {
	log := logging.FromContext(ctx).With("session_id", sessionID)
	log.Info("submit follow-up (stream): start")

	session, err := s.repo.FindByID(ctx, sessionID)
	if err != nil {
		log.Warn("submit follow-up (stream): session not found", "error", err.Error())
		emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventError, ErrorCode: "SESSION_NOT_FOUND", Message: err.Error()})
		emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventDone})
		return
	}
	if session.Status != riskfactor.StatusProcessing {
		log.Warn("submit follow-up (stream): session not processing", "status", string(session.Status))
		emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventError, ErrorCode: "SESSION_NOT_PROCESSING", Message: riskfactor.ErrSessionNotProcessing.Error()})
		emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventDone})
		return
	}

	events, err := s.judger.JudgeStream(ctx, riskfactor.JudgeInput{
		SessionID:       sessionID,
		RiskFactorType:  session.RiskFactorType,
		MainQuestion:    session.MainQuestion,
		History:         session.History,
		CurrentQuestion: session.FollowUpQuestion(),
		LatestAnswer:    answer,
	})
	if err != nil {
		log.Error("submit follow-up (stream): judge stream failed", "error", err.Error())
		emitLLMError(emitter, sessionID, err)
		return
	}

	judgement := consumeJudgeStream(sessionID, events, emitter)
	if judgement == nil {
		log.Warn("submit follow-up (stream): judge stream terminated with error")
		return
	}

	result := s.applyJudgementAndSave(ctx, session, answer, judgement)
	log.Info("submit follow-up (stream): done", "status", string(result.Status))
	emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventResult, Result: result})
	emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventDone})
}

// GetSession 查询单个会话当前完整状态。
func (s *SessionAppService) GetSession(ctx context.Context, sessionID string) (SessionResult, error) {
	log := logging.FromContext(ctx).With("session_id", sessionID)
	session, err := s.repo.FindByID(ctx, sessionID)
	if err != nil {
		log.Warn("get session: not found", "error", err.Error())
		return SessionResult{}, err
	}
	return toSessionResult(session), nil
}

// IsNotFound 供 api 层统一判断"未找到"语义，避免 api 层直接依赖 domain 包的错误变量名细节。
func IsNotFound(err error) bool {
	return errors.Is(err, riskfactor.ErrSessionNotFound)
}

// applyJudgementAndSave 是同步/流式两条路径共用的核心步骤：调用聚合领域方法完成状态迁移、持久化。
// 正是这一函数的复用，保证了"流式仅影响传输方式，不改变domain状态机契约"这一设计原则。
func (s *SessionAppService) applyJudgementAndSave(ctx context.Context, session *riskfactor.RiskFactorSession, answer string, judgement *riskfactor.JudgementResult) SessionResult {
	log := logging.FromContext(ctx).With("session_id", session.ID)
	var err error
	if session.CurrentRound == 0 && len(session.History) == 0 {
		err = session.SubmitInitialAnswer(answer, judgement)
	} else {
		err = session.SubmitFollowUpAnswer(answer, judgement)
	}
	if err != nil {
		log.Error("apply judgement: state transition failed", "error", err.Error())
		return llmErrorResult(session.ID, session.RiskFactorType, session.MainQuestion, err)
	}
	if err := s.repo.Save(ctx, session); err != nil {
		log.Error("apply judgement: persist failed", "error", err.Error())
		return llmErrorResult(session.ID, session.RiskFactorType, session.MainQuestion, err)
	}
	log.Debug("apply judgement: persisted", "status", string(session.Status), "current_round", session.CurrentRound)
	return toSessionResult(session)
}

// consumeJudgeStream 转发 message_delta 事件，直到收到 Result 或 Error 终止事件；
// 返回值为 nil 表示已发生错误（错误+done事件已由本函数发出），调用方应直接返回。
func consumeJudgeStream(sessionID string, events <-chan riskfactor.JudgeStreamEvent, emitter StreamEmitter) *riskfactor.JudgementResult {
	for ev := range events {
		switch ev.Type {
		case riskfactor.StreamEventMessageDelta:
			emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventMessageDelta, Content: ev.MessageDelta})
		case riskfactor.StreamEventResult:
			return ev.Result
		case riskfactor.StreamEventError:
			emitLLMError(emitter, sessionID, ev.Err)
			return nil
		}
	}
	return nil
}

func emitLLMError(emitter StreamEmitter, sessionID string, err error) {
	emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventError, ErrorCode: "LLM_JUDGE_FAILED", Message: err.Error()})
	emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventDone})
}

func llmErrorResult(sessionID string, riskFactorType riskfactor.RiskFactorType, mainQuestion string, err error) SessionResult {
	return SessionResult{
		SessionID:      sessionID,
		RiskFactorType: riskFactorType,
		MainQuestion:   mainQuestion,
		Status:         riskfactor.StatusLLMError,
		Error:          &ResultError{Code: "LLM_JUDGE_FAILED", Message: err.Error()},
	}
}

func toSessionResult(s *riskfactor.RiskFactorSession) SessionResult {
	var cleared *bool
	switch s.Status {
	case riskfactor.StatusCleared:
		v := true
		cleared = &v
	case riskfactor.StatusNotCleared:
		v := false
		cleared = &v
	}
	return SessionResult{
		SessionID:         s.ID,
		RiskFactorType:    s.RiskFactorType,
		MainQuestion:      s.MainQuestion,
		Status:            s.Status,
		CurrentRound:      s.CurrentRound,
		MaxRounds:         s.MaxRounds,
		Message:           s.UserMessage(),
		Cleared:           cleared,
		TerminationReason: s.TerminationReason,
		ExtractedInfo:     s.ExtractedInfo,
		History:           s.History,
	}
}
