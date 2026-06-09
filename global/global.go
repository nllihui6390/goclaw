package global

import (
	"errors"
	"go-claw/config"
	"go-claw/internal/bootstrap"
	"go-claw/internal/gateway"
	"go-claw/internal/store"
	glog "go-claw/pkg/log"
	"sync"
	"time"
)

var (
	mu sync.RWMutex // 保护并发读写

	// 核心共享实例
	G_APP         *bootstrap.App      // App 实例（用于重启等操作）
	G_GATEWAY     *gateway.Gateway    // Gateway（含 agents, channels, router）
	G_CONFIG      *config.Config      // 配置结构（解析后的静态配置）
	G_SESSION_IDX *store.SessionIndex // 会话索引
	G_START_TIME  time.Time           // 启动时间
)

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

// ReloadConfig 重新载入配置（从 config.json 读取并更新全局配置，不重启 App）
// 同时更新 G_CONFIG 和 app.Config，确保 Agent 的 ConfigProvider 能读到最新值
// 同步 Agent 配置（含工具、技能等），确保实时生效
// ReloadConfigOnly 加载配置并刷新，但不主动重启 Agent，仅更新全局配置和缓存
func ReloadConfigOnly() error {
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		glog.Logger().Error("ReloadConfigOnly 加载失败", "err", err)
		return err
	}
	// 更新全局配置
	SetConfig(cfg)
	// 清除 agent 配置缓存
	config.InvalidateAllAgentCache()
	mu.RLock()
	app := G_APP
	mu.RUnlock()
	if app != nil {
		app.Config = cfg
	}
	glog.Logger().Info("配置已重新加载（仅全局，无 Agent 重启）", "agents", len(cfg.Agents.Profiles))
	return nil
}

// ReloadConfigAndSyncAgents 加载配置并同步 Agent（刷新工具、技能等，适用热加载）
func ReloadConfigAndSyncAgents() error {
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		glog.Logger().Error("ReloadConfigAndSyncAgents 加载失败", "err", err)
		return err
	}
	SetConfig(cfg)
	config.InvalidateAllAgentCache()
	mu.RLock()
	app := G_APP
	mu.RUnlock()
	if app != nil {
		app.Config = cfg
		app.SyncAgents(cfg)
	}
	glog.Logger().Info("配置已重新加载并同步 Agent", "agents", len(cfg.Agents.Profiles))
	return nil
}

// 网关中删除指定agent-并且删除 agent 工作空间目录
// name=agent名称
func RemoveAgentAndConfig(name string) error {
	mu.RLock()
	app := G_APP
	mu.RUnlock()
	if app == nil {
		return errors.New("app not initialized")
	}
	return app.DeleteAgent(name)
}

// 热加载单个 Agent 配置（热加载）
func ReloadAgent(name string) error {
	mu.RLock()
	app := G_APP
	mu.RUnlock()
	if app == nil {
		return errors.New("app not initialized")
	}
	return app.ReloadAgent(name)
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

// 重新加载配置后精准同步指定 agent 的某个渠道（注销旧的 + 注册新的）
func ReloadConfigAndSyncSingleChannel(agentName, channelName string) {
	ReloadConfigOnly()
	mu.RLock()
	app := G_APP
	mu.RUnlock()
	if app != nil {
		app.SyncSingleChannel(GetConfig(), agentName, channelName)
	}
}
