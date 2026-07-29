package application

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

// IDGenerator 生成批次/会话的业务唯一标识（字符串），由 api/main 层注入具体实现
// （如 UUID 或自定义前缀+时间戳），application 层不关心具体生成算法。
type IDGenerator interface {
	NewBatchID() string
	NewSessionID() string
}

// BatchAppService 编排批量首轮提交用例：创建 batch 记录，errgroup 并发调度各风险要素调用
// SessionAppService.SubmitInitial；单个风险要素失败不影响其他要素（各自独立返回 SessionResult）。
type BatchAppService struct {
	sessionSvc *SessionAppService
	repo       UserBatchRepository
	ids        IDGenerator
}

// NewBatchAppService 创建应用服务实例。
func NewBatchAppService(sessionSvc *SessionAppService, repo UserBatchRepository, ids IDGenerator) *BatchAppService {
	return &BatchAppService{sessionSvc: sessionSvc, repo: repo, ids: ids}
}

// SubmitBatch 批量首轮提交：先确保用户存在、创建批次记录，再并发对每个风险要素调用
// SessionAppService.SubmitInitial；返回的 Results 与输入 RiskFactors 一一对应（顺序保持一致）。
func (b *BatchAppService) SubmitBatch(ctx context.Context, input SubmitBatchInput) (BatchResult, error) {
	log := logging.FromContext(ctx).With("user_id", input.UserID)
	if err := b.repo.EnsureUser(ctx, User{UserID: input.UserID, Name: input.UserName}); err != nil {
		log.Error("submit batch: ensure user failed", "error", err.Error())
		return BatchResult{}, err
	}

	batchID := b.ids.NewBatchID()
	createdAt := time.Now().UTC()
	if err := b.repo.CreateBatch(ctx, Batch{BatchID: batchID, UserID: input.UserID, CreatedAt: createdAt}); err != nil {
		log.Error("submit batch: create batch failed", "batch_id", batchID, "error", err.Error())
		return BatchResult{}, err
	}
	log = log.With("batch_id", batchID)
	log.Info("submit batch: batch created", "risk_factor_count", len(input.RiskFactors))

	results := make([]SessionResult, len(input.RiskFactors))
	g, gctx := errgroup.WithContext(ctx)
	for i, rf := range input.RiskFactors {
		sessionID := b.ids.NewSessionID()
		g.Go(func() error {
			results[i] = b.sessionSvc.SubmitInitial(gctx, sessionID, batchID, input.UserID, rf.RiskFactorType, rf.MainQuestion, rf.Answer)
			return nil // 单要素失败已封装进 SessionResult.Error，不通过 error 中断整批
		})
	}
	// errgroup 的 Wait 在此处理论上不会返回非 nil error（各 goroutine 均返回 nil），
	// 但仍需调用以等待全部完成。
	_ = g.Wait()
	log.Info("submit batch: all risk factors processed")

	return BatchResult{BatchID: batchID, CreatedAt: createdAt, Results: results}, nil
}

// SubmitBatchStream 批量首轮提交的流式变体：并发对每个风险要素调用 SessionAppService.SubmitInitialStream，
// 各要素产出的事件都通过同一个 emitter 转发（emitter 的实现需自行保证并发写入安全，如内部加锁），
// 从而实现"多个风险要素的事件交错到达同一条 HTTP 流、依靠 session_id 字段区分归属"的效果。
// 本方法会阻塞直到所有风险要素均已产出 Done/Error 事件。
func (b *BatchAppService) SubmitBatchStream(ctx context.Context, input SubmitBatchInput, emitter StreamEmitter) {
	log := logging.FromContext(ctx).With("user_id", input.UserID)
	if err := b.repo.EnsureUser(ctx, User{UserID: input.UserID, Name: input.UserName}); err != nil {
		log.Error("submit batch (stream): ensure user failed", "error", err.Error())
		return
	}
	batchID := b.ids.NewBatchID()
	if err := b.repo.CreateBatch(ctx, Batch{BatchID: batchID, UserID: input.UserID, CreatedAt: time.Now().UTC()}); err != nil {
		log.Error("submit batch (stream): create batch failed", "batch_id", batchID, "error", err.Error())
		return
	}
	log = log.With("batch_id", batchID)
	log.Info("submit batch (stream): batch created", "risk_factor_count", len(input.RiskFactors))
	emitter.Emit(StreamEvent{Type: StreamEventBatchCreated, BatchID: batchID})

	var wg sync.WaitGroup
	for _, rf := range input.RiskFactors {
		sessionID := b.ids.NewSessionID()
		rf := rf
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.sessionSvc.SubmitInitialStream(ctx, sessionID, batchID, input.UserID, rf.RiskFactorType, rf.MainQuestion, rf.Answer, emitter)
		}()
	}
	wg.Wait()
	log.Info("submit batch (stream): all risk factors processed")
}

// GetBatch 查询整批状态：返回该批次下全部会话的当前完整状态。
func (b *BatchAppService) GetBatch(ctx context.Context, batchID string) (BatchResult, error) {
	log := logging.FromContext(ctx).With("batch_id", batchID)
	batch, err := b.repo.FindBatch(ctx, batchID)
	if err != nil {
		log.Warn("get batch: find batch failed", "error", err.Error())
		return BatchResult{}, err
	}

	sessions, err := b.sessionRepository().FindByBatchID(ctx, batchID)
	if err != nil {
		log.Error("get batch: find sessions by batch_id failed", "error", err.Error())
		return BatchResult{}, err
	}
	log.Info("get batch: found sessions", "session_count", len(sessions))

	results := make([]SessionResult, 0, len(sessions))
	for _, s := range sessions {
		results = append(results, toSessionResultFromSession(s))
	}
	return BatchResult{BatchID: batchID, CreatedAt: batch.CreatedAt, Results: results}, nil
}

// sessionRepository 从内部持有的 SessionAppService 中取出其 repo，避免 BatchAppService
// 再重复持有一份 riskfactor.SessionRepository 依赖。
func (b *BatchAppService) sessionRepository() riskfactor.SessionRepository {
	return b.sessionSvc.repo
}

func toSessionResultFromSession(s *riskfactor.RiskFactorSession) SessionResult {
	return toSessionResult(s)
}
