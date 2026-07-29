package persistence

import "time"

// UserModel 对应 users 表。
type UserModel struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID    string    `gorm:"column:user_id;uniqueIndex:uk_user_id;size:64;not null"`
	Name      string    `gorm:"column:name;size:128"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (UserModel) TableName() string { return "users" }

// BatchModel 对应 batches 表。
type BatchModel struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	BatchID   string    `gorm:"column:batch_id;uniqueIndex:uk_batch_id;size:64;not null"`
	UserID    string    `gorm:"column:user_id;index:idx_user_id;size:64;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (BatchModel) TableName() string { return "batches" }

// RiskFactorSessionModel 对应 risk_factor_sessions 表。
type RiskFactorSessionModel struct {
	ID                uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	SessionID         string    `gorm:"column:session_id;uniqueIndex:uk_session_id;size:64;not null"`
	BatchID           string    `gorm:"column:batch_id;index:idx_batch_id;size:64;not null"`
	UserID            string    `gorm:"column:user_id;index:idx_user_id;size:64;not null"`
	RiskFactorType    string    `gorm:"column:risk_factor_type;size:32;not null"`
	MainQuestion      string    `gorm:"column:main_question;type:text;not null"`
	Status            string    `gorm:"column:status;size:32;not null;default:processing;index:idx_status"`
	CurrentRound      int       `gorm:"column:current_round;not null;default:0"`
	MaxRounds         int       `gorm:"column:max_rounds;not null;default:3"`
	Completeness      *bool     `gorm:"column:completeness"`
	Reasonableness    *bool     `gorm:"column:reasonableness"`
	TerminationReason *string   `gorm:"column:termination_reason;size:32"`
	Cleared           *bool     `gorm:"column:cleared"`
	ExtractedInfo     JSONMap   `gorm:"column:extracted_info;type:json"`
	FollowUpQuestion  string    `gorm:"column:follow_up_question;type:text"`
	Version           int       `gorm:"column:version;not null;default:0"`
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (RiskFactorSessionModel) TableName() string { return "risk_factor_sessions" }

// QARecordModel 对应 qa_records 表。
type QARecordModel struct {
	ID                 uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	SessionID          string    `gorm:"column:session_id;index:idx_session_id;size:64;not null"`
	Round              int       `gorm:"column:round;not null"`
	Question           string    `gorm:"column:question;type:text;not null"`
	Answer             string    `gorm:"column:answer;type:text;not null"`
	Completeness       *bool     `gorm:"column:completeness"`
	Reasonableness     *bool     `gorm:"column:reasonableness"`
	ExtractedInfoDelta JSONMap   `gorm:"column:extracted_info_delta;type:json"`
	CreatedAt          time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (QARecordModel) TableName() string { return "qa_records" }
