package application_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/NoxiouSi/eino-risk-qa/internal/application"
	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

// fakeJudger 是一个可编程的 RiskJudger fake：按 JudgeInput.LatestAnswer 关键字返回预设结果，
// 或返回预设错误，用于在不依赖真实/mock ChatModel 的情况下测试 application 层编排逻辑。
type fakeJudger struct {
	mu        sync.Mutex
	responses map[string]*riskfactor.JudgementResult // 按 LatestAnswer 匹配
	errs      map[string]error
	calls     int
	inputs    []riskfactor.JudgeInput
}

func newFakeJudger() *fakeJudger {
	return &fakeJudger{
		responses: map[string]*riskfactor.JudgementResult{},
		errs:      map[string]error{},
	}
}

func (f *fakeJudger) Judge(ctx context.Context, input riskfactor.JudgeInput) (*riskfactor.JudgementResult, error) {
	f.mu.Lock()
	f.calls++
	f.inputs = append(f.inputs, input)
	f.mu.Unlock()
	if err, ok := f.errs[input.LatestAnswer]; ok {
		return nil, err
	}
	if r, ok := f.responses[input.LatestAnswer]; ok {
		return r, nil
	}
	return &riskfactor.JudgementResult{Completeness: true, Reasonableness: true}, nil
}

func (f *fakeJudger) JudgeStream(ctx context.Context, input riskfactor.JudgeInput) (<-chan riskfactor.JudgeStreamEvent, error) {
	panic("not used in application layer tests")
}

var _ riskfactor.RiskJudger = (*fakeJudger)(nil)

// fakeSessionRepository 是一个内存实现的 SessionRepository，用于隔离测试 application 层
// 而不依赖真实数据库；行为需与 GORMSessionRepository 的关键契约保持一致（Save覆盖式更新、
// FindByID 未找到返回 riskfactor.ErrSessionNotFound）。
type fakeSessionRepository struct {
	mu       sync.Mutex
	sessions map[string]*riskfactor.RiskFactorSession
}

func newFakeSessionRepository() *fakeSessionRepository {
	return &fakeSessionRepository{sessions: map[string]*riskfactor.RiskFactorSession{}}
}

func (r *fakeSessionRepository) Save(ctx context.Context, s *riskfactor.RiskFactorSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[s.ID] = s
	return nil
}

func (r *fakeSessionRepository) FindByID(ctx context.Context, sessionID string) (*riskfactor.RiskFactorSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[sessionID]
	if !ok {
		return nil, riskfactor.ErrSessionNotFound
	}
	return s, nil
}

func (r *fakeSessionRepository) FindByBatchID(ctx context.Context, batchID string) ([]*riskfactor.RiskFactorSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []*riskfactor.RiskFactorSession
	for _, s := range r.sessions {
		if s.BatchID == batchID {
			result = append(result, s)
		}
	}
	return result, nil
}

var _ riskfactor.SessionRepository = (*fakeSessionRepository)(nil)

// fakeUserBatchRepository 内存实现的 UserBatchRepository。
type fakeUserBatchRepository struct {
	mu      sync.Mutex
	users   map[string]application.User
	batches map[string]application.Batch
}

func newFakeUserBatchRepository() *fakeUserBatchRepository {
	return &fakeUserBatchRepository{
		users:   map[string]application.User{},
		batches: map[string]application.Batch{},
	}
}

func (r *fakeUserBatchRepository) EnsureUser(ctx context.Context, u application.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[u.UserID]; !ok {
		r.users[u.UserID] = u
	}
	return nil
}

func (r *fakeUserBatchRepository) CreateBatch(ctx context.Context, b application.Batch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches[b.BatchID] = b
	return nil
}

func (r *fakeUserBatchRepository) FindBatch(ctx context.Context, batchID string) (*application.Batch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.batches[batchID]
	if !ok {
		return nil, application.ErrBatchNotFound
	}
	return &b, nil
}

func (r *fakeUserBatchRepository) FindUser(ctx context.Context, userID string) (*application.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[userID]
	if !ok {
		return nil, application.ErrUserNotFound
	}
	return &u, nil
}

var _ application.UserBatchRepository = (*fakeUserBatchRepository)(nil)

// sequentialIDGenerator 生成可预测、递增的 ID，便于测试断言。
type sequentialIDGenerator struct {
	mu         sync.Mutex
	batchSeq   int
	sessionSeq int
}

func newSequentialIDGenerator() *sequentialIDGenerator {
	return &sequentialIDGenerator{}
}

func (g *sequentialIDGenerator) NewBatchID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.batchSeq++
	return fmt.Sprintf("batch_%d", g.batchSeq)
}

func (g *sequentialIDGenerator) NewSessionID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sessionSeq++
	return fmt.Sprintf("sess_%d", g.sessionSeq)
}

var _ application.IDGenerator = (*sequentialIDGenerator)(nil)

// fakeMainQuestionCatalog 内存实现的 MainQuestionCatalog，按预设的 riskFactorType -> mainQuestion
// 映射返回结果。
type fakeMainQuestionCatalog struct {
	questions map[string]string
	trees     map[string]application.QuestionTree
}

func newFakeMainQuestionCatalog() *fakeMainQuestionCatalog {
	return &fakeMainQuestionCatalog{questions: map[string]string{}, trees: map[string]application.QuestionTree{}}
}

func (c *fakeMainQuestionCatalog) FindQuestionTrees(ctx context.Context, riskFactorTypes []string) (map[string]application.QuestionTree, error) {
	result := make(map[string]application.QuestionTree, len(riskFactorTypes))
	for _, t := range riskFactorTypes {
		if tree, ok := c.trees[t]; ok {
			result[t] = tree
			continue
		}
		if q, ok := c.questions[t]; ok {
			result[t] = application.QuestionTree{RiskFactorType: t, Root: application.QuestionNode{RiskFactorType: t, QuestionKey: t + "_main", QuestionText: q, AnswerType: "group"}}
		}
	}
	return result, nil
}

var _ application.RiskFactorQuestionCatalog = (*fakeMainQuestionCatalog)(nil)

type fakeAttachmentRepository struct {
	files map[string]application.UploadedFile
}

func newFakeAttachmentRepository() *fakeAttachmentRepository {
	return &fakeAttachmentRepository{files: map[string]application.UploadedFile{}}
}

func (r *fakeAttachmentRepository) Create(ctx context.Context, file application.UploadedFile) error {
	r.files[file.FileID] = file
	return nil
}

func (r *fakeAttachmentRepository) FindOwned(ctx context.Context, fileID, userID, riskFactorType, questionKey string) (*application.UploadedFile, error) {
	file, ok := r.files[fileID]
	if !ok || file.UserID != userID || file.RiskFactorType != riskFactorType || file.QuestionKey != questionKey {
		return nil, fmt.Errorf("file not found")
	}
	return &file, nil
}

var _ application.AttachmentRepository = (*fakeAttachmentRepository)(nil)
