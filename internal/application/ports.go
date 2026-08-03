package application

import (
	"context"
	"errors"
	"time"
)

// ErrBatchNotFound 按 batch_id 未找到批次记录（供 api 层转换为 404 BATCH_NOT_FOUND）。
var ErrBatchNotFound = errors.New("application: batch not found")

// ErrUserNotFound 按 user_id 未找到用户记录（供 api 层转换为 404 USER_NOT_FOUND）。
// 与"批量提交时的 EnsureUser 幂等 upsert"不同，该错误专用于"查询用户已预配置的风险项"场景：
// 风险项是预配置业务数据（如由外部系统导入），不存在时不应自动创建用户。
var ErrUserNotFound = errors.New("application: user not found")

// User 应用层的用户记录（贫血模型，无业务规则，不属于domain核心领域知识）。
type User struct {
	UserID string
	Name   string
	// RiskFactorTypes 该用户拥有的风险要素类型列表（对应 users.risk_factor_types 逗号分隔列解析结果），
	// 用于查询该用户应回答哪些主问题；EnsureUser（批量提交场景）不关心此字段。
	RiskFactorTypes []string
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
	// FindUser 按 user_id 查询用户记录（含其预配置的 RiskFactorTypes）；不存在返回 ErrUserNotFound。
	FindUser(ctx context.Context, userID string) (*User, error)
}

// RiskFactorQuestionCatalog 查询风险要素的统一层级问题配置及其审核 Skill。
type RiskFactorQuestionCatalog interface {
	FindQuestionTrees(ctx context.Context, riskFactorTypes []string) (map[string]QuestionTree, error)
}

// UploadedFile 是应用层使用的文件元数据，不暴露真实存储根目录。
type UploadedFile struct {
	FileID         string
	UserID         string
	RiskFactorType string
	QuestionKey    string
	OriginalName   string
	StoredPath     string
	MIMEType       string
	SizeBytes      int64
	SHA256         string
	CreatedAt      time.Time
}

// AttachmentRepository 保存并按归属查询上传文件元数据。
type AttachmentRepository interface {
	Create(ctx context.Context, file UploadedFile) error
	FindOwned(ctx context.Context, fileID, userID, riskFactorType, questionKey string) (*UploadedFile, error)
}
