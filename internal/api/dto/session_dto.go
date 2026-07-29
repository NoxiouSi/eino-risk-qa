package dto

// FollowUpAnswerRequest 对应 POST /api/v1/sessions/{session_id}/answers 请求体。
type FollowUpAnswerRequest struct {
	Answer string `json:"answer"`
	Stream bool   `json:"stream"`
}

// SessionDetail 对应 GET /api/v1/sessions/{session_id} 响应体，及批次查询响应中 sessions 数组的单项。
type SessionDetail struct {
	SessionID         string                 `json:"session_id"`
	RiskFactorType    string                 `json:"risk_factor_type"`
	MainQuestion      string                 `json:"main_question"`
	Status            string                 `json:"status"`
	CurrentRound      int                    `json:"current_round"`
	MaxRounds         int                    `json:"max_rounds"`
	Message           string                 `json:"message"`
	Cleared           *bool                  `json:"cleared"`
	TerminationReason *string                `json:"termination_reason"`
	ExtractedInfo     map[string]interface{} `json:"extracted_info"`
	History           []QAPairPayload        `json:"history,omitempty"`
	Error             *ErrorPayload          `json:"error,omitempty"`
}

// QAPairPayload 历史问答记录的单条快照。
type QAPairPayload struct {
	Round          int    `json:"round"`
	Question       string `json:"question"`
	Answer         string `json:"answer"`
	Completeness   bool   `json:"completeness"`
	Reasonableness bool   `json:"reasonableness"`
}

// ErrorResponse 统一错误响应结构（4xx/5xx场景）。
type ErrorResponse struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}
