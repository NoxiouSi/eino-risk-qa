package dto

// BatchRequest 对应 POST /api/v1/batches 请求体。
type BatchRequest struct {
	User        UserPayload      `json:"user"`
	RiskFactors []RiskFactorItem `json:"risk_factors"`
	Stream      bool             `json:"stream"`
}

// UserPayload 请求体中的用户信息。
type UserPayload struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}

// RiskFactorItem 请求体中单个风险要素。
type RiskFactorItem struct {
	RiskFactorType string `json:"risk_factor_type"`
	MainQuestion   string `json:"main_question"`
	Answer         string `json:"answer"`
}

// BatchResponse 对应 POST /api/v1/batches 与 GET /api/v1/batches/{batch_id} 的响应体。
type BatchResponse struct {
	BatchID   string          `json:"batch_id"`
	CreatedAt string          `json:"created_at"`
	Results   []SessionResult `json:"results,omitempty"`
	Sessions  []SessionDetail `json:"sessions,omitempty"`
}

// SessionResult 批量提交响应中单个风险要素的即时状态（不含history）。
type SessionResult struct {
	SessionID         string                 `json:"session_id"`
	RiskFactorType    string                 `json:"risk_factor_type"`
	Status            string                 `json:"status"`
	CurrentRound      int                    `json:"current_round"`
	Message           string                 `json:"message"`
	Cleared           *bool                  `json:"cleared"`
	TerminationReason *string                `json:"termination_reason"`
	ExtractedInfo     map[string]interface{} `json:"extracted_info"`
	Error             *ErrorPayload          `json:"error"`
}

// ErrorPayload 单要素失败时的错误信息。
type ErrorPayload struct {
	Code    string `json:"error_code"`
	Message string `json:"message"`
}
