package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/NoxiouSi/eino-risk-qa/internal/application"
	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

// GORMMainQuestionRepository 实现 application.MainQuestionCatalog：查询
// risk_factor_main_questions 表，该表是风险要素类型到主问题的全局固定映射（所有用户共用），
// 无状态机、无业务规则，因此不下沉到 domain 层。
type GORMMainQuestionRepository struct {
	db *gorm.DB
}

// NewGORMMainQuestionRepository 创建实例。
func NewGORMMainQuestionRepository(db *gorm.DB) *GORMMainQuestionRepository {
	return &GORMMainQuestionRepository{db: db}
}

var _ application.MainQuestionCatalog = (*GORMMainQuestionRepository)(nil)

// FindMainQuestions 按给定的风险要素类型批量查询对应主问题，返回 riskFactorType -> mainQuestion 映射；
// 若给定类型列表为空，直接返回空 map，不查询数据库。
func (r *GORMMainQuestionRepository) FindMainQuestions(ctx context.Context, riskFactorTypes []string) (map[string]string, error) {
	log := logging.FromContext(ctx)
	result := make(map[string]string, len(riskFactorTypes))
	if len(riskFactorTypes) == 0 {
		return result, nil
	}

	var rows []RiskFactorMainQuestionModel
	if err := r.db.WithContext(ctx).Where("risk_factor_type IN ?", riskFactorTypes).Find(&rows).Error; err != nil {
		log.Error("find main questions: query failed", "error", err.Error())
		return nil, err
	}
	for _, row := range rows {
		result[row.RiskFactorType] = row.MainQuestion
	}
	return result, nil
}
