package persistence

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/NoxiouSi/eino-risk-qa/internal/application"
)

// GORMUserBatchRepository 实现 application.UserBatchRepository：users/batches 表的简单持久化，
// 无状态机、无业务规则，因此不下沉到 domain 层。
type GORMUserBatchRepository struct {
	db *gorm.DB
}

// NewGORMUserBatchRepository 创建实例。
func NewGORMUserBatchRepository(db *gorm.DB) *GORMUserBatchRepository {
	return &GORMUserBatchRepository{db: db}
}

var _ application.UserBatchRepository = (*GORMUserBatchRepository)(nil)

// EnsureUser 若 user_id 不存在则插入；已存在时忽略（幂等，不覆盖已有 name）。
func (r *GORMUserBatchRepository) EnsureUser(ctx context.Context, u application.User) error {
	var existing UserModel
	err := r.db.WithContext(ctx).Where("user_id = ?", u.UserID).Take(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return r.db.WithContext(ctx).Create(&UserModel{UserID: u.UserID, Name: u.Name}).Error
}

// CreateBatch 插入一条批次记录。
func (r *GORMUserBatchRepository) CreateBatch(ctx context.Context, b application.Batch) error {
	return r.db.WithContext(ctx).Create(&BatchModel{BatchID: b.BatchID, UserID: b.UserID}).Error
}

// FindBatch 按 batch_id 查询；不存在返回 application.ErrBatchNotFound。
func (r *GORMUserBatchRepository) FindBatch(ctx context.Context, batchID string) (*application.Batch, error) {
	var bm BatchModel
	if err := r.db.WithContext(ctx).Where("batch_id = ?", batchID).Take(&bm).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, application.ErrBatchNotFound
		}
		return nil, err
	}
	return &application.Batch{BatchID: bm.BatchID, UserID: bm.UserID, CreatedAt: bm.CreatedAt}, nil
}
