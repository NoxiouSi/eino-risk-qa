package persistence

import "time"

// UserModel 对应 users 表。
type UserModel struct {
	ID              uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID          string    `gorm:"column:user_id;uniqueIndex:uk_user_id;size:64;not null"`
	Name            string    `gorm:"column:name;size:128"`
	RiskFactorTypes string    `gorm:"column:risk_factor_types;size:255;not null;default:''"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (UserModel) TableName() string { return "users" }

// RiskFactorQuestionModel 对应统一问题配置表。
type RiskFactorQuestionModel struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	RiskFactorType string    `gorm:"column:risk_factor_type;size:32;not null"`
	QuestionKey    string    `gorm:"column:question_key;size:64;not null"`
	ParentID       *uint64   `gorm:"column:parent_id"`
	QuestionText   string    `gorm:"column:question_text;type:text;not null"`
	AnswerType     string    `gorm:"column:answer_type;size:16;not null"`
	Required       bool      `gorm:"column:required;not null"`
	MinSubmitCount int       `gorm:"column:min_submit_count;not null"`
	MaxSubmitCount int       `gorm:"column:max_submit_count;not null"`
	SortOrder      int       `gorm:"column:sort_order;not null"`
	Enabled        bool      `gorm:"column:enabled;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (RiskFactorQuestionModel) TableName() string { return "risk_factor_questions" }

type AuditSkillModel struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	SkillKey     string    `gorm:"column:skill_key;size:64;not null"`
	Name         string    `gorm:"column:name;size:128;not null"`
	RuleText     string    `gorm:"column:rule_text;type:text;not null"`
	EvidenceType string    `gorm:"column:evidence_type;size:16;not null"`
	Enabled      bool      `gorm:"column:enabled;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (AuditSkillModel) TableName() string { return "audit_skills" }

type QuestionSkillRefModel struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	QuestionID uint64    `gorm:"column:question_id;not null"`
	SkillID    uint64    `gorm:"column:skill_id;not null"`
	SortOrder  int       `gorm:"column:sort_order;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (QuestionSkillRefModel) TableName() string { return "question_skill_refs" }

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
	QuestionJudgements JSONSlice `gorm:"column:question_judgements;type:json"`
	ExtractedInfoDelta JSONMap   `gorm:"column:extracted_info_delta;type:json"`
	CreatedAt          time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (QARecordModel) TableName() string { return "qa_records" }

type UploadedFileModel struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	FileID         string    `gorm:"column:file_id;size:64;not null;uniqueIndex"`
	UserID         string    `gorm:"column:user_id;size:64;not null"`
	RiskFactorType string    `gorm:"column:risk_factor_type;size:32;not null"`
	QuestionKey    string    `gorm:"column:question_key;size:64;not null"`
	OriginalName   string    `gorm:"column:original_name;size:255;not null"`
	StoredPath     string    `gorm:"column:stored_path;size:512;not null"`
	MIMEType       string    `gorm:"column:mime_type;size:128;not null"`
	SizeBytes      int64     `gorm:"column:size_bytes;not null"`
	SHA256         string    `gorm:"column:sha256;size:64;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (UploadedFileModel) TableName() string { return "uploaded_files" }

type QuestionSubmissionModel struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	SubmissionID   string    `gorm:"column:submission_id;size:64;not null;uniqueIndex"`
	SessionID      string    `gorm:"column:session_id;size:64;not null"`
	Round          int       `gorm:"column:round;not null"`
	RiskFactorType string    `gorm:"column:risk_factor_type;size:32;not null"`
	QuestionKey    string    `gorm:"column:question_key;size:64;not null"`
	ValueType      string    `gorm:"column:value_type;size:16;not null"`
	TextValue      *string   `gorm:"column:text_value;type:text"`
	FileID         *string   `gorm:"column:file_id;size:64"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (QuestionSubmissionModel) TableName() string { return "question_submissions" }
