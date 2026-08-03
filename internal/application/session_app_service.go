package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

// SessionAppService 编排单个风险要素会话的首轮提交与追问提交两个用例：
// 加载/新建聚合 -> 调用 RiskJudger 端口获取判断 -> 调用聚合领域方法完成状态迁移 ->
// 通过 SessionRepository 端口持久化，事务边界在此层控制（不包含业务规则本身）。
type SessionAppService struct {
	judger              riskfactor.RiskJudger
	repo                riskfactor.SessionRepository
	catalog             RiskFactorQuestionCatalog
	attachments         AttachmentRepository
	storageRoot         string
	maxFilesPerQuestion int
}

// NewSessionAppService 创建应用服务实例。
func NewSessionAppService(judger riskfactor.RiskJudger, repo riskfactor.SessionRepository) *SessionAppService {
	return &SessionAppService{judger: judger, repo: repo}
}

// ConfigureQuestionSupport 装配统一问题目录与附件能力。
func (s *SessionAppService) ConfigureQuestionSupport(catalog RiskFactorQuestionCatalog, attachments AttachmentRepository, storageRoot string, maxFiles ...int) {
	s.catalog = catalog
	s.attachments = attachments
	s.storageRoot = storageRoot
	if len(maxFiles) > 0 {
		s.maxFilesPerQuestion = maxFiles[0]
	}
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

// SubmitInitialQuestions 使用统一问题配置处理首轮结构化答案。
func (s *SessionAppService) SubmitInitialQuestions(ctx context.Context, sessionID, batchID, userID string, riskFactorType riskfactor.RiskFactorType, answers []QuestionAnswerInput) SessionResult {
	mainQuestion, specs, resolved, summary, err := s.prepareQuestionInput(ctx, userID, riskFactorType, answers)
	if err != nil {
		return llmErrorResult(sessionID, riskFactorType, mainQuestion, err)
	}
	session := riskfactor.NewRiskFactorSession(sessionID, batchID, userID, riskFactorType, mainQuestion)
	judgement, err := s.judger.Judge(ctx, riskfactor.JudgeInput{SessionID: sessionID, RiskFactorType: riskFactorType, MainQuestion: mainQuestion, CurrentQuestion: mainQuestion, LatestAnswer: summary, Questions: specs, Answers: resolved})
	if err != nil {
		return llmErrorResult(sessionID, riskFactorType, mainQuestion, err)
	}
	return s.applyJudgementAndSave(ctx, session, summary, judgement, resolved)
}

// SubmitInitialQuestionsStream 是结构化首轮提交的流式变体。
func (s *SessionAppService) SubmitInitialQuestionsStream(ctx context.Context, sessionID, batchID, userID string, riskFactorType riskfactor.RiskFactorType, answers []QuestionAnswerInput, emitter StreamEmitter) {
	mainQuestion, specs, resolved, summary, err := s.prepareQuestionInput(ctx, userID, riskFactorType, answers)
	if err != nil {
		emitLLMError(emitter, sessionID, err)
		return
	}
	session := riskfactor.NewRiskFactorSession(sessionID, batchID, userID, riskFactorType, mainQuestion)
	events, err := s.judger.JudgeStream(ctx, riskfactor.JudgeInput{SessionID: sessionID, RiskFactorType: riskFactorType, MainQuestion: mainQuestion, CurrentQuestion: mainQuestion, LatestAnswer: summary, Questions: specs, Answers: resolved})
	if err != nil {
		emitLLMError(emitter, sessionID, err)
		return
	}
	judgement := consumeJudgeStream(sessionID, events, emitter)
	if judgement == nil {
		return
	}
	result := s.applyJudgementAndSave(ctx, session, summary, judgement, resolved)
	emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventResult, Result: result})
	emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventDone})
}

// SubmitFollowUpQuestions 使用统一问题配置处理追问的结构化答案。
func (s *SessionAppService) SubmitFollowUpQuestions(ctx context.Context, sessionID string, answers []QuestionAnswerInput) (SessionResult, error) {
	session, err := s.repo.FindByID(ctx, sessionID)
	if err != nil {
		return SessionResult{}, err
	}
	if session.Status != riskfactor.StatusProcessing {
		return SessionResult{}, riskfactor.ErrSessionNotProcessing
	}
	mainQuestion, specs, resolved, summary, err := s.prepareQuestionInput(ctx, session.UserID, session.RiskFactorType, answers)
	if err != nil {
		return SessionResult{}, err
	}
	judgement, err := s.judger.Judge(ctx, riskfactor.JudgeInput{SessionID: sessionID, RiskFactorType: session.RiskFactorType, MainQuestion: mainQuestion, History: session.History, CurrentQuestion: session.FollowUpQuestion(), LatestAnswer: summary, Questions: specs, Answers: resolved})
	if err != nil {
		return llmErrorResult(sessionID, session.RiskFactorType, session.MainQuestion, err), nil
	}
	return s.applyJudgementAndSave(ctx, session, summary, judgement, resolved), nil
}

// SubmitFollowUpQuestionsStream 是结构化追问的流式变体。
func (s *SessionAppService) SubmitFollowUpQuestionsStream(ctx context.Context, sessionID string, answers []QuestionAnswerInput, emitter StreamEmitter) {
	session, err := s.repo.FindByID(ctx, sessionID)
	if err != nil {
		emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventError, ErrorCode: "SESSION_NOT_FOUND", Message: err.Error()})
		emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventDone})
		return
	}
	if session.Status != riskfactor.StatusProcessing {
		emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventError, ErrorCode: "SESSION_NOT_PROCESSING", Message: riskfactor.ErrSessionNotProcessing.Error()})
		emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventDone})
		return
	}
	mainQuestion, specs, resolved, summary, err := s.prepareQuestionInput(ctx, session.UserID, session.RiskFactorType, answers)
	if err != nil {
		emitLLMError(emitter, sessionID, err)
		return
	}
	events, err := s.judger.JudgeStream(ctx, riskfactor.JudgeInput{SessionID: sessionID, RiskFactorType: session.RiskFactorType, MainQuestion: mainQuestion, History: session.History, CurrentQuestion: session.FollowUpQuestion(), LatestAnswer: summary, Questions: specs, Answers: resolved})
	if err != nil {
		emitLLMError(emitter, sessionID, err)
		return
	}
	judgement := consumeJudgeStream(sessionID, events, emitter)
	if judgement == nil {
		return
	}
	result := s.applyJudgementAndSave(ctx, session, summary, judgement, resolved)
	emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventResult, Result: result})
	emitter.Emit(StreamEvent{SessionID: sessionID, Type: StreamEventDone})
}

func (s *SessionAppService) prepareQuestionInput(ctx context.Context, userID string, riskFactorType riskfactor.RiskFactorType, answers []QuestionAnswerInput) (string, []riskfactor.QuestionSpec, []riskfactor.QuestionAnswer, string, error) {
	if s.catalog == nil {
		return "", nil, nil, "", errors.New("question catalog is not configured")
	}
	trees, err := s.catalog.FindQuestionTrees(ctx, []string{string(riskFactorType)})
	if err != nil {
		return "", nil, nil, "", err
	}
	tree, ok := trees[string(riskFactorType)]
	if !ok || tree.Root.AnswerType != "group" {
		return "", nil, nil, "", fmt.Errorf("question tree not found for risk factor %s", riskFactorType)
	}
	byKey, specs := buildQuestionSpecs(tree.Root.Children)
	resolved, summaries, err := s.resolveQuestionAnswers(ctx, userID, riskFactorType, byKey, answers)
	if err != nil {
		return "", nil, nil, "", err
	}
	sort.Strings(summaries)
	return tree.Root.QuestionText, specs, resolved, strings.Join(summaries, "\n"), nil
}

func buildQuestionSpecs(questions []QuestionNode) (map[string]QuestionNode, []riskfactor.QuestionSpec) {
	byKey := make(map[string]QuestionNode, len(questions))
	specs := make([]riskfactor.QuestionSpec, 0, len(questions))
	for _, question := range questions {
		byKey[question.QuestionKey] = question
		rules := make([]string, 0, len(question.Skills))
		for _, skill := range question.Skills {
			rules = append(rules, skill.RuleText)
		}
		specs = append(specs, riskfactor.QuestionSpec{QuestionKey: question.QuestionKey, QuestionText: question.QuestionText, AnswerType: question.AnswerType, Required: question.Required, MinSubmitCount: question.MinSubmitCount, MaxSubmitCount: question.MaxSubmitCount, Rules: rules})
	}
	return byKey, specs
}

func (s *SessionAppService) resolveQuestionAnswers(ctx context.Context, userID string, riskFactorType riskfactor.RiskFactorType, byKey map[string]QuestionNode, answers []QuestionAnswerInput) ([]riskfactor.QuestionAnswer, []string, error) {
	resolved := make([]riskfactor.QuestionAnswer, 0, len(answers))
	summaries := make([]string, 0, len(answers))
	seenQuestions := make(map[string]struct{}, len(answers))
	seenFiles := make(map[string]struct{})
	for _, answer := range answers {
		if _, duplicate := seenQuestions[answer.QuestionKey]; duplicate {
			return nil, nil, fmt.Errorf("duplicate question_key %s", answer.QuestionKey)
		}
		seenQuestions[answer.QuestionKey] = struct{}{}
		question, exists := byKey[answer.QuestionKey]
		if !exists {
			return nil, nil, fmt.Errorf("unknown or disabled question_key %s", answer.QuestionKey)
		}
		item, summary, err := s.resolveQuestionAnswer(ctx, userID, riskFactorType, question, answer, seenFiles)
		if err != nil {
			return nil, nil, err
		}
		resolved = append(resolved, item)
		summaries = append(summaries, summary)
	}
	return resolved, summaries, nil
}

func (s *SessionAppService) resolveQuestionAnswer(ctx context.Context, userID string, riskFactorType riskfactor.RiskFactorType, question QuestionNode, answer QuestionAnswerInput, seenFiles map[string]struct{}) (riskfactor.QuestionAnswer, string, error) {
	item := riskfactor.QuestionAnswer{QuestionKey: answer.QuestionKey, ValueType: question.AnswerType, Text: strings.TrimSpace(answer.Text), FileIDs: append([]string(nil), answer.FileIDs...)}
	if question.AnswerType == "text" {
		if item.Text == "" || len(item.FileIDs) > 0 {
			return item, "", fmt.Errorf("question %s requires text only", answer.QuestionKey)
		}
		return item, question.QuestionText + ": " + item.Text, nil
	}
	if err := s.validateFileAnswer(question, item); err != nil {
		return item, "", err
	}
	for _, fileID := range item.FileIDs {
		if _, duplicate := seenFiles[fileID]; duplicate {
			return item, "", fmt.Errorf("duplicate file_id %s", fileID)
		}
		seenFiles[fileID] = struct{}{}
		file, err := s.attachments.FindOwned(ctx, fileID, userID, string(riskFactorType), answer.QuestionKey)
		if err != nil {
			return item, "", fmt.Errorf("invalid file %s for question %s: %w", fileID, answer.QuestionKey, err)
		}
		path, err := secureStoredPath(s.storageRoot, file.StoredPath)
		if err != nil {
			return item, "", err
		}
		item.ImagePaths = append(item.ImagePaths, path)
	}
	return item, fmt.Sprintf("%s: 已提交%d个文件", question.QuestionText, len(item.FileIDs)), nil
}

func (s *SessionAppService) validateFileAnswer(question QuestionNode, item riskfactor.QuestionAnswer) error {
	if item.Text != "" || len(item.FileIDs) < question.MinSubmitCount {
		return fmt.Errorf("question %s requires at least %d file(s) when submitted", item.QuestionKey, question.MinSubmitCount)
	}
	maxFiles := question.MaxSubmitCount
	if maxFiles <= 0 || s.maxFilesPerQuestion > 0 && s.maxFilesPerQuestion < maxFiles {
		maxFiles = s.maxFilesPerQuestion
	}
	if maxFiles > 0 && len(item.FileIDs) > maxFiles {
		return fmt.Errorf("question %s exceeds maximum of %d files", item.QuestionKey, maxFiles)
	}
	if s.attachments == nil {
		return errors.New("attachment repository is not configured")
	}
	return nil
}

func secureStoredPath(root, storedPath string) (string, error) {
	if root == "" {
		return "", errors.New("storage root is not configured")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate, err := filepath.Abs(filepath.Join(absoluteRoot, filepath.Clean(storedPath)))
	if err != nil {
		return "", err
	}
	if candidate != absoluteRoot && !strings.HasPrefix(candidate, absoluteRoot+string(filepath.Separator)) {
		return "", errors.New("stored file path escapes storage root")
	}
	return candidate, nil
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
func (s *SessionAppService) applyJudgementAndSave(ctx context.Context, session *riskfactor.RiskFactorSession, answer string, judgement *riskfactor.JudgementResult, structuredAnswers ...[]riskfactor.QuestionAnswer) SessionResult {
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
	if len(structuredAnswers) > 0 && len(session.History) > 0 {
		session.History[len(session.History)-1].Answers = append([]riskfactor.QuestionAnswer(nil), structuredAnswers[0]...)
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
	var judgements []riskfactor.QuestionJudgement
	var missing []string
	if len(s.History) > 0 {
		judgements = append([]riskfactor.QuestionJudgement(nil), s.History[len(s.History)-1].Judgements...)
		for _, item := range judgements {
			if item.Required && !item.Completeness {
				missing = append(missing, item.QuestionKey)
			}
		}
	}
	switch s.Status {
	case riskfactor.StatusCleared:
		v := true
		cleared = &v
	case riskfactor.StatusNotCleared:
		v := false
		cleared = &v
	}
	return SessionResult{
		SessionID:           s.ID,
		RiskFactorType:      s.RiskFactorType,
		MainQuestion:        s.MainQuestion,
		Status:              s.Status,
		CurrentRound:        s.CurrentRound,
		MaxRounds:           s.MaxRounds,
		Message:             s.UserMessage(),
		Cleared:             cleared,
		TerminationReason:   s.TerminationReason,
		ExtractedInfo:       s.ExtractedInfo,
		MissingQuestionKeys: missing,
		QuestionJudgements:  judgements,
		History:             s.History,
	}
}
