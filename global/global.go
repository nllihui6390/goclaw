package global

import (
	"go-claw/config"
	"go-claw/internal/gateway"
	"go-claw/internal/store"
	"sync"
)

var (
	mu sync.RWMutex // 保护并发读写

	// 核心共享实例
	G_GATEWAY     *gateway.Gateway     // Gateway（含 agents, channels, router）
	G_CONFIG      *config.Config       // 配置结构（解析后的静态配置）
	G_SESSION_IDX *store.SessionIndex  // 会话索引
)

// SetGateway 设置 Gateway 实例
func SetGateway(gw *gateway.Gateway) {
	mu.Lock()
	defer mu.Unlock()
	G_GATEWAY = gw
}

// GetGateway 获取 Gateway 实例
func GetGateway() *gateway.Gateway {
	mu.RLock()
	defer mu.RUnlock()
	return G_GATEWAY
}

// SetConfig 设置配置
func SetConfig(cfg *config.Config) {
	mu.Lock()
	defer mu.Unlock()
	G_CONFIG = cfg
}

// GetConfig 获取配置
func GetConfig() *config.Config {
	mu.RLock()
	defer mu.RUnlock()
	return G_CONFIG
}

// SetSessionIndex 设置会话索引
func SetSessionIndex(idx *store.SessionIndex) {
	mu.Lock()
	defer mu.Unlock()
	G_SESSION_IDX = idx
}

// GetSessionIndex 获取会话索引
func GetSessionIndex() *store.SessionIndex {
	mu.RLock()
	defer mu.RUnlock()
	return G_SESSION_IDX
}