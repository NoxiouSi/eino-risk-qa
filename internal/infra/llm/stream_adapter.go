package llm

import (
	"context"
	"errors"
	"io"

	"github.com/cloudwego/eino/schema"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

// JudgeStream 流式判断：基于 ChatModel.Stream() 获取工具调用参数的增量字符串，通过内置的
// ArgumentScanner 提取 follow_up_question 字段的部分值并转发为 message_delta 事件；
// 流结束后累积拼接完整参数，反序列化得到最终 JudgementResult 并发出 Result 事件。
//
// 若判断结果为终态（completeness=true），follow_up_question 为空，不会产生任何真正的增量事件；
// 此时在拿到完整结果后，一次性发出一个 message_delta 事件，内容为领域常量 ClosingMessage
// （因为该文案是常量、无需也不必要模拟逐字过程）。
func (a *JudgerAdapter) JudgeStream(ctx context.Context, input riskfactor.JudgeInput) (<-chan riskfactor.JudgeStreamEvent, error) {
	log := logging.FromContext(ctx).With("session_id", input.SessionID)
	toolModel, err := a.chatModel.WithTools([]*schema.ToolInfo{judgementToolInfo()})
	if err != nil {
		log.Error("judge stream: bind tools failed", "error", err.Error())
		return nil, err
	}
	messages := BuildMessages(input)

	sr, err := toolModel.Stream(ctx, messages)
	if err != nil {
		log.Error("judge stream: chat model stream failed", "error", err.Error())
		return nil, err
	}

	events := make(chan riskfactor.JudgeStreamEvent, 16)
	go a.consumeStream(ctx, sr, input.SessionID, events)
	return events, nil
}

func (a *JudgerAdapter) consumeStream(ctx context.Context, sr *schema.StreamReader[*schema.Message], sessionID string, events chan riskfactor.JudgeStreamEvent) {
	log := logging.FromContext(ctx).With("session_id", sessionID)
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
			log.Error("judge stream: recv chunk failed", "error", err.Error())
			events <- riskfactor.JudgeStreamEvent{SessionID: sessionID, Type: riskfactor.StreamEventError, Err: err}
			return
		}
		if len(chunk.ToolCalls) == 0 {
			continue
		}
		delta := scanner.Feed(chunk.ToolCalls[0].Function.Arguments)
		if delta != "" {
			deltaCount++
			events <- riskfactor.JudgeStreamEvent{SessionID: sessionID, Type: riskfactor.StreamEventMessageDelta, MessageDelta: delta}
		}
	}
	log.Debug("judge stream: stream ended", "delta_count", deltaCount)

	msg := &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{
			{Function: schema.FunctionCall{Name: judgementToolName, Arguments: scanner.FullArguments()}},
		},
	}
	result, err := parseJudgementFromMessage(msg)
	if err != nil {
		log.Error("judge stream: parse final result failed", "error", err.Error())
		events <- riskfactor.JudgeStreamEvent{SessionID: sessionID, Type: riskfactor.StreamEventError, Err: err}
		return
	}
	log.Info("judge stream: succeeded", "completeness", result.Completeness, "reasonableness", result.Reasonableness)

	// completeness=true（终态）时 follow_up_question 为空，不会有增量事件产生，
	// 此处一次性发出完整的收尾话术，供前端直接渲染（规则详见 docs/DESIGN.md 流式输出设计）。
	if result.Completeness {
		events <- riskfactor.JudgeStreamEvent{SessionID: sessionID, Type: riskfactor.StreamEventMessageDelta, MessageDelta: riskfactor.ClosingMessage}
	}

	events <- riskfactor.JudgeStreamEvent{SessionID: sessionID, Type: riskfactor.StreamEventResult, Result: result}
}
