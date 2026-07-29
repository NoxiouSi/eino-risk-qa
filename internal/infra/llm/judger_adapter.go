package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

// ErrNoToolCall 模型未按预期返回工具调用。
var ErrNoToolCall = errors.New("llm: model did not return expected tool call")

// ErrMaxRetriesExceeded 重试后仍未拿到可解析的结构化结果。
var ErrMaxRetriesExceeded = errors.New("llm: max retries exceeded")

// DefaultMaxRetries 结构化输出解析失败时的默认重试次数（不含首次调用）。
const DefaultMaxRetries = 1

// JudgerAdapter 实现 domain.RiskJudger 端口：组合 ChatModel（通过 factory 可插拔获取）、
// Prompt 构建、Tool Schema 绑定与结构化输出解析，含失败重试。
type JudgerAdapter struct {
	chatModel  model.ToolCallingChatModel
	maxRetries int
}

// NewJudgerAdapter 创建适配器；chatModel 应已通过 factory.NewToolCallingChatModel 构造。
func NewJudgerAdapter(chatModel model.ToolCallingChatModel) *JudgerAdapter {
	return &JudgerAdapter{chatModel: chatModel, maxRetries: DefaultMaxRetries}
}

var _ riskfactor.RiskJudger = (*JudgerAdapter)(nil)

// Judge 同步判断：绑定工具、调用 Generate、解析工具调用参数为 JudgementResult；解析失败时重试。
func (a *JudgerAdapter) Judge(ctx context.Context, input riskfactor.JudgeInput) (*riskfactor.JudgementResult, error) {
	toolModel, err := a.chatModel.WithTools([]*schema.ToolInfo{judgementToolInfo()})
	if err != nil {
		return nil, err
	}
	messages := BuildMessages(input)

	var lastErr error
	for attempt := 0; attempt <= a.maxRetries; attempt++ {
		msg, err := toolModel.Generate(ctx, messages)
		if err != nil {
			lastErr = err
			continue
		}
		result, err := parseJudgementFromMessage(msg)
		if err != nil {
			lastErr = err
			continue
		}
		return result, nil
	}
	return nil, fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
}

// parseJudgementFromMessage 从模型返回的消息中提取第一个工具调用并反序列化为 JudgementResult。
func parseJudgementFromMessage(msg *schema.Message) (*riskfactor.JudgementResult, error) {
	if msg == nil || len(msg.ToolCalls) == 0 {
		return nil, ErrNoToolCall
	}
	tc := msg.ToolCalls[0]
	var args judgementArgs
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return nil, fmt.Errorf("llm: parse tool call arguments failed: %w", err)
	}
	return &riskfactor.JudgementResult{
		Completeness:     args.Completeness,
		Reasonableness:   args.Reasonableness,
		FollowUpQuestion: args.FollowUpQuestion,
		ExtractedInfo:    args.ExtractedInfo,
		ReasoningSummary: args.ReasoningSummary,
	}, nil
}
