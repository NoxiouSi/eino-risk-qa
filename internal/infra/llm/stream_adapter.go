package llm

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/cloudwego/eino/schema"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

// JudgeStream 流式判断：基于 ChatModel.Stream() 获取工具调用参数的增量字符串，通过内置的
// ArgumentScanner 提取 follow_up_question 字段的部分值并转发为 message_delta 事件；
// 流结束后累积拼接完整参数，反序列化得到最终 JudgementResult 并发出 Result 事件。
//
// 若单个风险要素到达终态（completeness=true），follow_up_question 为空，不发送 message_delta；
// 批次收尾必须等待全部风险要素结束后由聚合状态统一决定。
func (a *JudgerAdapter) JudgeStream(ctx context.Context, input riskfactor.JudgeInput) (<-chan riskfactor.JudgeStreamEvent, error) {
	log := logging.FromContext(ctx).With("session_id", input.SessionID)
	chatModel, err := a.modelFor(input)
	if err != nil {
		log.Error("judge stream: select chat model failed", "error", err.Error())
		return nil, err
	}

	// L1 攻击检测：流式场景同样在 BuildMessages 前执行
	if a.attackDetector != nil {
		userInput := extractUserInputText(input)
		detectResult, detectErr := a.attackDetector.Detect(ctx, userInput)
		if detectErr != nil && errors.Is(detectErr, ErrAttackDetected) {
			log.Warn(
				"judge stream: attack intent detected, rejecting as 'review not passed'",
				"confidence", detectResult.Confidence,
				"reason", detectResult.Reason,
			)
			events := make(chan riskfactor.JudgeStreamEvent, 1)
			events <- riskfactor.JudgeStreamEvent{
				SessionID:      input.SessionID,
				RiskFactorType: input.RiskFactorType,
				Type:           riskfactor.StreamEventResult,
				Result: &riskfactor.JudgementResult{
					AttackDetected:   true,
					Completeness:     true,
					Reasonableness:   false,
					ReasoningSummary: fmt.Sprintf("attack_detected: %s", detectResult.Reason),
				},
			}
			close(events)
			return events, nil
		}
		if detectErr != nil {
			log.Warn("judge stream: attack detection non-critical error, continuing", "error", detectErr.Error())
		}
	}
	toolModel, err := chatModel.WithTools([]*schema.ToolInfo{judgementToolInfo()})
	if err != nil {
		log.Error("judge stream: bind tools failed", "error", err.Error())
		return nil, err
	}
	messages, err := BuildMessages(input)
	if err != nil {
		log.Error("judge stream: build messages failed", "error", err.Error())
		return nil, err
	}

	requestCtx, cancel := context.WithTimeout(ctx, a.requestTimeout)
	sr, err := toolModel.Stream(requestCtx, messages)
	if err != nil {
		cancel()
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			log.Error("judge stream: chat model request timed out", "timeout", a.requestTimeout.String())
			return nil, ErrRequestTimeout
		}
		log.Error("judge stream: chat model stream failed", "error", err.Error())
		return nil, err
	}

	events := make(chan riskfactor.JudgeStreamEvent, 16)
	go a.consumeStream(requestCtx, cancel, sr, input, events)
	return events, nil
}

func (a *JudgerAdapter) consumeStream(ctx context.Context, cancel context.CancelFunc, sr *schema.StreamReader[*schema.Message], input riskfactor.JudgeInput, events chan riskfactor.JudgeStreamEvent) {
	sessionID := input.SessionID
	log := logging.FromContext(ctx).With("session_id", sessionID)
	defer cancel()
	defer close(events)
	defer sr.Close()

	scanner := NewArgumentScanner()
	deltaCount := 0

	for {
		chunk, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				err = ErrRequestTimeout
				log.Error("judge stream: receive timed out", "timeout", a.requestTimeout.String())
			} else {
				log.Error("judge stream: recv chunk failed", "error", err.Error())
			}
			events <- riskfactor.JudgeStreamEvent{SessionID: sessionID, RiskFactorType: input.RiskFactorType, Type: riskfactor.StreamEventError, Err: err}
			return
		}
		if len(chunk.ToolCalls) == 0 {
			continue
		}
		delta := scanner.Feed(chunk.ToolCalls[0].Function.Arguments)
		if delta != "" {
			// 流式增量安全审查：拦截包含注入模式或敏感信息的 delta
			safeDelta := SanitizeStreamDelta(delta)
			if safeDelta != "" {
				deltaCount++
				events <- riskfactor.JudgeStreamEvent{SessionID: sessionID, RiskFactorType: input.RiskFactorType, Type: riskfactor.StreamEventMessageDelta, MessageDelta: safeDelta}
			}
		}
	}
	log.Debug("judge stream: stream ended", "delta_count", deltaCount)

	msg := &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{
			{Function: schema.FunctionCall{Name: judgementToolName, Arguments: scanner.FullArguments()}},
		},
	}
	result, err := parseJudgementForInput(msg, input)
	if err != nil {
		log.Error("judge stream: parse final result failed", "error", err.Error())
		events <- riskfactor.JudgeStreamEvent{SessionID: sessionID, RiskFactorType: input.RiskFactorType, Type: riskfactor.StreamEventError, Err: err}
		return
	}
	log.Info("judge stream: succeeded", "completeness", result.Completeness, "reasonableness", result.Reasonableness)

	events <- riskfactor.JudgeStreamEvent{SessionID: sessionID, RiskFactorType: input.RiskFactorType, Type: riskfactor.StreamEventResult, Result: result}
}
