package handler

import (
	"github.com/NoxiouSi/eino-risk-qa/internal/api/dto"
	"github.com/NoxiouSi/eino-risk-qa/internal/application"
	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

// toSessionResultDTO 将 application.SessionResult 映射为批量提交/追问提交响应中的单项结构（不含history）。
func toQuestionJudgementPayloads(items []riskfactor.QuestionJudgement) []dto.QuestionJudgementPayload {
	result := make([]dto.QuestionJudgementPayload, 0, len(items))
	for _, item := range items {
		result = append(result, dto.QuestionJudgementPayload{QuestionKey: item.QuestionKey, Required: item.Required, Completeness: item.Completeness, Reasonableness: item.Reasonableness, Note: item.Note})
	}
	return result
}

func toSessionResultDTO(r application.SessionResult) dto.SessionResult {
	var terminationReason *string
	if r.TerminationReason != nil {
		v := string(*r.TerminationReason)
		terminationReason = &v
	}
	var errPayload *dto.ErrorPayload
	if r.Error != nil {
		errPayload = &dto.ErrorPayload{Code: r.Error.Code, Message: r.Error.Message}
	}
	return dto.SessionResult{
		SessionID:           r.SessionID,
		RiskFactorType:      string(r.RiskFactorType),
		Status:              string(r.Status),
		CurrentRound:        r.CurrentRound,
		Message:             r.Message,
		Cleared:             r.Cleared,
		TerminationReason:   terminationReason,
		ExtractedInfo:       nonNilMap(r.ExtractedInfo),
		MissingQuestionKeys: append([]string(nil), r.MissingQuestionKeys...),
		QuestionJudgements:  toQuestionJudgementPayloads(r.QuestionJudgements),
		Error:               errPayload,
	}
}

// toSessionDetailDTO 将 application.SessionResult 映射为会话详情/批次查询响应中的单项结构（含history）。
func toQuestionItemDTOs(questions []application.QuestionNode) []dto.QuestionItem {
	result := make([]dto.QuestionItem, 0, len(questions))
	for _, question := range questions {
		result = append(result, dto.QuestionItem{
			QuestionKey: question.QuestionKey, QuestionText: question.QuestionText,
			AnswerType: question.AnswerType, Required: question.Required,
			MinSubmitCount: question.MinSubmitCount, SortOrder: question.SortOrder,
		})
	}
	return result
}

func toSessionDetailDTO(r application.SessionResult) dto.SessionDetail {
	var terminationReason *string
	if r.TerminationReason != nil {
		v := string(*r.TerminationReason)
		terminationReason = &v
	}
	var errPayload *dto.ErrorPayload
	if r.Error != nil {
		errPayload = &dto.ErrorPayload{Code: r.Error.Code, Message: r.Error.Message}
	}
	history := make([]dto.QAPairPayload, 0, len(r.History))
	for _, qa := range r.History {
		history = append(history, dto.QAPairPayload{
			Round:              qa.Round,
			Question:           qa.Question,
			Answer:             qa.Answer,
			Completeness:       qa.Completeness,
			Reasonableness:     qa.Reasonableness,
			QuestionJudgements: toQuestionJudgementPayloads(qa.Judgements),
		})
	}
	return dto.SessionDetail{
		SessionID:           r.SessionID,
		RiskFactorType:      string(r.RiskFactorType),
		MainQuestion:        r.MainQuestion,
		Questions:           toQuestionItemDTOs(r.Questions),
		Status:              string(r.Status),
		CurrentRound:        r.CurrentRound,
		MaxRounds:           r.MaxRounds,
		Message:             r.Message,
		Cleared:             r.Cleared,
		TerminationReason:   terminationReason,
		ExtractedInfo:       nonNilMap(r.ExtractedInfo),
		MissingQuestionKeys: append([]string(nil), r.MissingQuestionKeys...),
		QuestionJudgements:  toQuestionJudgementPayloads(r.QuestionJudgements),
		History:             history,
		Error:               errPayload,
	}
}

func nonNilMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	return m
}

func parseRiskFactorType(s string) riskfactor.RiskFactorType {
	return riskfactor.RiskFactorType(s)
}
