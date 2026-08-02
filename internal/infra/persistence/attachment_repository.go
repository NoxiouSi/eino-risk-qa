package persistence

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/NoxiouSi/eino-risk-qa/internal/application"
)

var ErrUploadedFileNotFound = errors.New("persistence: uploaded file not found")

type GORMAttachmentRepository struct {
	db *gorm.DB
}

func NewGORMAttachmentRepository(db *gorm.DB) *GORMAttachmentRepository {
	return &GORMAttachmentRepository{db: db}
}

var _ application.AttachmentRepository = (*GORMAttachmentRepository)(nil)

func (r *GORMAttachmentRepository) Create(ctx context.Context, file application.UploadedFile) error {
	return r.db.WithContext(ctx).Create(&UploadedFileModel{FileID: file.FileID, UserID: file.UserID, RiskFactorType: file.RiskFactorType, QuestionKey: file.QuestionKey, OriginalName: file.OriginalName, StoredPath: file.StoredPath, MIMEType: file.MIMEType, SizeBytes: file.SizeBytes, SHA256: file.SHA256, CreatedAt: file.CreatedAt}).Error
}

func (r *GORMAttachmentRepository) CountOwned(ctx context.Context, userID, riskFactorType, questionKey string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&UploadedFileModel{}).Where("user_id = ? AND risk_factor_type = ? AND question_key = ?", userID, riskFactorType, questionKey).Count(&count).Error
	return count, err
}

func (r *GORMAttachmentRepository) FindOwned(ctx context.Context, fileID, userID, riskFactorType, questionKey string) (*application.UploadedFile, error) {
	var model UploadedFileModel
	err := r.db.WithContext(ctx).Where("file_id = ? AND user_id = ? AND risk_factor_type = ? AND question_key = ?", fileID, userID, riskFactorType, questionKey).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUploadedFileNotFound
	}
	if err != nil {
		return nil, err
	}
	return &application.UploadedFile{FileID: model.FileID, UserID: model.UserID, RiskFactorType: model.RiskFactorType, QuestionKey: model.QuestionKey, OriginalName: model.OriginalName, StoredPath: model.StoredPath, MIMEType: model.MIMEType, SizeBytes: model.SizeBytes, SHA256: model.SHA256, CreatedAt: model.CreatedAt}, nil
}
