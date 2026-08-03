package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/google/uuid"

	"github.com/NoxiouSi/eino-risk-qa/internal/application"
	"github.com/NoxiouSi/eino-risk-qa/internal/config"
)

const (
	mimeJPEG = "image/jpeg"
	mimePNG  = "image/png"
	mimeWebP = "image/webp"
)

type AttachmentHandler struct {
	repo    application.AttachmentRepository
	catalog application.RiskFactorQuestionCatalog
	cfg     config.StorageConfig
}

func NewAttachmentHandler(repo application.AttachmentRepository, cfg config.StorageConfig, catalogs ...application.RiskFactorQuestionCatalog) *AttachmentHandler {
	h := &AttachmentHandler{repo: repo, cfg: cfg}
	if len(catalogs) > 0 {
		h.catalog = catalogs[0]
	}
	return h
}

type attachmentResponse struct {
	FileID       string `json:"file_id"`
	OriginalName string `json:"original_name"`
	MIMEType     string `json:"mime_type"`
	SizeBytes    int64  `json:"size_bytes"`
}

func (h *AttachmentHandler) Upload(ctx context.Context, c *app.RequestContext) {
	userID := strings.TrimSpace(string(c.FormValue("user_id")))
	riskType := strings.TrimSpace(string(c.FormValue("risk_factor_type")))
	questionKey := strings.TrimSpace(string(c.FormValue("question_key")))
	if userID == "" || riskType == "" || questionKey == "" {
		writeError(c, http.StatusBadRequest, CodeInvalidParam, "user_id, risk_factor_type and question_key are required")
		return
	}
	if status, code, message := h.validateUploadTarget(ctx, riskType, questionKey); status != 0 {
		writeError(c, status, code, message)
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil || fileHeader == nil {
		writeError(c, http.StatusBadRequest, CodeInvalidParam, "file is required")
		return
	}
	if fileHeader.Size <= 0 || fileHeader.Size > h.cfg.MaxFileBytes {
		writeError(c, http.StatusBadRequest, CodeInvalidParam, "file size exceeds limit")
		return
	}
	data, mimeType, err := readAndValidateImage(fileHeader, h.cfg)
	if err != nil {
		writeError(c, http.StatusBadRequest, CodeInvalidParam, err.Error())
		return
	}
	fileID := uuid.NewString()
	ext := extensionForMIME(mimeType)
	day := time.Now().UTC().Format("2006/01/02")
	relativePath := filepath.Join(day, fileID+ext)
	absolutePath := filepath.Join(h.cfg.LocalDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0700); err != nil {
		writeError(c, http.StatusInternalServerError, CodeInternalError, "create upload directory failed")
		return
	}
	if err := os.WriteFile(absolutePath, data, 0600); err != nil {
		writeError(c, http.StatusInternalServerError, CodeInternalError, "save upload failed")
		return
	}
	digest := sha256.Sum256(data)
	meta := application.UploadedFile{FileID: fileID, UserID: userID, RiskFactorType: riskType, QuestionKey: questionKey, OriginalName: filepath.Base(fileHeader.Filename), StoredPath: relativePath, MIMEType: mimeType, SizeBytes: int64(len(data)), SHA256: hex.EncodeToString(digest[:]), CreatedAt: time.Now().UTC()}
	if err := h.repo.Create(ctx, meta); err != nil {
		_ = os.Remove(absolutePath)
		writeError(c, http.StatusInternalServerError, CodeInternalError, "save upload metadata failed")
		return
	}
	c.JSON(consts.StatusCreated, attachmentResponse{FileID: fileID, OriginalName: meta.OriginalName, MIMEType: mimeType, SizeBytes: meta.SizeBytes})
}

func (h *AttachmentHandler) validateUploadTarget(ctx context.Context, riskType, questionKey string) (int, string, string) {
	if h.catalog == nil {
		return 0, "", ""
	}
	valid, err := h.isUploadQuestion(ctx, riskType, questionKey)
	if err != nil {
		return http.StatusInternalServerError, CodeInternalError, "load question configuration failed"
	}
	if !valid {
		return http.StatusBadRequest, CodeInvalidParam, "question_key is not an enabled image or file question"
	}
	return 0, "", ""
}

func (h *AttachmentHandler) isUploadQuestion(ctx context.Context, riskType, questionKey string) (bool, error) {
	trees, err := h.catalog.FindQuestionTrees(ctx, []string{riskType})
	if err != nil {
		return false, err
	}
	tree, ok := trees[riskType]
	if !ok {
		return false, nil
	}
	for _, question := range tree.Root.Children {
		if question.QuestionKey == questionKey {
			return question.AnswerType == "image" || question.AnswerType == "file", nil
		}
	}
	return false, nil
}

func readAndValidateImage(header *multipart.FileHeader, cfg config.StorageConfig) ([]byte, string, error) {
	file, err := header.Open()
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, cfg.MaxFileBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > cfg.MaxFileBytes {
		return nil, "", fmt.Errorf("file size exceeds limit")
	}
	inputMIMEType := http.DetectContentType(data)
	if !extensionMatchesMIME(strings.ToLower(filepath.Ext(header.Filename)), inputMIMEType) {
		return nil, "", fmt.Errorf("file extension does not match image content")
	}
	if !slices.Contains(cfg.AllowedMIMETypes, inputMIMEType) {
		return nil, "", fmt.Errorf("unsupported image type")
	}
	targetBytes := cfg.MaxStoredImageBytes
	if targetBytes <= 0 {
		targetBytes = 1024 * 1024
	}
	compressed, err := compressImageToJPEG(data, targetBytes)
	if err != nil {
		return nil, "", err
	}
	return compressed, mimeJPEG, nil
}

func extensionMatchesMIME(ext, mimeType string) bool {
	switch mimeType {
	case mimeJPEG:
		return ext == ".jpg" || ext == ".jpeg"
	case mimePNG:
		return ext == ".png"
	case mimeWebP:
		return ext == ".webp"
	default:
		return false
	}
}

func extensionForMIME(mimeType string) string {
	switch mimeType {
	case mimeJPEG:
		return ".jpg"
	case mimePNG:
		return ".png"
	case mimeWebP:
		return ".webp"
	default:
		return ""
	}
}
