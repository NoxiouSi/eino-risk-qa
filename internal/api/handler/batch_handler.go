package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/NoxiouSi/eino-risk-qa/internal/api/dto"
	"github.com/NoxiouSi/eino-risk-qa/internal/application"
)

// BatchHandler 处理 /api/v1/batches 相关的两个接口：批量首轮提交、批次查询。
type BatchHandler struct {
	batchSvc *application.BatchAppService
}

// NewBatchHandler 创建 Handler 实例。
func NewBatchHandler(batchSvc *application.BatchAppService) *BatchHandler {
	return &BatchHandler{batchSvc: batchSvc}
}

// SubmitBatch 处理 POST /api/v1/batches。
func (h *BatchHandler) SubmitBatch(ctx context.Context, c *app.RequestContext) {
	var req dto.BatchRequest
	if err := c.BindAndValidate(&req); err != nil {
		writeError(c, http.StatusBadRequest, CodeInvalidParam, "invalid request body")
		return
	}
	if err := validateBatchRequest(req); err != nil {
		writeError(c, http.StatusBadRequest, CodeInvalidParam, err.Error())
		return
	}

	input := toSubmitBatchInput(req)

	if !req.Stream {
		result, err := h.batchSvc.SubmitBatch(ctx, input)
		if err != nil {
			writeError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
			return
		}
		c.JSON(consts.StatusOK, toBatchResponse(result))
		return
	}

	pr, pw := io.Pipe()
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.SetStatusCode(consts.StatusOK)
	c.Response.SetBodyStream(pr, -1)

	go func() {
		defer pw.Close()
		h.batchSvc.SubmitBatchStream(ctx, input, newSSEForwarder(pw))
	}()
}

// GetBatch 处理 GET /api/v1/batches/{batch_id}。
func (h *BatchHandler) GetBatch(ctx context.Context, c *app.RequestContext) {
	batchID := c.Param("batch_id")
	result, err := h.batchSvc.GetBatch(ctx, batchID)
	if err != nil {
		if errors.Is(err, application.ErrBatchNotFound) {
			writeError(c, http.StatusNotFound, CodeBatchNotFound, "batch not found")
			return
		}
		writeError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}

	resp := dto.BatchResponse{BatchID: result.BatchID, CreatedAt: result.CreatedAt.Format(rfc3339)}
	for _, r := range result.Results {
		resp.Sessions = append(resp.Sessions, toSessionDetailDTO(r))
	}
	c.JSON(consts.StatusOK, resp)
}

func validateBatchRequest(req dto.BatchRequest) error {
	if req.User.UserID == "" {
		return invalidParamError("user.user_id is required")
	}
	if len(req.RiskFactors) == 0 {
		return invalidParamError("risk_factors must not be empty")
	}
	for _, rf := range req.RiskFactors {
		if rf.RiskFactorType == "" || rf.MainQuestion == "" || rf.Answer == "" {
			return invalidParamError("risk_factor_type, main_question and answer are required for every risk factor")
		}
	}
	return nil
}

func toSubmitBatchInput(req dto.BatchRequest) application.SubmitBatchInput {
	input := application.SubmitBatchInput{
		UserID:   req.User.UserID,
		UserName: req.User.Name,
	}
	for _, rf := range req.RiskFactors {
		input.RiskFactors = append(input.RiskFactors, application.RiskFactorInput{
			RiskFactorType: parseRiskFactorType(rf.RiskFactorType),
			MainQuestion:   rf.MainQuestion,
			Answer:         rf.Answer,
		})
	}
	return input
}

func toBatchResponse(result application.BatchResult) dto.BatchResponse {
	resp := dto.BatchResponse{BatchID: result.BatchID, CreatedAt: result.CreatedAt.Format(rfc3339)}
	for _, r := range result.Results {
		resp.Results = append(resp.Results, toSessionResultDTO(r))
	}
	return resp
}

const rfc3339 = "2006-01-02T15:04:05Z07:00"

type invalidParamError string

func (e invalidParamError) Error() string { return string(e) }
