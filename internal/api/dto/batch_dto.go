package dto

type UserPayload struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}

type QuestionAnswerRequest struct {
	QuestionKey string   `json:"question_key"`
	Text        string   `json:"text,omitempty"`
	FileIDs     []string `json:"file_ids,omitempty"`
}

type RiskFactorItem struct {
	RiskFactorType string                  `json:"risk_factor_type"`
	Answers        []QuestionAnswerRequest `json:"answers"`
	MainQuestion   string                  `json:"main_question,omitempty"`
	Answer         string                  `json:"answer,omitempty"`
}

type BatchRequest struct {
	User        UserPayload      `json:"user"`
	RiskFactors []RiskFactorItem `json:"risk_factors"`
	Stream      bool             `json:"stream"`
}

type BatchResponse struct {
	BatchID   string          `json:"batch_id"`
	UserID    string          `json:"user_id"`
	UserName  string          `json:"user_name"`
	CreatedAt string          `json:"created_at"`
	Results   []SessionResult `json:"results,omitempty"`
	Sessions  []SessionDetail `json:"sessions,omitempty"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}
