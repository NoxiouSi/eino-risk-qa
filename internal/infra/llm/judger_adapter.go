package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
// 内置 AttackDetector 在 BuildMessages 前检测攻击意图，攻击时返回合成 JudgementResult 按"审核不通过"处理。
type JudgerAdapter struct {
	chatModel        model.ToolCallingChatModel
	visionModel      model.ToolCallingChatModel
	primaryHasVision bool
	maxRetries       int
	requestTimeout   time.Duration
	attackDetector   *AttackDetector
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

// ConfigureAttackDetector 设置攻击判别器；为 nil 时跳过攻击检测。
func (a *JudgerAdapter) ConfigureAttackDetector(detector *AttackDetector) {
	a.attackDetector = detector
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
// 攻击检测在 BuildMessages 之前执行；命中攻击时直接返回合成的"审核不通过"JudgementResult。
func (a *JudgerAdapter) Judge(ctx context.Context, input riskfactor.JudgeInput) (*riskfactor.JudgementResult, error) {
	log := logging.FromContext(ctx).With("session_id", input.SessionID, "risk_factor_type", string(input.RiskFactorType))
	chatModel, err := a.modelFor(input)
	if err != nil {
		log.Error("judge: select chat model failed", "error", err.Error())
		return nil, err
	}

	// L1 攻击检测：在 BuildMessages 之前执行，检测原始用户输入
	if a.attackDetector != nil {
		userInput := extractUserInputText(input)
		detectResult, detectErr := a.attackDetector.Detect(ctx, userInput)
		if detectErr != nil && errors.Is(detectErr, ErrAttackDetected) {
			log.Warn(
				"judge: attack intent detected, rejecting as 'review not passed'",
				"confidence", detectResult.Confidence,
				"reason", detectResult.Reason,
			)
			return &riskfactor.JudgementResult{
				AttackDetected:   true,
				Completeness:     true,
				Reasonableness:   false,
				ReasoningSummary: fmt.Sprintf("attack_detected: %s", detectResult.Reason),
			}, nil
		}
		// 非攻击检测错误（判别器关闭/mock等）不阻塞，继续正常流程
		if detectErr != nil {
			log.Warn("judge: attack detection non-critical error, continuing", "error", detectErr.Error())
		}
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
// 所有从 LLM 获取的文本均经过安全处理（脱敏 + 追问安全审查），防止数据泄露和恶意输出。
func parseJudgementFromMessage(msg *schema.Message, specs []riskfactor.QuestionSpec) (*riskfactor.JudgementResult, error) {
	if msg == nil || len(msg.ToolCalls) == 0 {
		return nil, ErrNoToolCall
	}
	tc := msg.ToolCalls[0]
	var args judgementArgs
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return nil, fmt.Errorf("llm: parse tool call arguments failed: %w", err)
	}

	// 输出安全防护：
	// 1. extracted_info 脱敏 —— 防止 LLM 输出完整的身份证号、手机号、银行卡号等
	args.ExtractedInfo = DesensitizeExtractedInfo(args.ExtractedInfo)
	// 2. follow_up_question 安全审查 —— 拦截包含注入模式或敏感信息的追问文本
	args.FollowUpQuestion = SanitizeFollowUpQuestion(args.FollowUpQuestion)
	// 3. 逐问题 Note 脱敏
	for i := range args.Items {
		if args.Items[i].Note != "" {
			args.Items[i].Note = DesensitizeText(args.Items[i].Note)
		}
	}

	if len(specs) == 0 {
		return &riskfactor.JudgementResult{Completeness: args.Completeness, Reasonableness: args.Reasonableness, FollowUpQuestion: args.FollowUpQuestion, ExtractedInfo: args.ExtractedInfo, ReasoningSummary: args.ReasoningSummary}, nil
	}
	items := make([]riskfactor.QuestionJudgement, 0, len(args.Items))
	for _, item := range args.Items {
		items = append(items, riskfactor.QuestionJudgement{QuestionKey: item.QuestionKey, Completeness: item.Completeness, Reasonableness: item.Reasonableness, Note: item.Note})
	}
	result := riskfactor.AggregateJudgement(specs, items, args.ExtractedInfo, args.ReasoningSummary, args.FollowUpQuestion)
	// 二次脱敏 —— AggregateJudgement 可能重新组装 extracted_info
	result.ExtractedInfo = DesensitizeExtractedInfo(result.ExtractedInfo)
	return result, nil
}

// extractUserInputText 从 JudgeInput 中提取原始用户输入文本（拼接所有回答），供攻击判别器使用。
func extractUserInputText(input riskfactor.JudgeInput) string {
	var parts []string
	for _, a := range input.Answers {
		if a.Text != "" {
			parts = append(parts, a.Text)
		}
	}
	return strings.Join(parts, "\n")
}
