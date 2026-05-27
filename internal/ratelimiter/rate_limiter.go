package ratelimiter

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RateLimiter 每模型独立限流器
type RateLimiter struct {
	qpmLimit    int            // 每分钟请求上限
	window      []time.Time    // 滑动窗口
	mu          sync.Mutex
	cooldown    time.Duration  // 429 冷却时间
	cooldownUntil time.Time    // 冷却截止时间
	concurrencySem chan struct{} // 并发信号量
	maxConcurrency int          // 最大并发数
}

// NewRateLimiter 创建限流器
func NewRateLimiter(qpmLimit, maxConcurrency int) *RateLimiter {
	if maxConcurrency <= 0 {
		maxConcurrency = 5
	}
	return &RateLimiter{
		qpmLimit:       qpmLimit,
		concurrencySem: make(chan struct{}, maxConcurrency),
		maxConcurrency: maxConcurrency,
	}
}

// Wait 等待限流许可
func (r *RateLimiter) Wait(ctx context.Context) error {
	// 检查冷却状态
	if r.isCoolingDown() {
		waitTime := time.Until(r.cooldownUntil)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
		}
	}

	// 获取并发许可
	select {
	case r.concurrencySem <- struct{}{}:
		defer func() { <-r.concurrencySem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	// QPM 滑动窗口检查
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	// 清理过期窗口
	r.window = r.filterExpired(now)

	if len(r.window) >= r.qpmLimit {
		// 需要等待最早的请求过期
		waitTime := time.Until(r.window[0].Add(time.Minute))
		// 添加少量抖动避免同步突发
		jitter := time.Duration(now.Nanosecond() % 500) * time.Millisecond
		waitTime += jitter

		r.mu.Unlock()
		select {
		case <-ctx.Done():
			r.mu.Lock()
			return ctx.Err()
		case <-time.After(waitTime):
			r.mu.Lock()
		}

		// 再次清理窗口
		r.window = r.filterExpired(time.Now())
	}

	// 记录本次请求
	r.window = append(r.window, now)

	return nil
}

// Mark429 标记收到 429 响应，进入冷却
func (r *RateLimiter) Mark429(retryAfter time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if retryAfter > 0 {
		r.cooldownUntil = time.Now().Add(retryAfter)
	} else {
		r.cooldownUntil = time.Now().Add(10 * time.Second) // 默认冷却 10 秒
	}
	r.cooldown = time.Until(r.cooldownUntil)
}

// isCoolingDown 检查是否处于冷却状态
func (r *RateLimiter) isCoolingDown() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return time.Now().Before(r.cooldownUntil)
}

// filterExpired 清理过期请求记录
func (r *RateLimiter) filterExpired(now time.Time) []time.Time {
	cutoff := now.Add(-time.Minute)
	var filtered []time.Time
	for _, t := range r.window {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// RateLimiterRegistry 限流器注册中心（每模型独立）
type RateLimiterRegistry struct {
	limiters map[string]*RateLimiter
	mu       sync.RWMutex
}

// NewRegistry 创建限流器注册中心
func NewRegistry() *RateLimiterRegistry {
	return &RateLimiterRegistry{
		limiters: make(map[string]*RateLimiter),
	}
}

// Register 注册模型限流器
func (reg *RateLimiterRegistry) Register(model string, qpmLimit, maxConcurrency int) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.limiters[model] = NewRateLimiter(qpmLimit, maxConcurrency)
}

// Get 获取模型限流器
func (reg *RateLimiterRegistry) Get(model string) *RateLimiter {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	limiter, exists := reg.limiters[model]
	if !exists {
		// 自动创建默认限流器（60 QPM, 5 并发）
		reg.mu.RUnlock()
		reg.Register(model, 60, 5)
		reg.mu.RLock()
		limiter = reg.limiters[model]
	}
	return limiter
}

// Wait 等待指定模型的限流许可
func (reg *RateLimiterRegistry) Wait(ctx context.Context, model string) error {
	limiter := reg.Get(model)
	return limiter.Wait(ctx)
}

// Mark429 标记指定模型收到 429 响应
func (reg *RateLimiterRegistry) Mark429(model string, retryAfter time.Duration) {
	limiter := reg.Get(model)
	limiter.Mark429(retryAfter)
}

// Status 获取所有限流器状态
func (reg *RateLimiterRegistry) Status() map[string]string {
	reg.mu.RLock()
	defer reg.mu.RUnlock()

	status := make(map[string]string)
	for model, limiter := range reg.limiters {
		limiter.mu.Lock()
		status[model] = fmt.Sprintf("qpm: %d/%d, cooling: %v, concurrency: %d/%d",
			len(limiter.window), limiter.qpmLimit,
			time.Now().Before(limiter.cooldownUntil),
			len(limiter.concurrencySem), limiter.maxConcurrency)
		limiter.mu.Unlock()
	}
	return status
}