package persistence

import (
	"github.com/google/uuid"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

// toDomain 将 GORM 实体（session + 其全部 qa_records）映射为领域聚合根。
func toDomain(sm *RiskFactorSessionModel, qas []QARecordModel) *riskfactor.RiskFactorSession {
	history := make([]riskfactor.QAPair, 0, len(qas))
	for _, qa := range qas {
		history = append(history, riskfactor.QAPair{
			Round:          qa.Round,
			Question:       qa.Question,
			Answer:         qa.Answer,
			Completeness:   boolValue(qa.Completeness),
			Reasonableness: boolValue(qa.Reasonableness),
			Judgements:     toQuestionJudgements(qa.QuestionJudgements),
			CreatedAt:      qa.CreatedAt,
		})
	}

	var terminationReason *riskfactor.TerminationReason
	if sm.TerminationReason != nil {
		r := riskfactor.TerminationReason(*sm.TerminationReason)
		terminationReason = &r
	}

	return riskfactor.ReconstructRiskFactorSession(riskfactor.ReconstructParams{
		ID:                sm.SessionID,
		BatchID:           sm.BatchID,
		UserID:            sm.UserID,
		RiskFactorType:    riskfactor.RiskFactorType(sm.RiskFactorType),
		MainQuestion:      sm.MainQuestion,
		Status:            riskfactor.SessionStatus(sm.Status),
		CurrentRound:      sm.CurrentRound,
		MaxRounds:         sm.MaxRounds,
		TerminationReason: terminationReason,
		Completeness:      sm.Completeness,
		Reasonableness:    sm.Reasonableness,
		ExtractedInfo:     map[string]interface{}(sm.ExtractedInfo),
		History:           history,
		FollowUpQuestion:  sm.FollowUpQuestion,
		Version:           sm.Version,
		CreatedAt:         sm.CreatedAt,
		UpdatedAt:         sm.UpdatedAt,
	})
}

// toModel 将领域聚合根映射为 GORM session 实体（不含 qa_records，由调用方单独处理增量记录）。
// currentVersion 为乐观锁当前版本号（新建时传 0）。
func toModel(s *riskfactor.RiskFactorSession, currentVersion int) *RiskFactorSessionModel {
	var terminationReason *string
	if s.TerminationReason != nil {
		v := string(*s.TerminationReason)
		terminationReason = &v
	}

	var cleared *bool
	switch s.Status {
	case riskfactor.StatusCleared:
		v := true
		cleared = &v
	case riskfactor.StatusNotCleared:
		v := false
		cleared = &v
	}

	return &RiskFactorSessionModel{
		SessionID:         s.ID,
		BatchID:           s.BatchID,
		UserID:            s.UserID,
		RiskFactorType:    string(s.RiskFactorType),
		MainQuestion:      s.MainQuestion,
		Status:            string(s.Status),
		CurrentRound:      s.CurrentRound,
		MaxRounds:         s.MaxRounds,
		Completeness:      s.Completeness,
		Reasonableness:    s.Reasonableness,
		TerminationReason: terminationReason,
		Cleared:           cleared,
		ExtractedInfo:     JSONMap(s.ExtractedInfo),
		FollowUpQuestion:  s.FollowUpQuestion(),
		Version:           currentVersion,
		CreatedAt:         s.CreatedAt,
		UpdatedAt:         s.UpdatedAt,
	}
}

// toQARecordModels 将聚合根 History 中"尚未落库"的部分（fromRound 之后）转换为待插入的 QARecordModel。
func toQARecordModels(s *riskfactor.RiskFactorSession, fromRound int) []QARecordModel {
	records := make([]QARecordModel, 0)
	for _, qa := range s.History {
		if qa.Round < fromRound {
			continue
		}
		completeness := qa.Completeness
		reasonableness := qa.Reasonableness
		records = append(records, QARecordModel{
			SessionID:          s.ID,
			Round:              qa.Round,
			Question:           qa.Question,
			Answer:             qa.Answer,
			Completeness:       &completeness,
			Reasonableness:     &reasonableness,
			QuestionJudgements: fromQuestionJudgements(qa.Judgements),
			CreatedAt:          qa.CreatedAt,
		})
	}
	return records
}

func toQuestionSubmissionModels(s *riskfactor.RiskFactorSession, fromRound int) []QuestionSubmissionModel {
	var result []QuestionSubmissionModel
	for _, qa := range s.History {
		if qa.Round < fromRound {
			continue
		}
		for _, answer := range qa.Answers {
			if answer.Text != "" {
				text := answer.Text
				result = append(result, QuestionSubmissionModel{SubmissionID: uuid.NewString(), SessionID: s.ID, Round: qa.Round, RiskFactorType: string(s.RiskFactorType), QuestionKey: answer.QuestionKey, ValueType: "text", TextValue: &text, CreatedAt: qa.CreatedAt})
			}
			for _, fileIDValue := range answer.FileIDs {
				fileID := fileIDValue
				valueType := answer.ValueType
				if valueType == "" {
					valueType = "file"
				}
				result = append(result, QuestionSubmissionModel{SubmissionID: uuid.NewString(), SessionID: s.ID, Round: qa.Round, RiskFactorType: string(s.RiskFactorType), QuestionKey: answer.QuestionKey, ValueType: valueType, FileID: &fileID, CreatedAt: qa.CreatedAt})
			}
		}
	}
	return result
}

func fromQuestionJudgements(items []riskfactor.QuestionJudgement) JSONSlice {
	if len(items) == 0 {
		return nil
	}
	result := make(JSONSlice, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]interface{}{"question_key": item.QuestionKey, "required": item.Required, "completeness": item.Completeness, "reasonableness": item.Reasonableness, "note": item.Note})
	}
	return result
}

func toQuestionJudgements(items JSONSlice) []riskfactor.QuestionJudgement {
	result := make([]riskfactor.QuestionJudgement, 0, len(items))
	for _, item := range items {
		q := riskfactor.QuestionJudgement{}
		q.QuestionKey, _ = item["question_key"].(string)
		q.Required, _ = item["required"].(bool)
		q.Completeness, _ = item["completeness"].(bool)
		q.Reasonableness, _ = item["reasonableness"].(bool)
		q.Note, _ = item["note"].(string)
		result = append(result, q)
	}
	return result
}

func boolValue(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
