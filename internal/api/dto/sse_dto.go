package dto

// SSEMessageDeltaPayload message_delta 事件的 data 字段结构。
type SSEMessageDeltaPayload struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
}

// SSEResultPayload result 事件的 data 字段结构，字段与 SessionResult 一致（多了 risk_factor_type 便于批量场景多路复用识别）。
type SSEResultPayload struct {
	SessionID         string                 `json:"session_id"`
	RiskFactorType    string                 `json:"risk_factor_type,omitempty"`
	Status            string                 `json:"status"`
	CurrentRound      int                    `json:"current_round"`
	Message           string                 `json:"message"`
	Cleared           *bool                  `json:"cleared"`
	TerminationReason *string                `json:"termination_reason"`
	ExtractedInfo     map[string]interface{} `json:"extracted_info"`
}

// SSEDonePayload done 事件的 data 字段结构。
type SSEDonePayload struct {
	SessionID string `json:"session_id"`
}

// SSEErrorPayload error 事件的 data 字段结构。
type SSEErrorPayload struct {
	SessionID string `json:"session_id"`
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}
