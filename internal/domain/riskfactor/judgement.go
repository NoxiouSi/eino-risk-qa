package riskfactor

// JudgementResult 单轮 LLM 判断结果值对象。
type JudgementResult struct {
	// Completeness 完整性：信息是否已全部覆盖，驱动追问循环是否继续。
	Completeness bool
	// Reasonableness 合理性：内容是否可信、无矛盾，参与终态结论合成。
	Reasonableness bool
	// FollowUpQuestion 针对完整性缺口生成的追问问题，仅 Completeness=false 时有效。
	FollowUpQuestion string
	// ExtractedInfo 本轮从回答中提取到的结构化信息（增量，尚未与历史合并）。
	ExtractedInfo map[string]interface{}
	// ReasoningSummary 判断依据摘要，供审计使用。
	ReasoningSummary string
}

// MergeInto 将本轮提取信息与历史累积信息合并，返回合并后的新 map。
// 合并规则：同名字段以本轮（最新）取值为准；existing 中未被本轮覆盖的字段保留。
// existing 为 nil 时视为空 map，不会 panic。
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
