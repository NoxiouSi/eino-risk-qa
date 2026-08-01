package application

import (
	"context"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

// UserAppService 编排"查询用户主问题"这一只读用例：先按 user_id 查用户拿到其预配置的
// RiskFactorTypes，再按这些类型批量查询全局固定的主问题映射表，按用户配置顺序组装返回。
// 不涉及状态机/事务，属于简单配置查询，因此不依赖 domain 层的聚合根或端口。
type UserAppService struct {
	userRepo UserBatchRepository
	catalog  MainQuestionCatalog
}

// NewUserAppService 创建应用服务实例。
func NewUserAppService(userRepo UserBatchRepository, catalog MainQuestionCatalog) *UserAppService {
	return &UserAppService{userRepo: userRepo, catalog: catalog}
}

// GetMainQuestions 查询指定用户拥有的风险项及各自对应的主问题。
// 用户不存在时返回 ErrUserNotFound（供 api 层转换为 404 USER_NOT_FOUND）；
// 若用户已配置的某个风险要素类型在映射表中缺失对应主问题，该类型会被跳过（不返回半成品项）。
func (s *UserAppService) GetMainQuestions(ctx context.Context, userID string) (MainQuestionsResult, error) {
	log := logging.FromContext(ctx).With("user_id", userID)
	log.Info("get main questions: start")

	user, err := s.userRepo.FindUser(ctx, userID)
	if err != nil {
		log.Warn("get main questions: find user failed", "error", err.Error())
		return MainQuestionsResult{}, err
	}

	questions, err := s.catalog.FindMainQuestions(ctx, user.RiskFactorTypes)
	if err != nil {
		log.Error("get main questions: find main questions failed", "error", err.Error())
		return MainQuestionsResult{}, err
	}

	items := make([]MainQuestionItem, 0, len(user.RiskFactorTypes))
	for _, t := range user.RiskFactorTypes {
		q, ok := questions[t]
		if !ok {
			log.Warn("get main questions: main question missing for risk factor type", "risk_factor_type", t)
			continue
		}
		items = append(items, MainQuestionItem{RiskFactorType: riskfactor.RiskFactorType(t), MainQuestion: q})
	}

	log.Info("get main questions: succeeded", "item_count", len(items))
	return MainQuestionsResult{UserID: userID, Items: items}, nil
}
