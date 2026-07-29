package persistence_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
	"github.com/NoxiouSi/eino-risk-qa/internal/infra/persistence"
)

func newSession(id string) *riskfactor.RiskFactorSession {
	return riskfactor.NewRiskFactorSession(id, "batch_1", "user_1", riskfactor.RiskFactorTypeIdentity, "请说明您的身份信息及职业背景")
}

func newSessionInBatch(id, batchID string, riskFactorType riskfactor.RiskFactorType, mainQuestion string) *riskfactor.RiskFactorSession {
	return riskfactor.NewRiskFactorSession(id, batchID, "user_1", riskFactorType, mainQuestion)
}

// Save 首次保存（新建）应能成功插入 session 记录 + 对应的 qa_records，且 FindByID 能还原出等价的聚合。
func TestGORMSessionRepository_SaveAndFindByID_RoundTrip_Cleared(t *testing.T) {
	db := setupTestDB(t)
	repo := persistence.NewGORMSessionRepository(db)
	ctx := context.Background()

	s := newSession("sess_roundtrip_1")
	require.NoError(t, s.SubmitInitialAnswer("我是XX公司的财务经理", &riskfactor.JudgementResult{
		Completeness: true, Reasonableness: true,
		ExtractedInfo: map[string]interface{}{"occupation": "财务经理"},
	}))

	require.NoError(t, repo.Save(ctx, s))

	loaded, err := repo.FindByID(ctx, "sess_roundtrip_1")
	require.NoError(t, err)
	assert.Equal(t, riskfactor.StatusCleared, loaded.Status)
	assert.Equal(t, riskfactor.ClosingMessage, loaded.UserMessage())
	assert.Equal(t, "财务经理", loaded.ExtractedInfo["occupation"])
	require.Len(t, loaded.History, 1)
	assert.Equal(t, 0, loaded.History[0].Round)
	assert.Equal(t, "我是XX公司的财务经理", loaded.History[0].Answer)
}

// Save 应支持"先保存Processing态（含追问）→ 追问回答后再次保存”这种跨多次调用的增量持久化：
// 第二次 Save 不应重复插入 round=0 的 QA 记录，只新增 round=1 的记录。
func TestGORMSessionRepository_IncrementalSave_AcrossFollowUpRounds(t *testing.T) {
	db := setupTestDB(t)
	repo := persistence.NewGORMSessionRepository(db)
	ctx := context.Background()

	s := newSession("sess_incremental_1")
	require.NoError(t, s.SubmitInitialAnswer("我是财务经理", &riskfactor.JudgementResult{
		Completeness: false, Reasonableness: true, FollowUpQuestion: "任职时间是？",
		ExtractedInfo: map[string]interface{}{"occupation": "财务经理"},
	}))
	require.NoError(t, repo.Save(ctx, s))

	loadedAfterFirstSave, err := repo.FindByID(ctx, "sess_incremental_1")
	require.NoError(t, err)
	assert.Equal(t, riskfactor.StatusProcessing, loadedAfterFirstSave.Status)
	assert.Equal(t, "任职时间是？", loadedAfterFirstSave.UserMessage())
	require.Len(t, loadedAfterFirstSave.History, 1)

	// 模拟下一次 HTTP 请求：重新加载、提交追问回答、再次保存
	require.NoError(t, loadedAfterFirstSave.SubmitFollowUpAnswer("2020年至今", &riskfactor.JudgementResult{
		Completeness: true, Reasonableness: true,
		ExtractedInfo: map[string]interface{}{"tenure": "2020年至今"},
	}))
	require.NoError(t, repo.Save(ctx, loadedAfterFirstSave))

	final, err := repo.FindByID(ctx, "sess_incremental_1")
	require.NoError(t, err)
	assert.Equal(t, riskfactor.StatusCleared, final.Status)
	require.Len(t, final.History, 2)
	assert.Equal(t, 0, final.History[0].Round)
	assert.Equal(t, 1, final.History[1].Round)
	assert.Equal(t, "任职时间是？", final.History[1].Question)
	assert.Equal(t, "财务经理", final.ExtractedInfo["occupation"])
	assert.Equal(t, "2020年至今", final.ExtractedInfo["tenure"])
}

// 未排除且达到最大轮次：验证 NotCleared + termination_reason=max_rounds_incomplete 能正确落库与还原。
func TestGORMSessionRepository_SaveAndFindByID_NotClearedMaxRounds(t *testing.T) {
	db := setupTestDB(t)
	repo := persistence.NewGORMSessionRepository(db)
	ctx := context.Background()

	s := newSession("sess_maxrounds_1")
	require.NoError(t, s.SubmitInitialAnswer("回答0", &riskfactor.JudgementResult{Completeness: false, Reasonableness: true, FollowUpQuestion: "追问1"}))
	require.NoError(t, s.SubmitFollowUpAnswer("回答1", &riskfactor.JudgementResult{Completeness: false, Reasonableness: true, FollowUpQuestion: "追问2"}))
	require.NoError(t, s.SubmitFollowUpAnswer("回答2", &riskfactor.JudgementResult{Completeness: false, Reasonableness: true, FollowUpQuestion: "追问3"}))
	require.NoError(t, s.SubmitFollowUpAnswer("回答3", &riskfactor.JudgementResult{Completeness: false, Reasonableness: true}))

	require.NoError(t, repo.Save(ctx, s))

	loaded, err := repo.FindByID(ctx, "sess_maxrounds_1")
	require.NoError(t, err)
	assert.Equal(t, riskfactor.StatusNotCleared, loaded.Status)
	require.NotNil(t, loaded.TerminationReason)
	assert.Equal(t, riskfactor.TerminationReasonMaxRoundsIncomplete, *loaded.TerminationReason)
	assert.Equal(t, riskfactor.ClosingMessage, loaded.UserMessage())
	assert.Len(t, loaded.History, 4)
}

// FindByID 对不存在的 session_id 应返回 ErrSessionNotFound。
func TestGORMSessionRepository_FindByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := persistence.NewGORMSessionRepository(db)
	ctx := context.Background()

	_, err := repo.FindByID(ctx, "sess_does_not_exist")

	assert.ErrorIs(t, err, persistence.ErrSessionNotFound)
}

// 乐观锁冲突：模拟两次并发加载后基于同一"旧版本"分别保存，第二次保存应因 version 不匹配而失败，
// 避免并发提交追问答案导致轮次错乱（覆盖 docs/DESIGN.md 中乐观锁设计的行为）。
func TestGORMSessionRepository_Save_OptimisticLockConflict(t *testing.T) {
	db := setupTestDB(t)
	repo := persistence.NewGORMSessionRepository(db)
	ctx := context.Background()

	s := newSession("sess_optlock_1")
	require.NoError(t, s.SubmitInitialAnswer("回答0", &riskfactor.JudgementResult{Completeness: false, Reasonableness: true, FollowUpQuestion: "追问1"}))
	require.NoError(t, repo.Save(ctx, s))

	// 模拟两个并发请求分别加载到同一版本
	copyA, err := repo.FindByID(ctx, "sess_optlock_1")
	require.NoError(t, err)
	copyB, err := repo.FindByID(ctx, "sess_optlock_1")
	require.NoError(t, err)

	require.NoError(t, copyA.SubmitFollowUpAnswer("回答A", &riskfactor.JudgementResult{Completeness: true, Reasonableness: true}))
	require.NoError(t, repo.Save(ctx, copyA)) // 第一次保存成功，底层 version 递增

	require.NoError(t, copyB.SubmitFollowUpAnswer("回答B", &riskfactor.JudgementResult{Completeness: true, Reasonableness: true}))
	err = repo.Save(ctx, copyB) // 第二次保存应因 version 过期而冲突

	assert.ErrorIs(t, err, persistence.ErrOptimisticLockConflict)
}

// Save 应保证"新增QA记录"与"更新session状态/轮次"在同一事务内完成：此处通过验证 round 计数与
// History 条数在正常路径下始终一致来间接验证（事务失败的场景在 OptimisticLockConflict 测试中已覆盖：
// 冲突时不会留下部分写入的脏数据）。
func TestGORMSessionRepository_Save_KeepsSessionAndQARecordsConsistent(t *testing.T) {
	db := setupTestDB(t)
	repo := persistence.NewGORMSessionRepository(db)
	ctx := context.Background()

	s := newSession("sess_consistency_1")
	require.NoError(t, s.SubmitInitialAnswer("回答0", &riskfactor.JudgementResult{Completeness: false, Reasonableness: true, FollowUpQuestion: "追问1"}))
	require.NoError(t, repo.Save(ctx, s))

	var qaCount int64
	require.NoError(t, db.Table("qa_records").Where("session_id = ?", "sess_consistency_1").Count(&qaCount).Error)
	assert.EqualValues(t, 1, qaCount)

	loaded, err := repo.FindByID(ctx, "sess_consistency_1")
	require.NoError(t, err)
	assert.Equal(t, 1, loaded.CurrentRound)
	assert.Len(t, loaded.History, 1)
}

// FindByBatchID 应返回该批次下的全部会话（各自带完整History），且不误返回其他批次的会话。
func TestGORMSessionRepository_FindByBatchID_ReturnsAllSessionsInBatch(t *testing.T) {
	db := setupTestDB(t)
	repo := persistence.NewGORMSessionRepository(db)
	ctx := context.Background()

	identity := newSessionInBatch("sess_batch_a_1", "batch_a", riskfactor.RiskFactorTypeIdentity, "请说明您的身份信息及职业背景")
	require.NoError(t, identity.SubmitInitialAnswer("我是财务经理", &riskfactor.JudgementResult{
		Completeness: false, Reasonableness: true, FollowUpQuestion: "任职时间是？",
	}))
	require.NoError(t, repo.Save(ctx, identity))

	fundSource := newSessionInBatch("sess_batch_a_2", "batch_a", riskfactor.RiskFactorTypeFundSource, "请说明本次资金的来源")
	require.NoError(t, fundSource.SubmitInitialAnswer("资金来源于工资收入，已工作多年积累", &riskfactor.JudgementResult{
		Completeness: true, Reasonableness: true,
	}))
	require.NoError(t, repo.Save(ctx, fundSource))

	other := newSessionInBatch("sess_batch_b_1", "batch_b", riskfactor.RiskFactorTypeIdentity, "其他批次的主问题")
	require.NoError(t, other.SubmitInitialAnswer("与batch_a无关的回答", &riskfactor.JudgementResult{
		Completeness: true, Reasonableness: true,
	}))
	require.NoError(t, repo.Save(ctx, other))

	sessions, err := repo.FindByBatchID(ctx, "batch_a")
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	byID := map[string]*riskfactor.RiskFactorSession{}
	for _, s := range sessions {
		byID[s.ID] = s
	}
	require.Contains(t, byID, "sess_batch_a_1")
	require.Contains(t, byID, "sess_batch_a_2")
	assert.Equal(t, riskfactor.StatusProcessing, byID["sess_batch_a_1"].Status)
	assert.Len(t, byID["sess_batch_a_1"].History, 1)
	assert.Equal(t, riskfactor.StatusCleared, byID["sess_batch_a_2"].Status)
}

// 不存在的 batch_id 应返回空切片、不返回错误。
func TestGORMSessionRepository_FindByBatchID_EmptyForUnknownBatch(t *testing.T) {
	db := setupTestDB(t)
	repo := persistence.NewGORMSessionRepository(db)

	sessions, err := repo.FindByBatchID(context.Background(), "batch_does_not_exist")

	require.NoError(t, err)
	assert.Empty(t, sessions)
}
