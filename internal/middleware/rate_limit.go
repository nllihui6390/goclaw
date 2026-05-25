package middleware

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter 简单令牌桶限流器
type RateLimiter struct {
	mu         sync.Mutex
	tokens     map[string]*bucket
	maxTokens  int
	refillRate time.Duration
}

type bucket struct {
	tokens   int
	lastTime time.Time
}

// NewRateLimiter 创建限流器
func NewRateLimiter(maxTokens int, refillRate time.Duration) *RateLimiter {
	return &RateLimiter{
		tokens:     make(map[string]*bucket),
		maxTokens:  maxTokens,
		refillRate: refillRate,
	}
}

// Allow 检查是否允许请求
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, exists := rl.tokens[key]
	now := time.Now()

	if !exists {
		rl.tokens[key] = &bucket{tokens: rl.maxTokens - 1, lastTime: now}
		return true
	}

	// 补充令牌
	elapsed := now.Sub(b.lastTime)
	tokensToAdd := int(elapsed / rl.refillRate)
	if tokensToAdd > 0 {
		b.tokens += tokensToAdd
		if b.tokens > rl.maxTokens {
			b.tokens = rl.maxTokens
		}
		b.lastTime = now
	}

	if b.tokens > 0 {
		b.tokens--
		return true
	}
	return false
}

// RateLimitMiddleware 限流中间件
func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if !limiter.Allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
