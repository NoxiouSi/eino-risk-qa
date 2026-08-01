package dto

// MainQuestionItem 单个风险要素类型及其对应的主问题。
type MainQuestionItem struct {
	RiskFactorType string `json:"risk_factor_type"`
	MainQuestion   string `json:"main_question"`
}

// MainQuestionsResponse 对应 GET /api/v1/users/{user_id}/main-questions 响应体。
type MainQuestionsResponse struct {
	UserID string             `json:"user_id"`
	Items  []MainQuestionItem `json:"items"`
}
