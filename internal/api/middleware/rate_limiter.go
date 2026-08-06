package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/NoxiouSi/eino-risk-qa/internal/api/dto"
)

// RateLimiter 基于令牌桶算法的简易限流器，按客户端 IP 维度限流。
type RateLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*tokenBucket
	rate        float64 // 每秒补充令牌数
	burst       int     // 桶容量（允许的突发请求数）
	cleanupTick time.Duration
}

type tokenBucket struct {
	tokens    float64
	lastCheck time.Time
}

// NewRateLimiter 创建限流器。rate 为每秒补充令牌数，burst 为桶容量。
// 例如 NewRateLimiter(10, 20) 表示每秒可处理 10 个请求，突发最多 20 个。
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	if rate <= 0 {
		rate = 10
	}
	if burst <= 0 {
		burst = 20
	}
	rl := &RateLimiter{
		buckets:     make(map[string]*tokenBucket),
		rate:        rate,
		burst:       burst,
		cleanupTick: 5 * time.Minute,
	}
	// 后台定期清理过期桶
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanupTick)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		threshold := rl.cleanupTick * 2
		for key, bucket := range rl.buckets {
			if time.Since(bucket.lastCheck) > threshold {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow 检查并消耗一个令牌，返回是否允许通过。
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, ok := rl.buckets[key]
	now := time.Now()
	if !ok {
		bucket = &tokenBucket{tokens: float64(rl.burst), lastCheck: now}
		rl.buckets[key] = bucket
	}

	elapsed := now.Sub(bucket.lastCheck).Seconds()
	bucket.tokens += elapsed * rl.rate
	if bucket.tokens > float64(rl.burst) {
		bucket.tokens = float64(rl.burst)
	}
	bucket.lastCheck = now

	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

// Handler 返回 Hertz 兼容的限流中间件。
// keyFunc 用于从请求中提取限流维度，如 IP、用户 ID 等。
func (rl *RateLimiter) Handler(keyFunc func(c *app.RequestContext) string) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		key := keyFunc(c)
		if !rl.Allow(key) {
			c.Header("Retry-After", "1")
			c.JSON(http.StatusTooManyRequests, dto.ErrorResponse{
				ErrorCode: "RATE_LIMITED",
				Message:   "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}

// IPKeyFunc 从请求中提取客户端 IP 作为限流 key。
func IPKeyFunc(c *app.RequestContext) string {
	// 优先检查 X-Forwarded-For（反向代理场景）
	if xff := string(c.GetHeader("X-Forwarded-For")); xff != "" {
		return xff
	}
	if xri := string(c.GetHeader("X-Real-IP")); xri != "" {
		return xri
	}
	return c.ClientIP()
}
