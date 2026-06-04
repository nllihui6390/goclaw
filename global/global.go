package global

import (
	"errors"
	"go-claw/config"
	"go-claw/internal/bootstrap"
	"go-claw/internal/gateway"
	"go-claw/internal/store"
	"sync"
	"time"
)

var (
	mu sync.RWMutex // 保护并发读写

	// 核心共享实例
	G_APP         *bootstrap.App       // App 实例（用于重启等操作）
	G_GATEWAY     *gateway.Gateway     // Gateway（含 agents, channels, router）
	G_CONFIG      *config.Config       // 配置结构（解析后的静态配置）
	G_SESSION_IDX *store.SessionIndex  // 会话索引
	G_START_TIME  time.Time            // 启动时间
)

// SetApp 设置 App 实例
func SetApp(app *bootstrap.App) {
	mu.Lock()
	defer mu.Unlock()
	G_APP = app
}

// GetApp 获取 App 实例
func GetApp() *bootstrap.App {
	mu.RLock()
	defer mu.RUnlock()
	return G_APP
}

// SetStartTime 设置启动时间
func SetStartTime(t time.Time) {
	mu.Lock()
	defer mu.Unlock()
	G_START_TIME = t
}

// GetStartTime 获取启动时间
func GetStartTime() time.Time {
	mu.RLock()
	defer mu.RUnlock()
	return G_START_TIME
}

// Restart 重启系统（重新加载配置并同步 Agent/Channel）
func Restart() error {
	mu.RLock()
	app := G_APP
	mu.RUnlock()

	if app == nil {
		return errors.New("app not initialized")
	}

	err := app.Restart()
	if err != nil {
		return err
	}

	// 更新启动时间
	mu.Lock()
	G_START_TIME = time.Now()
	mu.Unlock()

	return nil
}

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