package persistence

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

// ErrOptimisticLockConflict 乐观锁冲突：并发更新导致 version 不匹配。
var ErrOptimisticLockConflict = errors.New("persistence: optimistic lock conflict")

// ErrSessionNotFound 按 session_id 未找到记录。它包装了 riskfactor.ErrSessionNotFound，
// 因此 errors.Is(err, riskfactor.ErrSessionNotFound) 与 errors.Is(err, persistence.ErrSessionNotFound)
// 均能成立，使 application 层可以只依赖 domain 层的哨兵错误、无需感知 infra 层的具体错误类型。
var ErrSessionNotFound = fmt.Errorf("persistence: %w", riskfactor.ErrSessionNotFound)

// GORMSessionRepository 实现 domain.SessionRepository 端口：
// Save 在同一事务内同时持久化 session 状态与新增的 QA 记录，并通过 version 字段做乐观锁；
// FindByID 还原完整聚合（含全部历史问答）。
type GORMSessionRepository struct {
	db *gorm.DB
}

// NewGORMSessionRepository 创建一个基于给定 *gorm.DB 的仓储实现。
func NewGORMSessionRepository(db *gorm.DB) *GORMSessionRepository {
	return &GORMSessionRepository{db: db}
}

var _ riskfactor.SessionRepository = (*GORMSessionRepository)(nil)

// Save 事务内同时持久化 session 状态与新增的 QA 记录（不会重复插入已存在的轮次记录）。
//
// 实现要点：
//   - 新建（数据库中不存在该 session_id）：INSERT session + INSERT 全部 History 对应的 qa_records，version 初始为 0；
//   - 更新（已存在）：先查询当前 version，若聚合内存中不知道自己的"起始 version"，采用"按 session_id 匹配 + 影响行数校验"的
//     方式实现乐观锁——每次 Save 都将 version 无条件 +1，并要求 UPDATE 语句在执行前先读到的 version 与 DB 当前一致，
//     否则 RowsAffected=0，判定为冲突；
//   - 只插入 History 中 round >= 数据库已有的最大 round+1 的记录，避免重复插入。
func (r *GORMSessionRepository) Save(ctx context.Context, session *riskfactor.RiskFactorSession) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing RiskFactorSessionModel
		err := tx.Where("session_id = ?", session.ID).Take(&existing).Error

		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			model := toModel(session, 0)
			if err := tx.Create(model).Error; err != nil {
				return err
			}
			records := toQARecordModels(session, 0)
			if len(records) > 0 {
				if err := tx.Create(&records).Error; err != nil {
					return err
				}
			}
			return nil
		case err != nil:
			return err
		}

		// 已存在：先确定数据库中已落库的最大 round，只插入更新之后新增的记录。
		var maxRound int64
		hasAny := int64(0)
		if err := tx.Table("qa_records").
			Where("session_id = ?", session.ID).
			Count(&hasAny).Error; err != nil {
			return err
		}
		nextRound := 0
		if hasAny > 0 {
			if err := tx.Table("qa_records").
				Where("session_id = ?", session.ID).
				Select("COALESCE(MAX(round), -1)").Scan(&maxRound).Error; err != nil {
				return err
			}
			nextRound = int(maxRound) + 1
		}

		// 乐观锁核心：WHERE 条件必须使用聚合根被加载时持有的 version（session.Version()），
		// 而不是本次事务内刚查到的 existing.Version——否则永远能匹配上，检测不到并发冲突。
		expectedVersion := session.Version()
		model := toModel(session, expectedVersion+1)
		result := tx.Model(&RiskFactorSessionModel{}).
			Where("session_id = ? AND version = ?", session.ID, expectedVersion).
			Updates(map[string]interface{}{
				"status":             model.Status,
				"current_round":      model.CurrentRound,
				"max_rounds":         model.MaxRounds,
				"completeness":       model.Completeness,
				"reasonableness":     model.Reasonableness,
				"termination_reason": model.TerminationReason,
				"cleared":            model.Cleared,
				"extracted_info":     model.ExtractedInfo,
				"follow_up_question": model.FollowUpQuestion,
				"version":            existing.Version + 1,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrOptimisticLockConflict
		}

		records := toQARecordModels(session, nextRound)
		if len(records) > 0 {
			if err := tx.Create(&records).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// FindByID 按业务 session_id 加载聚合，还原完整历史；不存在时返回 ErrSessionNotFound。
func (r *GORMSessionRepository) FindByID(ctx context.Context, sessionID string) (*riskfactor.RiskFactorSession, error) {
	var sm RiskFactorSessionModel
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Take(&sm).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	var qas []QARecordModel
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("round ASC").
		Find(&qas).Error; err != nil {
		return nil, err
	}

	return toDomain(&sm, qas), nil
}

// FindByBatchID 按业务 batch_id 列出该批次下的全部会话（含各自完整历史），
// 用于批次查询接口；不存在任何会话时返回空切片、不返回错误（由调用方结合 batches 表判断 batch 是否存在）。
func (r *GORMSessionRepository) FindByBatchID(ctx context.Context, batchID string) ([]*riskfactor.RiskFactorSession, error) {
	var sms []RiskFactorSessionModel
	if err := r.db.WithContext(ctx).
		Where("batch_id = ?", batchID).
		Order("id ASC").
		Find(&sms).Error; err != nil {
		return nil, err
	}
	if len(sms) == 0 {
		return []*riskfactor.RiskFactorSession{}, nil
	}

	sessionIDs := make([]string, 0, len(sms))
	for _, sm := range sms {
		sessionIDs = append(sessionIDs, sm.SessionID)
	}

	var qas []QARecordModel
	if err := r.db.WithContext(ctx).
		Where("session_id IN ?", sessionIDs).
		Order("round ASC").
		Find(&qas).Error; err != nil {
		return nil, err
	}
	qasBySession := make(map[string][]QARecordModel, len(sms))
	for _, qa := range qas {
		qasBySession[qa.SessionID] = append(qasBySession[qa.SessionID], qa)
	}

	results := make([]*riskfactor.RiskFactorSession, 0, len(sms))
	for i := range sms {
		results = append(results, toDomain(&sms[i], qasBySession[sms[i].SessionID]))
	}
	return results, nil
}
