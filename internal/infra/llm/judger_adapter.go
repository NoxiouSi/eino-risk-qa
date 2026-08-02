package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

// ErrNoToolCall 模型未按预期返回工具调用。
var ErrNoToolCall = errors.New("llm: model did not return expected tool call")

// ErrMaxRetriesExceeded 重试后仍未拿到可解析的结构化结果。
var ErrMaxRetriesExceeded = errors.New("llm: max retries exceeded")

// ErrVisionProviderRequired 表示图片回答没有可用的视觉模型。
var ErrVisionProviderRequired = errors.New("llm: image answers require a vision-capable provider")

// ErrRequestTimeout 表示模型在允许时间内未完成判断。
var ErrRequestTimeout = errors.New("llm: request timed out")

const (
	// DefaultMaxRetries 结构化输出解析失败时的默认重试次数（不含首次调用）。
	DefaultMaxRetries = 1
	// DefaultRequestTimeout 同步和流式模型判断的默认总超时。
	DefaultRequestTimeout = 5 * time.Minute
)

// JudgerAdapter 实现 domain.RiskJudger 端口：组合 ChatModel（通过 factory 可插拔获取）、
// Prompt 构建、Tool Schema 绑定与结构化输出解析，含失败重试。
type JudgerAdapter struct {
	chatModel        model.ToolCallingChatModel
	visionModel      model.ToolCallingChatModel
	primaryHasVision bool
	maxRetries       int
	requestTimeout   time.Duration
}

// NewJudgerAdapter 创建适配器；chatModel 应已通过 factory.NewToolCallingChatModel 构造。
func NewJudgerAdapter(chatModel model.ToolCallingChatModel) *JudgerAdapter {
	return &JudgerAdapter{chatModel: chatModel, maxRetries: DefaultMaxRetries, requestTimeout: DefaultRequestTimeout}
}

// ConfigurePrimaryVisionSupport 声明主模型是否支持真实图片输入。
func (a *JudgerAdapter) ConfigurePrimaryVisionSupport(supported bool) {
	a.primaryHasVision = supported
}

// ConfigureVisionModel 设置图片审核专用模型。
func (a *JudgerAdapter) ConfigureVisionModel(visionModel model.ToolCallingChatModel) {
	a.visionModel = visionModel
}

// ConfigureRequestTimeout 设置单次判断（包括重试和流读取）的总超时；非正值恢复默认值。
func (a *JudgerAdapter) ConfigureRequestTimeout(timeout time.Duration) {
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	a.requestTimeout = timeout
}

func (a *JudgerAdapter) modelFor(input riskfactor.JudgeInput) (model.ToolCallingChatModel, error) {
	if !hasImageAnswers(input) {
		return a.chatModel, nil
	}
	if a.visionModel != nil {
		return a.visionModel, nil
	}
	if a.primaryHasVision {
		return a.chatModel, nil
	}
	return nil, ErrVisionProviderRequired
}

var _ riskfactor.RiskJudger = (*JudgerAdapter)(nil)

// Judge 同步判断：绑定工具、调用 Generate、解析工具调用参数为 JudgementResult；解析失败时重试。
func (a *JudgerAdapter) Judge(ctx context.Context, input riskfactor.JudgeInput) (*riskfactor.JudgementResult, error) {
	log := logging.FromContext(ctx).With("session_id", input.SessionID, "risk_factor_type", string(input.RiskFactorType))
	chatModel, err := a.modelFor(input)
	if err != nil {
		log.Error("judge: select chat model failed", "error", err.Error())
		return nil, err
	}
	toolModel, err := chatModel.WithTools([]*schema.ToolInfo{judgementToolInfo()})
	if err != nil {
		log.Error("judge: bind tools failed", "error", err.Error())
		return nil, err
	}
	messages, err := BuildMessages(input)
	if err != nil {
		log.Error("judge: build messages failed", "error", err.Error())
		return nil, err
	}

	requestCtx, cancel := context.WithTimeout(ctx, a.requestTimeout)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt <= a.maxRetries; attempt++ {
		log.Debug("judge: calling chat model", "attempt", attempt, "timeout", a.requestTimeout.String())
		msg, err := toolModel.Generate(requestCtx, messages)
		if err != nil {
			if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
				log.Error("judge: chat model request timed out", "timeout", a.requestTimeout.String())
				return nil, fmt.Errorf("%w after %s", ErrRequestTimeout, a.requestTimeout)
			}
			log.Warn("judge: chat model generate failed", "attempt", attempt, "error", err.Error())
			lastErr = err
			continue
		}
		result, err := parseJudgementForInput(msg, input)
		if err != nil {
			log.Warn("judge: parse tool call result failed", "attempt", attempt, "error", err.Error())
			lastErr = err
			continue
		}
		log.Info("judge: succeeded", "attempt", attempt, "completeness", result.Completeness, "reasonableness", result.Reasonableness)
		return result, nil
	}
	log.Error("judge: max retries exceeded", "max_retries", a.maxRetries, "last_error", lastErr)
	return nil, fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
}

func parseJudgementForInput(msg *schema.Message, input riskfactor.JudgeInput) (*riskfactor.JudgementResult, error) {
	result, err := parseJudgementFromMessage(msg, input.Questions)
	if err != nil || len(input.Questions) == 0 || len(input.Answers) == 0 {
		return result, err
	}

	merged := make(map[string]riskfactor.QuestionJudgement, len(input.Questions))
	for _, qa := range input.History {
		for _, judgement := range qa.Judgements {
			merged[judgement.QuestionKey] = judgement
		}
	}
	submitted := make(map[string]struct{}, len(input.Answers))
	for _, answer := range input.Answers {
		submitted[answer.QuestionKey] = struct{}{}
		delete(merged, answer.QuestionKey)
	}
	for _, judgement := range result.Questions {
		if _, ok := submitted[judgement.QuestionKey]; ok {
			merged[judgement.QuestionKey] = judgement
		}
	}
	items := make([]riskfactor.QuestionJudgement, 0, len(merged))
	for _, spec := range input.Questions {
		if judgement, ok := merged[spec.QuestionKey]; ok {
			items = append(items, judgement)
		}
	}
	return riskfactor.AggregateJudgement(input.Questions, items, result.ExtractedInfo, result.ReasoningSummary, result.FollowUpQuestion), nil
}

// parseJudgementFromMessage 从模型返回的消息中提取第一个工具调用并反序列化为 JudgementResult。
func parseJudgementFromMessage(msg *schema.Message, specs []riskfactor.QuestionSpec) (*riskfactor.JudgementResult, error) {
	if msg == nil || len(msg.ToolCalls) == 0 {
		return nil, ErrNoToolCall
	}
	tc := msg.ToolCalls[0]
	var args judgementArgs
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return nil, fmt.Errorf("llm: parse tool call arguments failed: %w", err)
	}
	if len(specs) == 0 {
		return &riskfactor.JudgementResult{Completeness: args.Completeness, Reasonableness: args.Reasonableness, FollowUpQuestion: args.FollowUpQuestion, ExtractedInfo: args.ExtractedInfo, ReasoningSummary: args.ReasoningSummary}, nil
	}
	items := make([]riskfactor.QuestionJudgement, 0, len(args.Items))
	for _, item := range args.Items {
		items = append(items, riskfactor.QuestionJudgement{QuestionKey: item.QuestionKey, Completeness: item.Completeness, Reasonableness: item.Reasonableness, Note: item.Note})
	}
	return riskfactor.AggregateJudgement(specs, items, args.ExtractedInfo, args.ReasoningSummary, args.FollowUpQuestion), nil
}
