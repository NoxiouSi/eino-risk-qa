package persistence

import (
	"context"
	"fmt"
	"sort"

	"gorm.io/gorm"

	"github.com/NoxiouSi/eino-risk-qa/internal/application"
	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

// GORMMainQuestionRepository 实现统一风险问题目录。
type GORMMainQuestionRepository struct {
	db *gorm.DB
}

func NewGORMMainQuestionRepository(db *gorm.DB) *GORMMainQuestionRepository {
	return &GORMMainQuestionRepository{db: db}
}

var _ application.RiskFactorQuestionCatalog = (*GORMMainQuestionRepository)(nil)

type questionSkillRow struct {
	QuestionID uint64
	SkillKey   string
	SkillName  string
	RuleText   string
	SortOrder  int
}

func (r *GORMMainQuestionRepository) FindQuestionTrees(ctx context.Context, riskFactorTypes []string) (map[string]application.QuestionTree, error) {
	if len(riskFactorTypes) == 0 {
		return map[string]application.QuestionTree{}, nil
	}
	questions, err := r.loadQuestions(ctx, riskFactorTypes)
	if err != nil {
		return nil, err
	}
	refs, err := r.loadSkillRefs(ctx, questions)
	if err != nil {
		return nil, err
	}
	return assembleQuestionTrees(questions, refs)
}

func (r *GORMMainQuestionRepository) loadQuestions(ctx context.Context, riskFactorTypes []string) ([]RiskFactorQuestionModel, error) {
	var questions []RiskFactorQuestionModel
	err := r.db.WithContext(ctx).Where("risk_factor_type IN ? AND enabled = ?", riskFactorTypes, true).Order("risk_factor_type, sort_order, id").Find(&questions).Error
	if err != nil {
		logging.FromContext(ctx).Error("find question trees: query questions failed", "error", err.Error())
	}
	return questions, err
}

func (r *GORMMainQuestionRepository) loadSkillRefs(ctx context.Context, questions []RiskFactorQuestionModel) (map[uint64][]application.SkillSpec, error) {
	refs := make(map[uint64][]application.SkillSpec)
	ids := make([]uint64, 0, len(questions))
	for _, question := range questions {
		ids = append(ids, question.ID)
	}
	if len(ids) == 0 {
		return refs, nil
	}
	var rows []questionSkillRow
	err := r.db.WithContext(ctx).Table("question_skill_refs AS r").Select("r.question_id, s.skill_key, s.name AS skill_name, s.rule_text, r.sort_order").Joins("JOIN audit_skills AS s ON s.id = r.skill_id AND s.enabled = ?", true).Where("r.question_id IN ?", ids).Order("r.question_id, r.sort_order, r.id").Scan(&rows).Error
	if err != nil {
		logging.FromContext(ctx).Error("find question trees: query skills failed", "error", err.Error())
		return nil, err
	}
	for _, row := range rows {
		refs[row.QuestionID] = append(refs[row.QuestionID], application.SkillSpec{SkillKey: row.SkillKey, Name: row.SkillName, RuleText: row.RuleText})
	}
	return refs, nil
}

func assembleQuestionTrees(questions []RiskFactorQuestionModel, refs map[uint64][]application.SkillSpec) (map[string]application.QuestionTree, error) {
	nodes := make(map[uint64]application.QuestionNode, len(questions))
	roots := make(map[string]uint64)
	children := make(map[uint64][]application.QuestionNode)
	for _, question := range questions {
		node := application.QuestionNode{ID: question.ID, RiskFactorType: question.RiskFactorType, QuestionKey: question.QuestionKey, ParentID: question.ParentID, QuestionText: question.QuestionText, AnswerType: question.AnswerType, Required: question.Required, MinSubmitCount: question.MinSubmitCount, MaxSubmitCount: question.MaxSubmitCount, SortOrder: question.SortOrder, Skills: refs[question.ID]}
		nodes[question.ID] = node
		if err := classifyQuestion(question, node, roots, children); err != nil {
			return nil, err
		}
	}
	if err := validateQuestionParents(nodes, children); err != nil {
		return nil, err
	}
	return buildQuestionTreeResult(nodes, roots, children)
}

func classifyQuestion(question RiskFactorQuestionModel, node application.QuestionNode, roots map[string]uint64, children map[uint64][]application.QuestionNode) error {
	if question.ParentID == nil && question.AnswerType == "group" {
		if _, exists := roots[question.RiskFactorType]; exists {
			return fmt.Errorf("multiple enabled root questions for risk factor %s", question.RiskFactorType)
		}
		roots[question.RiskFactorType] = question.ID
		return nil
	}
	if question.ParentID == nil || question.AnswerType == "group" {
		return fmt.Errorf("invalid question hierarchy for %s", question.QuestionKey)
	}
	if len(node.Skills) == 0 {
		return fmt.Errorf("enabled question %s has no enabled audit skill", question.QuestionKey)
	}
	children[*question.ParentID] = append(children[*question.ParentID], node)
	return nil
}

func validateQuestionParents(nodes map[uint64]application.QuestionNode, children map[uint64][]application.QuestionNode) error {
	for parentID, childNodes := range children {
		parent, ok := nodes[parentID]
		if !ok || parent.AnswerType != "group" || parent.ParentID != nil {
			return fmt.Errorf("orphan or invalid parent question id %d", parentID)
		}
		for _, child := range childNodes {
			if child.RiskFactorType != parent.RiskFactorType {
				return fmt.Errorf("question %s belongs to a different risk factor than its parent", child.QuestionKey)
			}
		}
	}
	return nil
}

func buildQuestionTreeResult(nodes map[uint64]application.QuestionNode, roots map[string]uint64, children map[uint64][]application.QuestionNode) (map[string]application.QuestionTree, error) {
	result := make(map[string]application.QuestionTree, len(roots))
	for riskType, rootID := range roots {
		root := nodes[rootID]
		root.Children = children[rootID]
		if len(root.Children) == 0 {
			return nil, fmt.Errorf("root question for risk factor %s has no enabled children", riskType)
		}
		sort.SliceStable(root.Children, func(i, j int) bool { return root.Children[i].SortOrder < root.Children[j].SortOrder })
		result[riskType] = application.QuestionTree{RiskFactorType: riskType, Root: root}
	}
	return result, nil
}
