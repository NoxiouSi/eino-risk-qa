package riskfactor_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

func newTestSession() *riskfactor.RiskFactorSession {
	return riskfactor.NewRiskFactorSession(
		"sess_1", "batch_1", "user_1",
		riskfactor.RiskFactorTypeIdentity,
		"请说明您的身份信息及职业背景",
	)
}

func TestNewRiskFactorSession_InitialState(t *testing.T) {
	s := newTestSession()

	assert.Equal(t, riskfactor.StatusProcessing, s.Status)
	assert.Equal(t, 0, s.CurrentRound)
	assert.Equal(t, riskfactor.DefaultMaxRounds, s.MaxRounds)
	assert.Nil(t, s.TerminationReason)
	assert.Empty(t, s.History)
	assert.Empty(t, s.ExtractedInfo)
	// 尚未提交任何回答，UserMessage 在 Processing 态应为空（还没有追问问题）
	assert.Equal(t, "", s.UserMessage())
}

// completeness=true & reasonableness=true -> Cleared，立即结束，UserMessage 为收尾话术
func TestSubmitInitialAnswer_CompleteAndReasonable_Cleared(t *testing.T) {
	s := newTestSession()

	err := s.SubmitInitialAnswer("我是XX公司的财务经理，入职于2020年", &riskfactor.JudgementResult{
		Completeness:   true,
		Reasonableness: true,
		ExtractedInfo:  map[string]interface{}{"occupation": "财务经理"},
	})

	require.NoError(t, err)
	assert.Equal(t, riskfactor.StatusCleared, s.Status)
	assert.Nil(t, s.TerminationReason)
	assert.Equal(t, riskfactor.ClosingMessage, s.UserMessage())
	assert.Equal(t, "财务经理", s.ExtractedInfo["occupation"])
	require.Len(t, s.History, 1)
	assert.Equal(t, 0, s.History[0].Round)
	assert.True(t, s.History[0].Completeness)
	assert.True(t, s.History[0].Reasonableness)
	// Cleared 是终态，round 不再增加
	assert.Equal(t, 0, s.CurrentRound)
}

// completeness=true & reasonableness=false -> NotCleared(unreasonable)，即使信息完整也不再追问
func TestSubmitInitialAnswer_CompleteButUnreasonable_NotCleared(t *testing.T) {
	s := newTestSession()

	err := s.SubmitInitialAnswer("一些自相矛盾的回答", &riskfactor.JudgementResult{
		Completeness:   true,
		Reasonableness: false,
	})

	require.NoError(t, err)
	assert.Equal(t, riskfactor.StatusNotCleared, s.Status)
	require.NotNil(t, s.TerminationReason)
	assert.Equal(t, riskfactor.TerminationReasonUnreasonable, *s.TerminationReason)
	assert.Equal(t, riskfactor.ClosingMessage, s.UserMessage())
}

// completeness=false & round<MaxRounds -> 继续 Processing，round+1，记录追问问题
func TestSubmitInitialAnswer_Incomplete_ContinuesProcessing(t *testing.T) {
	s := newTestSession()

	err := s.SubmitInitialAnswer("我是财务经理", &riskfactor.JudgementResult{
		Completeness:     false,
		Reasonableness:   true,
		FollowUpQuestion: "您提到的职业背景中，具体的任职时间是？",
		ExtractedInfo:    map[string]interface{}{"occupation": "财务经理"},
	})

	require.NoError(t, err)
	assert.Equal(t, riskfactor.StatusProcessing, s.Status)
	assert.Equal(t, 1, s.CurrentRound)
	assert.Equal(t, "您提到的职业背景中，具体的任职时间是？", s.UserMessage())
	assert.Equal(t, "您提到的职业背景中，具体的任职时间是？", s.FollowUpQuestion())
}

// 完整性满足即结束追问循环，不再看 reasonableness 是否继续追问——即使此前几轮 reasonableness 一直是 true，
// 只要某一轮 completeness=true，无论 reasonableness 是 true 还是 false 都立即终止（不会因为 reasonableness
// 需要"进一步确认"而继续追问）。此处验证第2轮 completeness=true 时立即终止。
func TestFollowUpLoop_EndsAssoonAsCompletenessSatisfied(t *testing.T) {
	s := newTestSession()

	require.NoError(t, s.SubmitInitialAnswer("我是财务经理", &riskfactor.JudgementResult{
		Completeness:     false,
		Reasonableness:   true,
		FollowUpQuestion: "任职时间是？",
	}))
	assert.Equal(t, riskfactor.StatusProcessing, s.Status)
	assert.Equal(t, 1, s.CurrentRound)

	err := s.SubmitFollowUpAnswer("2020年至今", &riskfactor.JudgementResult{
		Completeness:   true,
		Reasonableness: true,
		ExtractedInfo:  map[string]interface{}{"tenure": "2020年至今"},
	})

	require.NoError(t, err)
	assert.Equal(t, riskfactor.StatusCleared, s.Status)
	assert.Equal(t, riskfactor.ClosingMessage, s.UserMessage())
	require.Len(t, s.History, 2)
	assert.Equal(t, 1, s.History[1].Round)
	assert.Equal(t, "任职时间是？", s.History[1].Question)
	assert.Equal(t, "2020年至今", s.History[1].Answer)
}

// 连续3轮 completeness=false，第3轮（round已达MaxRounds）时应终止为 NotCleared(max_rounds_incomplete)，
// 且不再追问（round 不超过 MaxRounds）。
func TestFollowUpLoop_MaxRoundsReached_NotClearedMaxRounds(t *testing.T) {
	s := newTestSession()

	require.NoError(t, s.SubmitInitialAnswer("回答0", &riskfactor.JudgementResult{
		Completeness: false, Reasonableness: true, FollowUpQuestion: "追问1",
	}))
	assert.Equal(t, 1, s.CurrentRound)

	require.NoError(t, s.SubmitFollowUpAnswer("回答1", &riskfactor.JudgementResult{
		Completeness: false, Reasonableness: true, FollowUpQuestion: "追问2",
	}))
	assert.Equal(t, 2, s.CurrentRound)
	assert.Equal(t, riskfactor.StatusProcessing, s.Status)

	require.NoError(t, s.SubmitFollowUpAnswer("回答2", &riskfactor.JudgementResult{
		Completeness: false, Reasonableness: true, FollowUpQuestion: "追问3",
	}))
	assert.Equal(t, 3, s.CurrentRound)
	assert.Equal(t, riskfactor.StatusProcessing, s.Status)

	// 第3轮追问的回答判断仍然 completeness=false，此时 CurrentRound(3) 已达 MaxRounds(3)，应终止
	err := s.SubmitFollowUpAnswer("回答3", &riskfactor.JudgementResult{
		Completeness: false, Reasonableness: true,
	})

	require.NoError(t, err)
	assert.Equal(t, riskfactor.StatusNotCleared, s.Status)
	require.NotNil(t, s.TerminationReason)
	assert.Equal(t, riskfactor.TerminationReasonMaxRoundsIncomplete, *s.TerminationReason)
	assert.Equal(t, riskfactor.ClosingMessage, s.UserMessage())
	assert.Equal(t, 3, s.CurrentRound) // 达到上限后不再增加轮次
	require.Len(t, s.History, 4)       // round 0,1,2,3 共4条问答记录
}

// 终态 session 再提交追问应返回 ErrSessionNotProcessing。
func TestSubmitFollowUpAnswer_OnTerminalStatus_ReturnsError(t *testing.T) {
	s := newTestSession()
	require.NoError(t, s.SubmitInitialAnswer("完整且合理的回答", &riskfactor.JudgementResult{
		Completeness: true, Reasonableness: true,
	}))
	require.Equal(t, riskfactor.StatusCleared, s.Status)

	err := s.SubmitFollowUpAnswer("再来一轮", &riskfactor.JudgementResult{Completeness: true, Reasonableness: true})

	assert.ErrorIs(t, err, riskfactor.ErrSessionNotProcessing)
	// 终态不受影响
	assert.Equal(t, riskfactor.StatusCleared, s.Status)
}

// judgement 为 nil 时返回 ErrInvalidJudgement，不改变状态。
func TestSubmitInitialAnswer_NilJudgement_ReturnsError(t *testing.T) {
	s := newTestSession()

	err := s.SubmitInitialAnswer("任意回答", nil)

	assert.ErrorIs(t, err, riskfactor.ErrInvalidJudgement)
	assert.Equal(t, riskfactor.StatusProcessing, s.Status)
	assert.Empty(t, s.History)
}

// LLMError 场景：MarkLLMError 后状态变为 LLMError，UserMessage 返回空字符串；
// 之后可以重试 SubmitInitialAnswer（不消耗轮次）。
func TestMarkLLMError_ThenRetrySubmitInitialAnswer_DoesNotConsumeRound(t *testing.T) {
	s := newTestSession()
	s.MarkLLMError()

	assert.Equal(t, riskfactor.StatusLLMError, s.Status)
	assert.Equal(t, "", s.UserMessage())

	err := s.SubmitInitialAnswer("重试后的回答", &riskfactor.JudgementResult{
		Completeness: true, Reasonableness: true,
	})

	require.NoError(t, err)
	assert.Equal(t, riskfactor.StatusCleared, s.Status)
	assert.Equal(t, 0, s.CurrentRound) // 重试首轮，不消耗轮次
}

// extracted_info 跨轮次字段级合并：同名字段以最新轮次为准，历史字段保留。
func TestExtractedInfo_MergeAcrossRounds(t *testing.T) {
	s := newTestSession()

	require.NoError(t, s.SubmitInitialAnswer("回答0", &riskfactor.JudgementResult{
		Completeness: false, Reasonableness: true, FollowUpQuestion: "追问1",
		ExtractedInfo: map[string]interface{}{"occupation": "财务经理", "company": "XX公司"},
	}))

	require.NoError(t, s.SubmitFollowUpAnswer("回答1", &riskfactor.JudgementResult{
		Completeness: true, Reasonableness: true,
		ExtractedInfo: map[string]interface{}{"occupation": "高级财务经理", "tenure": "2020年至今"}, // occupation 覆盖旧值
	}))

	assert.Equal(t, riskfactor.StatusCleared, s.Status)
	assert.Equal(t, "高级财务经理", s.ExtractedInfo["occupation"]) // 以最新轮次为准
	assert.Equal(t, "XX公司", s.ExtractedInfo["company"])      // 历史字段保留
	assert.Equal(t, "2020年至今", s.ExtractedInfo["tenure"])    // 新增字段
}

// UserMessage 在其他非典型状态（如尚未初始化任何判断的 Processing 初始态）应返回空字符串而非 panic。
func TestUserMessage_OnFreshSession_ReturnsEmpty(t *testing.T) {
	s := newTestSession()
	assert.Equal(t, "", s.UserMessage())
}

func TestJudgementResult_MergeInto_NilExisting(t *testing.T) {
	j := &riskfactor.JudgementResult{ExtractedInfo: map[string]interface{}{"a": 1}}
	merged := j.MergeInto(nil)
	assert.Equal(t, map[string]interface{}{"a": 1}, merged)
}

func TestReconstructRiskFactorSession_RestoresFollowUpQuestion(t *testing.T) {
	reason := riskfactor.TerminationReasonUnreasonable
	completeness := true
	reasonableness := false

	s := riskfactor.ReconstructRiskFactorSession(riskfactor.ReconstructParams{
		ID:                "sess_x",
		BatchID:           "batch_x",
		UserID:            "user_x",
		RiskFactorType:    riskfactor.RiskFactorTypeFundSource,
		MainQuestion:      "资金来源？",
		Status:            riskfactor.StatusNotCleared,
		CurrentRound:      0,
		MaxRounds:         3,
		TerminationReason: &reason,
		Completeness:      &completeness,
		Reasonableness:    &reasonableness,
		ExtractedInfo:     map[string]interface{}{"source": "工资"},
		History:           nil,
		FollowUpQuestion:  "",
	})

	assert.Equal(t, riskfactor.StatusNotCleared, s.Status)
	assert.Equal(t, riskfactor.ClosingMessage, s.UserMessage())
	assert.Empty(t, s.History)
}
