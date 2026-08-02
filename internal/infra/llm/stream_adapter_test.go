package llm_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
	"github.com/NoxiouSi/eino-risk-qa/internal/infra/llm"
)

// 追问场景（completeness=false）：验证 follow_up_question 真正逐字流式到达（多个 message_delta 事件
// 拼接后等于最终 Result.FollowUpQuestion），且最后一定以 Result 事件收尾。
func TestJudgerAdapter_JudgeStream_Incomplete_StreamsFollowUpQuestionIncrementally(t *testing.T) {
	adapter := llm.NewJudgerAdapter(llm.NewMockChatModel())

	events, err := adapter.JudgeStream(context.Background(), riskfactor.JudgeInput{
		SessionID:       "sess_1",
		RiskFactorType:  riskfactor.RiskFactorTypeIdentity,
		MainQuestion:    "请说明您的身份信息及职业背景",
		CurrentQuestion: "请说明您的身份信息及职业背景",
		LatestAnswer:    "财务经理", // 短回答 -> completeness=false
	})
	require.NoError(t, err)

	var deltas []string
	var result *riskfactor.JudgementResult
	deltaCount := 0
	for ev := range events {
		switch ev.Type {
		case riskfactor.StreamEventMessageDelta:
			deltas = append(deltas, ev.MessageDelta)
			deltaCount++
			assert.Equal(t, "sess_1", ev.SessionID)
		case riskfactor.StreamEventResult:
			result = ev.Result
			assert.Equal(t, "sess_1", ev.SessionID)
		case riskfactor.StreamEventError:
			t.Fatalf("unexpected error event: %v", ev.Err)
		}
	}

	require.NotNil(t, result)
	assert.False(t, result.Completeness)
	require.Greater(t, deltaCount, 1, "追问问题应分多个片段逐字流出，而不是一次性给出")

	var joined string
	for _, d := range deltas {
		joined += d
	}
	assert.Equal(t, result.FollowUpQuestion, joined)
}

// 终态场景（completeness=true）不产生 message_delta；批次收尾由聚合全部会话状态后展示。
func TestJudgerAdapter_JudgeStream_Complete_DoesNotEmitBatchClosingMessage(t *testing.T) {
	adapter := llm.NewJudgerAdapter(llm.NewMockChatModel())

	events, err := adapter.JudgeStream(context.Background(), riskfactor.JudgeInput{
		SessionID:       "sess_2",
		RiskFactorType:  riskfactor.RiskFactorTypeIdentity,
		MainQuestion:    "请说明您的身份信息及职业背景",
		CurrentQuestion: "请说明您的身份信息及职业背景",
		LatestAnswer:    "我是XX科技有限公司的财务经理，主要负责公司资金调拨与预算审批工作",
	})
	require.NoError(t, err)

	var deltaEvents []riskfactor.JudgeStreamEvent
	var result *riskfactor.JudgementResult
	for ev := range events {
		if ev.Type == riskfactor.StreamEventMessageDelta {
			deltaEvents = append(deltaEvents, ev)
		}
		if ev.Type == riskfactor.StreamEventResult {
			result = ev.Result
		}
	}

	require.NotNil(t, result)
	assert.True(t, result.Completeness)
	assert.Empty(t, deltaEvents, "单个风险要素完成时不得输出批次收尾文案")
}

// channel 必须在所有事件发出后正常关闭（for range 能自然退出，不需要额外的 done 信号）。
func TestJudgerAdapter_JudgeStream_ChannelClosesAfterResult(t *testing.T) {
	adapter := llm.NewJudgerAdapter(llm.NewMockChatModel())

	events, err := adapter.JudgeStream(context.Background(), riskfactor.JudgeInput{
		SessionID:      "sess_3",
		RiskFactorType: riskfactor.RiskFactorTypeIdentity,
		MainQuestion:   "主问题",
		LatestAnswer:   "短",
	})
	require.NoError(t, err)

	sawResult := false
	for ev := range events {
		if ev.Type == riskfactor.StreamEventResult {
			sawResult = true
		}
	}
	assert.True(t, sawResult, "channel 关闭前必须发出过 Result 事件")
}
