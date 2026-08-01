package handler_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/NoxiouSi/eino-risk-qa/internal/application"
	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

// fakeJudger 是可编程的 RiskJudger fake，按 LatestAnswer 关键字返回预设结果/错误，
// 用于 api 层 handler 测试中驱动完整链路（不依赖真实数据库/LLM）。
type fakeJudger struct {
	mu        sync.Mutex
	responses map[string]*riskfactor.JudgementResult
	errs      map[string]error
}

func newFakeJudger() *fakeJudger {
	return &fakeJudger{responses: map[string]*riskfactor.JudgementResult{}, errs: map[string]error{}}
}

func (f *fakeJudger) Judge(ctx context.Context, input riskfactor.JudgeInput) (*riskfactor.JudgementResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.errs[input.LatestAnswer]; ok {
		return nil, err
	}
	if r, ok := f.responses[input.LatestAnswer]; ok {
		return r, nil
	}
	return &riskfactor.JudgementResult{Completeness: true, Reasonableness: true}, nil
}

func (f *fakeJudger) JudgeStream(ctx context.Context, input riskfactor.JudgeInput) (<-chan riskfactor.JudgeStreamEvent, error) {
	f.mu.Lock()
	err, hasErr := f.errs[input.LatestAnswer]
	result, ok := f.responses[input.LatestAnswer]
	f.mu.Unlock()
	if hasErr {
		return nil, err
	}
	if !ok {
		result = &riskfactor.JudgementResult{Completeness: true, Reasonableness: true}
	}

	events := make(chan riskfactor.JudgeStreamEvent, 8)
	go func() {
		defer close(events)
		if result.FollowUpQuestion != "" {
			for _, r := range result.FollowUpQuestion {
				events <- riskfactor.JudgeStreamEvent{SessionID: input.SessionID, Type: riskfactor.StreamEventMessageDelta, MessageDelta: string(r)}
			}
		} else if result.Completeness {
			events <- riskfactor.JudgeStreamEvent{SessionID: input.SessionID, Type: riskfactor.StreamEventMessageDelta, MessageDelta: riskfactor.ClosingMessage}
		}
		events <- riskfactor.JudgeStreamEvent{SessionID: input.SessionID, Type: riskfactor.StreamEventResult, Result: result}
	}()
	return events, nil
}

var _ riskfactor.RiskJudger = (*fakeJudger)(nil)

// fakeSessionRepository 内存实现，行为与真实实现的关键契约保持一致。
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

// fakeUserBatchRepository 内存实现。
type fakeUserBatchRepository struct {
	mu      sync.Mutex
	batches map[string]application.Batch
	users   map[string]application.User
}

func newFakeUserBatchRepository() *fakeUserBatchRepository {
	return &fakeUserBatchRepository{batches: map[string]application.Batch{}, users: map[string]application.User{}}
}

func (r *fakeUserBatchRepository) EnsureUser(ctx context.Context, u application.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[u.UserID]; !ok {
		r.users[u.UserID] = u
	}
	return nil
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

var _ application.UserBatchRepository = (*fakeUserBatchRepository)(nil)

// fakeMainQuestionCatalog 内存实现的 MainQuestionCatalog，按预设的 riskFactorType -> mainQuestion
// 映射返回结果，用于 api/application 层测试中隔离真实数据库。
type fakeMainQuestionCatalog struct {
	questions map[string]string
}

func newFakeMainQuestionCatalog() *fakeMainQuestionCatalog {
	return &fakeMainQuestionCatalog{questions: map[string]string{}}
}

func (c *fakeMainQuestionCatalog) FindMainQuestions(ctx context.Context, riskFactorTypes []string) (map[string]string, error) {
	result := make(map[string]string, len(riskFactorTypes))
	for _, t := range riskFactorTypes {
		if q, ok := c.questions[t]; ok {
			result[t] = q
		}
	}
	return result, nil
}

var _ application.MainQuestionCatalog = (*fakeMainQuestionCatalog)(nil)

// sequentialIDGenerator 生成可预测的 ID，便于测试断言。
type sequentialIDGenerator struct {
	mu   sync.Mutex
	seq  int
	kind string
}

func newSequentialIDGenerator() *sequentialIDGenerator {
	return &sequentialIDGenerator{}
}

func (g *sequentialIDGenerator) NewBatchID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seq++
	return fmt.Sprintf("batch_%d", g.seq)
}

func (g *sequentialIDGenerator) NewSessionID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seq++
	return fmt.Sprintf("sess_%d", g.seq)
}

var _ application.IDGenerator = (*sequentialIDGenerator)(nil)
