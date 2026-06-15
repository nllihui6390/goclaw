package model

import (
	"context"
	"sync"
	"time"
)

// =============================================
// Rate Limiting（ LLMRateLimiter）
// =============================================

// RateLimitConfig 速率限制配置。
//
//	RateLimitConfig：
//
// - MaxConcurrent: 并发信号量，限制同时进行的请求数
// - MaxQPM: 每分钟最大请求数，使用滑动窗口实现
// - AcquireTimeout: 获取槽位的超时时间
type RateLimitConfig struct {
	MaxConcurrent  int // 最大并发数（默认 10）
	MaxQPM         int // 每分钟最大请求数（默认 60，0 表示不限制）
	AcquireTimeout int // 获取槽位超时秒数（默认 30）
}

// DefaultRateLimitConfig 返回默认速率限制配置。
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		MaxConcurrent:  10,
		MaxQPM:         60,
		AcquireTimeout: 30,
	}
}

// RateLimiter 每个 model key 独立的速率限制器。
//
//	LLMRateLimiter：
//
// 执行顺序：
//  1. 等待 429 冷却（如果活跃）
//  2. 等待 QPM 槽位（滑动窗口）
//  3. 获取信号量槽位（并发上限）
type RateLimiter struct {
	maxConcurrent  int
	maxQPM         int
	acquireTimeout time.Duration

	semaphore chan struct{} // 并发控制信号量
	qpmWindow []time.Time   // QPM 滑动窗口
	mu        sync.Mutex    // 保护 qpmWindow
}

// NewRateLimiter 创建速率限制器。
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	maxConcurrent := config.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 10
	}

	acquireTimeout := time.Duration(config.AcquireTimeout) * time.Second
	if acquireTimeout <= 0 {
		acquireTimeout = 30 * time.Second
	}

	return &RateLimiter{
		maxConcurrent:  maxConcurrent,
		maxQPM:         config.MaxQPM,
		acquireTimeout: acquireTimeout,
		semaphore:      make(chan struct{}, maxConcurrent),
		qpmWindow:      make([]time.Time, 0),
	}
}

// Acquire 获取一个请求槽位。
//
// 返回获取时间，用于 Release 时计算耗时。
// 如果超时则返回错误。
//
// 执行顺序：
//  1. 等待 QPM 槽位（滑动窗口）
//  2. 获取信号量槽位（并发上限）
func (r *RateLimiter) Acquire(ctx context.Context) (time.Time, error) {
	acquiredAt := time.Now()

	// Step 1: QPM 滑动窗口
	if r.maxQPM > 0 {
		if err := r.acquireQPM(ctx); err != nil {
			return acquiredAt, err
		}
	}

	// Step 2: 并发信号量
	if err := r.acquireSemaphore(ctx); err != nil {
		return acquiredAt, err
	}

	return acquiredAt, nil
}

// Release 释放请求槽位。
func (r *RateLimiter) Release() {
	select {
	case <-r.semaphore:
		// 成功释放
	default:
		// 槽位已空，忽略
	}
}

// acquireQPM 等待 QPM 槽位。
func (r *RateLimiter) acquireQPM(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-time.Minute)

	// 清理过期的时间戳
	validIdx := 0
	for i, t := range r.qpmWindow {
		if t.After(windowStart) {
			validIdx = i
			break
		}
	}
	if validIdx > 0 {
		r.qpmWindow = r.qpmWindow[validIdx:]
	}

	// 检查是否超过 QPM 限制
	if len(r.qpmWindow) >= r.maxQPM {
		// 计算需要等待的时间
		oldest := r.qpmWindow[0]
		waitDuration := oldest.Add(time.Minute).Sub(now)
		if waitDuration > 0 {
			select {
			case <-time.After(waitDuration):
				// 等待完成，清理过期时间戳
				r.qpmWindow = r.qpmWindow[1:]
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	// 记录本次请求时间
	r.qpmWindow = append(r.qpmWindow, now)
	return nil
}

// acquireSemaphore 等待信号量槽位。
func (r *RateLimiter) acquireSemaphore(ctx context.Context) error {
	select {
	case r.semaphore <- struct{}{}:
		return nil
	case <-time.After(r.acquireTimeout):
		return context.DeadlineExceeded
	case <-ctx.Done():
		return ctx.Err()
	}
}

// =============================================
// 全局速率限制器管理
// =============================================

var (
	globalLimiters   = make(map[string]*RateLimiter)
	globalLimitersMu sync.RWMutex
)

// GetRateLimiter 获取或创建指定 key 的速率限制器。
//
// 每个 "provider_id:model_name" 获得独立的限制器，
// 防止一个模型的 429 阻塞其他模型。
func GetRateLimiter(key string, config RateLimitConfig) *RateLimiter {
	globalLimitersMu.RLock()
	limiter, exists := globalLimiters[key]
	globalLimitersMu.RUnlock()

	if exists {
		return limiter
	}

	globalLimitersMu.Lock()
	defer globalLimitersMu.Unlock()

	// 双重检查
	if limiter, exists = globalLimiters[key]; exists {
		return limiter
	}

	limiter = NewRateLimiter(config)
	globalLimiters[key] = limiter
	return limiter
}

// SetRateLimiter 设置指定 key 的速率限制器。
func SetRateLimiter(key string, limiter *RateLimiter) {
	globalLimitersMu.Lock()
	defer globalLimitersMu.Unlock()
	globalLimiters[key] = limiter
}
