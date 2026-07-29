package llm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
	"github.com/NoxiouSi/eino-risk-qa/internal/infra/llm"
)

func TestJudgerAdapter_Judge_WithMockChatModel_Incomplete(t *testing.T) {
	adapter := llm.NewJudgerAdapter(llm.NewMockChatModel())

	result, err := adapter.Judge(context.Background(), riskfactor.JudgeInput{
		RiskFactorType:  riskfactor.RiskFactorTypeIdentity,
		MainQuestion:    "请说明您的身份信息及职业背景",
		CurrentQuestion: "请说明您的身份信息及职业背景",
		LatestAnswer:    "财务经理", // 短回答，mock规则判定为不完整
	})

	require.NoError(t, err)
	assert.False(t, result.Completeness)
	assert.NotEmpty(t, result.FollowUpQuestion)
}

func TestJudgerAdapter_Judge_WithMockChatModel_CompleteAndReasonable(t *testing.T) {
	adapter := llm.NewJudgerAdapter(llm.NewMockChatModel())

	result, err := adapter.Judge(context.Background(), riskfactor.JudgeInput{
		RiskFactorType:  riskfactor.RiskFactorTypeIdentity,
		MainQuestion:    "请说明您的身份信息及职业背景",
		CurrentQuestion: "请说明您的身份信息及职业背景",
		LatestAnswer:    "我是XX科技有限公司的财务经理，主要负责公司资金调拨与预算审批工作",
	})

	require.NoError(t, err)
	assert.True(t, result.Completeness)
	assert.True(t, result.Reasonableness)
	assert.Empty(t, result.FollowUpQuestion)
}

func TestJudgerAdapter_Judge_UnreasonableKeyword(t *testing.T) {
	adapter := llm.NewJudgerAdapter(llm.NewMockChatModel())

	result, err := adapter.Judge(context.Background(), riskfactor.JudgeInput{
		RiskFactorType:  riskfactor.RiskFactorTypeFundSource,
		MainQuestion:    "请说明本次资金的来源",
		CurrentQuestion: "请说明本次资金的来源",
		LatestAnswer:    "不知道，反正就是有一大笔钱，说不清楚具体来源渠道",
	})

	require.NoError(t, err)
	assert.True(t, result.Completeness)    // 回答足够长
	assert.False(t, result.Reasonableness) // 含"不知道"关键词
}

// brokenToolCallModel 始终返回无法解析的 arguments，用于验证重试与最终失败路径。
type brokenToolCallModel struct {
	calls int
}

func (m *brokenToolCallModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.calls++
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{
			{Function: schema.FunctionCall{Name: "submit_risk_judgement", Arguments: "not-json"}},
		},
	}, nil
}

func (m *brokenToolCallModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("not used in this test")
}

func (m *brokenToolCallModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func TestJudgerAdapter_Judge_RetriesOnParseFailure_ThenFails(t *testing.T) {
	broken := &brokenToolCallModel{}
	adapter := llm.NewJudgerAdapter(broken)

	_, err := adapter.Judge(context.Background(), riskfactor.JudgeInput{
		RiskFactorType: riskfactor.RiskFactorTypeIdentity,
		MainQuestion:   "主问题",
		LatestAnswer:   "回答",
	})

	assert.ErrorIs(t, err, llm.ErrMaxRetriesExceeded)
	// DefaultMaxRetries=1，应尝试 1(首次)+1(重试) = 2 次
	assert.Equal(t, 2, broken.calls)
}

// noToolCallModel 返回不含任何 ToolCalls 的消息，验证 ErrNoToolCall 路径同样会触发重试并最终失败。
type noToolCallModel struct{}

func (noToolCallModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: "抱歉我不确定"}, nil
}
func (noToolCallModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("not used")
}
func (m noToolCallModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func TestJudgerAdapter_Judge_NoToolCall_ReturnsMaxRetriesExceeded(t *testing.T) {
	adapter := llm.NewJudgerAdapter(noToolCallModel{})

	_, err := adapter.Judge(context.Background(), riskfactor.JudgeInput{
		RiskFactorType: riskfactor.RiskFactorTypeIdentity,
		MainQuestion:   "主问题",
		LatestAnswer:   "回答",
	})

	assert.ErrorIs(t, err, llm.ErrMaxRetriesExceeded)
}
