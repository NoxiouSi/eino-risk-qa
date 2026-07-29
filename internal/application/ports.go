package application

import (
	"context"
	"errors"
	"time"
)

// ErrBatchNotFound 按 batch_id 未找到批次记录（供 api 层转换为 404 BATCH_NOT_FOUND）。
var ErrBatchNotFound = errors.New("application: batch not found")

// User 应用层的用户记录（贫血模型，无业务规则，不属于domain核心领域知识）。
type User struct {
	UserID string
	Name   string
}

// Batch 应用层的批次记录。
type Batch struct {
	BatchID   string
	UserID    string
	CreatedAt time.Time
}

// UserBatchRepository 应用层端口：users/batches 表的简单持久化能力（无状态机、无业务规则）。
// 由 infra/persistence 实现；不下沉到 domain 层，因为 User/Batch 本身不是核心领域知识的载体。
type UserBatchRepository interface {
	// EnsureUser 若用户不存在则创建，存在则忽略（幂等）。
	EnsureUser(ctx context.Context, u User) error
	// CreateBatch 创建一条批次记录。
	CreateBatch(ctx context.Context, b Batch) error
	// FindBatch 按 batch_id 查询批次记录；不存在返回 ErrBatchNotFound。
	FindBatch(ctx context.Context, batchID string) (*Batch, error)
}
