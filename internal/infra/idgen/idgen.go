package idgen

import "github.com/google/uuid"

// UUIDGenerator 是 application.IDGenerator 的默认实现：使用 UUID 并加上业务前缀，
// 便于日志/数据库中直观区分标识类型（如 batch_xxxxxxxx-xxxx-... / sess_xxxxxxxx-...）。
type UUIDGenerator struct{}

// NewUUIDGenerator 创建实例。
func NewUUIDGenerator() *UUIDGenerator {
	return &UUIDGenerator{}
}

// NewBatchID 生成一个带 "batch_" 前缀的批次业务标识。
func (UUIDGenerator) NewBatchID() string {
	return "batch_" + uuid.NewString()
}

// NewSessionID 生成一个带 "sess_" 前缀的会话业务标识。
func (UUIDGenerator) NewSessionID() string {
	return "sess_" + uuid.NewString()
}
