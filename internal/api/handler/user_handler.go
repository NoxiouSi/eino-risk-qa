package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/NoxiouSi/eino-risk-qa/internal/api/dto"
	"github.com/NoxiouSi/eino-risk-qa/internal/application"
	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

// UserHandler 处理 /api/v1/users/* 相关接口：按用户查询主问题列表。
type UserHandler struct {
	userSvc *application.UserAppService
}

// NewUserHandler 创建 Handler 实例。
func NewUserHandler(userSvc *application.UserAppService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

// GetMainQuestions 处理 GET /api/v1/users/{user_id}/main-questions。
func (h *UserHandler) GetMainQuestions(ctx context.Context, c *app.RequestContext) {
	log := logging.FromContext(ctx)
	userID := c.Param("user_id")
	log.Info("get main questions: received", "user_id", userID)

	result, err := h.userSvc.GetMainQuestions(ctx, userID)
	if err != nil {
		if errors.Is(err, application.ErrUserNotFound) {
			log.Warn("get main questions: user not found", "user_id", userID)
			writeError(c, http.StatusNotFound, CodeUserNotFound, "user not found")
			return
		}
		log.Error("get main questions: application service failed", "user_id", userID, "error", err.Error())
		writeError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}

	items := make([]dto.MainQuestionItem, 0, len(result.Items))
	for _, item := range result.Items {
		questions := make([]dto.QuestionItem, 0, len(item.Questions))
		for _, question := range item.Questions {
			questions = append(questions, dto.QuestionItem{QuestionKey: question.QuestionKey, QuestionText: question.QuestionText, AnswerType: question.AnswerType, Required: question.Required, MinSubmitCount: question.MinSubmitCount, SortOrder: question.SortOrder})
		}
		items = append(items, dto.MainQuestionItem{RiskFactorType: string(item.RiskFactorType), MainQuestion: item.MainQuestion, Questions: questions})
	}
	log.Info("get main questions: succeeded", "user_id", userID, "item_count", len(items))
	c.JSON(consts.StatusOK, dto.MainQuestionsResponse{UserID: result.UserID, Items: items})
}
