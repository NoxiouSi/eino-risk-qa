package dto

type QuestionItem struct {
	QuestionKey    string `json:"question_key"`
	QuestionText   string `json:"question_text"`
	AnswerType     string `json:"answer_type"`
	Required       bool   `json:"required"`
	MinSubmitCount int    `json:"min_submit_count"`
	SortOrder      int    `json:"sort_order"`
}

type MainQuestionItem struct {
	RiskFactorType string         `json:"risk_factor_type"`
	MainQuestion   string         `json:"main_question"`
	Questions      []QuestionItem `json:"questions"`
}

type MainQuestionsResponse struct {
	UserID string             `json:"user_id"`
	Items  []MainQuestionItem `json:"items"`
}
