package riskfactor

// QuestionJudgement 是单个可回答问题的审核结果。
type QuestionJudgement struct {
	QuestionKey    string `json:"question_key"`
	Required       bool   `json:"required"`
	Completeness   bool   `json:"completeness"`
	Reasonableness bool   `json:"reasonableness"`
	Note           string `json:"note"`
}

// JudgementResult 是 LLM 对本轮回答的结构化判断结果。
type JudgementResult struct {
	Completeness     bool
	Reasonableness   bool
	Questions        []QuestionJudgement
	MissingQuestions []string
	ExtractedInfo    map[string]interface{}
	ReasoningSummary string
	FollowUpQuestion string
}

// AggregateJudgement 从逐问题结果计算风险要素级快照。模型漏掉必填问题时按不完整处理。
func AggregateJudgement(specs []QuestionSpec, items []QuestionJudgement, extracted map[string]interface{}, reasoningSummary, followUpQuestion string) *JudgementResult {
	byKey := make(map[string]QuestionJudgement, len(items))
	for _, item := range items {
		byKey[item.QuestionKey] = item
	}
	result := &JudgementResult{Completeness: true, Reasonableness: true, Questions: make([]QuestionJudgement, 0, len(specs)), ExtractedInfo: extracted, ReasoningSummary: reasoningSummary, FollowUpQuestion: followUpQuestion}
	for _, spec := range specs {
		item, ok := byKey[spec.QuestionKey]
		if !ok {
			item = QuestionJudgement{QuestionKey: spec.QuestionKey, Completeness: false, Reasonableness: true, Note: "模型未返回该问题的判断"}
		}
		item.Required = spec.Required
		result.Questions = append(result.Questions, item)
		if spec.Required && !item.Completeness {
			result.Completeness = false
			result.MissingQuestions = append(result.MissingQuestions, spec.QuestionKey)
		}
		if item.Completeness && !item.Reasonableness {
			result.Reasonableness = false
		}
	}
	return result
}

// MergeInto 将本轮提取信息合并到已有信息中，同名 key 以本轮值覆盖。
func (j *JudgementResult) MergeInto(existing map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{}, len(existing)+len(j.ExtractedInfo))
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range j.ExtractedInfo {
		merged[k] = v
	}
	return merged
}
