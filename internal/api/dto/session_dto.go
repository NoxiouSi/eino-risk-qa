package dto

type FollowUpAnswerRequest struct {
	Answers []QuestionAnswerRequest `json:"answers"`
	Answer  string                  `json:"answer,omitempty"`
	Stream  bool                    `json:"stream"`
}

type QuestionJudgementPayload struct {
	QuestionKey    string `json:"question_key"`
	Required       bool   `json:"required"`
	Completeness   bool   `json:"completeness"`
	Reasonableness bool   `json:"reasonableness"`
	Note           string `json:"note"`
}

type SessionResult struct {
	SessionID           string                     `json:"session_id"`
	RiskFactorType      string                     `json:"risk_factor_type"`
	Status              string                     `json:"status"`
	CurrentRound        int                        `json:"current_round"`
	Message             string                     `json:"message"`
	Cleared             *bool                      `json:"cleared"`
	TerminationReason   *string                    `json:"termination_reason"`
	ExtractedInfo       map[string]interface{}     `json:"extracted_info"`
	MissingQuestionKeys []string                   `json:"missing_question_keys"`
	QuestionJudgements  []QuestionJudgementPayload `json:"question_judgements"`
	Error               *ErrorPayload              `json:"error"`
}

type QAPairPayload struct {
	Round              int                        `json:"round"`
	Question           string                     `json:"question"`
	Answer             string                     `json:"answer"`
	Completeness       bool                       `json:"completeness"`
	Reasonableness     bool                       `json:"reasonableness"`
	QuestionJudgements []QuestionJudgementPayload `json:"question_judgements"`
}

type SessionDetail struct {
	SessionID           string                     `json:"session_id"`
	RiskFactorType      string                     `json:"risk_factor_type"`
	MainQuestion        string                     `json:"main_question"`
	Questions           []QuestionItem             `json:"questions"`
	Status              string                     `json:"status"`
	CurrentRound        int                        `json:"current_round"`
	MaxRounds           int                        `json:"max_rounds"`
	Message             string                     `json:"message"`
	Cleared             *bool                      `json:"cleared"`
	TerminationReason   *string                    `json:"termination_reason"`
	ExtractedInfo       map[string]interface{}     `json:"extracted_info"`
	MissingQuestionKeys []string                   `json:"missing_question_keys"`
	QuestionJudgements  []QuestionJudgementPayload `json:"question_judgements"`
	History             []QAPairPayload            `json:"history"`
	Error               *ErrorPayload              `json:"error"`
}
