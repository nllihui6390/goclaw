package bootstrap

import (
	"path/filepath"

	"go-claw/config"
	"go-claw/internal/channel"
	"go-claw/pkg/utils"
)

// toDisplayConfig 从渠道配置提取显示控制配置
func toDisplayConfig(showToolMessages, showThinking, streamOutput bool) channel.DisplayConfig {
	return channel.DisplayConfig{
		ShowToolMessages: showToolMessages,
		ShowThinking:     showThinking,
		StreamOutput:     streamOutput,
	}
}

// registerConsoleChannel 创建并注册 Console 渠道（HTTP 聊天与 Gateway 共用同一实例）
func (app *App) registerConsoleChannel(workspaceDir, agentName string) {
	consoleConfig := app.loadConsoleConfigForAgent(workspaceDir, agentName)
	display := toDisplayConfig(consoleConfig.ShowToolMessages, consoleConfig.ShowThinking, consoleConfig.StreamOutput)
	consoleChan := channel.NewConsoleChannel(consoleConfig.BotPrefix, display)
	consoleChan.SetEnabled(true)
	app.Gateway.RegisterChannelWithoutServer(consoleChan)
}

// consoleConfigSourceAgent 选择 console 显示配置来源（优先 default agent，否则第一个启用的）
func (app *App) consoleConfigSourceAgent(workspaceDir string, cfg *config.Config) string {
	defaultAgent := cfg.GetDefaultAgent()
	if config.IsAgentChannelEnabled(workspaceDir, defaultAgent, "console") {
		return defaultAgent
	}
	agentNames, _ := config.ListAgentConfigs(workspaceDir)
	for _, name := range agentNames {
		if profile, ok := cfg.Agents.Profiles[name]; ok && !profile.Enabled {
			continue
		}
		if config.IsAgentChannelEnabled(workspaceDir, name, "console") {
			return name
		}
	}
	return defaultAgent
}

// syncGlobalConsoleChannel 同步全局 Console 渠道（任意 agent 启用则保持注册，全部禁用才注销）
func (app *App) syncGlobalConsoleChannel(workspaceDir string, cfg *config.Config) {
	if config.AnyAgentChannelEnabled(workspaceDir, cfg, "console") {
		if !app.Gateway.HasChannel("console") {
			agentName := app.consoleConfigSourceAgent(workspaceDir, cfg)
			app.registerConsoleChannel(workspaceDir, agentName)
			app.logger.Info("Console 渠道已注册", "config_from", agentName)
		}
	} else if app.Gateway.HasChannel("console") {
		app.Gateway.UnregisterChannel("console")
		app.logger.Info("Console 渠道已注销（无 agent 启用）")
	}
}

// initChannels 注册所有渠道
// Console：共享 HTTP 基础设施，按 agent 独立配置 enabled，全局实例任意 agent 启用即注册
// Bot 渠道：per-agent，每个 agent 可有自己的 lark/dingtalk/wecom/wechat
func (app *App) initChannels() {
	workspaceDir := filepath.Join(app.DataDir, app.Workspace)
	defaultAgent := app.Config.GetDefaultAgent()

	// 1. Console 渠道（全局共享）
	app.syncGlobalConsoleChannel(workspaceDir, app.Config)

	// 2. Bot 渠道（per-agent）——扫描 workspace 目录发现所有 agent
	app.initBotChannels(workspaceDir, app.Config)

	app.Gateway.SetDefaultAgent(defaultAgent)
}

// loadConsoleConfigForAgent 加载指定 agent 的 console 配置
func (app *App) loadConsoleConfigForAgent(workspaceDir, agentName string) config.ConsoleConfig {
	agentCfg, err := config.LoadAgentConfig(workspaceDir, agentName)
	if err != nil {
		app.logger.Warn("加载 agent console 配置失败，使用默认", "agent", agentName, "err", err)
		return config.ConsoleConfig{Enabled: true, ShowToolMessages: true, ShowThinking: true, StreamOutput: true}
	}
	return agentCfg.Channels.Console
}

// initBotChannels 为每个 agent 注册其 bot 渠道
// 扫描 workspace 目录发现所有 agent.json，同时检查 profiles 中的 enabled 状态
func (app *App) initBotChannels(workspaceDir string, cfg *config.Config) {
	agentNames, err := config.ListAgentConfigs(workspaceDir)
	if err != nil {
		app.logger.Warn("扫描 agent 目录失败", "err", err)
		return
	}

	for _, agentName := range agentNames {
		// 检查 profiles 中的 enabled 状态
		if profile, ok := cfg.Agents.Profiles[agentName]; ok {
			if !profile.Enabled {
				app.logger.Info("Agent 已禁用，跳过渠道注册", "agent", agentName)
				continue
			}
		}
		// 如果 profiles 中没有该 agent，默认视为启用（首次发现）

		agentCfg, err := config.LoadAgentConfig(workspaceDir, agentName)
		if err != nil {
			app.logger.Warn("加载 agent 配置失败，跳过渠道注册", "agent", agentName, "err", err)
			continue
		}

		app.registerBotChannelsForAgent(agentName, agentCfg.Channels)
	}
}

// registerBotChannelsForAgent 为指定 agent 注册其 bot 渠道
func (app *App) registerBotChannelsForAgent(agentName string, channels config.ChannelsConfig) {
	// 飞书
	if channels.Lark.Enabled {
		display := toDisplayConfig(channels.Lark.ShowToolMessages, channels.Lark.ShowThinking, channels.Lark.StreamOutput)
		larkChan := channel.NewLarkChannel(channels.Lark.AppID, channels.Lark.AppSecret, channels.Lark.BotPrefix, display)
		if err := app.Gateway.RegisterChannelForAgent(agentName, larkChan); err != nil {
			app.logger.Error("注册飞书渠道失败", "agent", agentName, "err", err)
		} else {
			app.logger.Info("飞书渠道已注册", "agent", agentName)
		}
	}

	// 钉钉
	if channels.DingTalk.Enabled {
		display := toDisplayConfig(channels.DingTalk.ShowToolMessages, channels.DingTalk.ShowThinking, channels.DingTalk.StreamOutput)
		dingtalkChan := channel.NewDingTalkChannel(channels.DingTalk.ClientID, channels.DingTalk.ClientSecret, channels.DingTalk.BotPrefix, display)
		if err := app.Gateway.RegisterChannelForAgent(agentName, dingtalkChan); err != nil {
			app.logger.Error("注册钉钉渠道失败", "agent", agentName, "err", err)
		} else {
			app.logger.Info("钉钉渠道已注册", "agent", agentName)
		}
	}

	// 企业微信
	if channels.WeCom.Enabled {
		display := toDisplayConfig(channels.WeCom.ShowToolMessages, channels.WeCom.ShowThinking, channels.WeCom.StreamOutput)
		wecomChan := channel.NewWeComChannel(channels.WeCom.BotID, channels.WeCom.Secret, channels.WeCom.BotPrefix, display)
		if err := app.Gateway.RegisterChannelForAgent(agentName, wecomChan); err != nil {
			app.logger.Error("注册企业微信渠道失败", "agent", agentName, "err", err)
		} else {
			app.logger.Info("企业微信渠道已注册", "agent", agentName)
		}
	}

	// 微信个人
	if channels.WeChat.Enabled {
		display := toDisplayConfig(channels.WeChat.ShowToolMessages, channels.WeChat.ShowThinking, channels.WeChat.StreamOutput)
		wechatChan := channel.NewWeChatChannel(
			channels.WeChat.BotToken,
			channels.WeChat.BotPrefix,
			channels.WeChat.BaseURL,
			channels.WeChat.MediaDir,
			channels.WeChat.BotTokenFile,
			display,
		)
		if err := app.Gateway.RegisterChannelForAgent(agentName, wechatChan); err != nil {
			app.logger.Error("注册微信渠道失败", "agent", agentName, "err", err)
		} else {
			app.logger.Info("微信渠道已注册", "agent", agentName)
		}
	}
}

// SyncChannels 根据新配置同步渠道（热加载时调用）
func (app *App) SyncChannels(newCfg *config.Config) {
	workspaceDir := filepath.Join(app.DataDir, newCfg.Gateway.Workspace)

	// 1. Console 渠道（全局共享，按全量 agent 配置决定注册/注销）
	app.syncGlobalConsoleChannel(workspaceDir, newCfg)

	// 2. Bot 渠道（per-agent）
	// 先注销所有旧的 per-agent 渠道
	app.unregisterAllBotChannels()

	// 再注册所有新的 per-agent 渠道（使用 newCfg 判断 agent 启用状态）
	app.initBotChannels(workspaceDir, newCfg)

	// 更新当前配置引用
	app.Config = newCfg
}

// unregisterAllBotChannels 注销所有 per-agent bot 渠道
func (app *App) unregisterAllBotChannels() {
	registeredChannels := app.Gateway.GetRegisteredChannels()
	for _, key := range registeredChannels {
		// 只处理 per-agent 渠道（格式为 agent:channel）
		if utils.ContainsColon(key) {
			app.Gateway.UnregisterChannel(key)
			app.logger.Info("Bot 渠道已注销", "key", key)
		}
	}
}

// SyncSingleChannel 精准同步单个渠道（注销旧的 + 注册新的）
// 仅针对变更的渠道操作，不影响其他渠道
func (app *App) SyncSingleChannel(newCfg *config.Config, agentName, channelName string) {
	workspaceDir := filepath.Join(app.DataDir, newCfg.Gateway.Workspace)
	channelKey := agentName + ":" + channelName

	// Console 渠道：per-agent 配置，全局单实例；仅当所有 agent 均禁用时才注销
	if channelName == "console" {
		app.syncGlobalConsoleChannel(workspaceDir, newCfg)
		app.Config = newCfg
		return
	}

	// Bot 渠道（per-agent）精准同步
	// Agent 整体禁用时只注销，不重新注册
	if profile, ok := newCfg.Agents.Profiles[agentName]; ok && !profile.Enabled {
		if app.Gateway.HasChannelForAgent(agentName, channelName) {
			app.Gateway.UnregisterChannelForAgent(agentName, channelName)
			app.logger.Info("Agent 已禁用，Bot 渠道已注销", "key", channelKey)
		}
		app.Config = newCfg
		return
	}

	// 1. 先注销该渠道（如果已注册）
	if app.Gateway.HasChannelForAgent(agentName, channelName) {
		app.Gateway.UnregisterChannelForAgent(agentName, channelName)
		app.logger.Info("Bot 渠道已注销", "key", channelKey)
	}

	// 2. 加载最新 agent 配置
	agentCfg, err := config.LoadAgentConfig(workspaceDir, agentName)
	if err != nil {
		app.logger.Warn("加载 agent 配置失败，无法注册渠道", "agent", agentName, "err", err)
		app.Config = newCfg
		return
	}

	// 3. 如果启用，重新注册
	app.registerSingleBotChannel(agentName, channelName, agentCfg.Channels)

	app.Config = newCfg
}

// registerSingleBotChannel 注册单个 bot 渠道
func (app *App) registerSingleBotChannel(agentName, channelName string, channels config.ChannelsConfig) {
	switch channelName {
	case "lark":
		if channels.Lark.Enabled {
			display := toDisplayConfig(channels.Lark.ShowToolMessages, channels.Lark.ShowThinking, channels.Lark.StreamOutput)
			larkChan := channel.NewLarkChannel(channels.Lark.AppID, channels.Lark.AppSecret, channels.Lark.BotPrefix, display)
			if err := app.Gateway.RegisterChannelForAgent(agentName, larkChan); err != nil {
				app.logger.Error("注册飞书渠道失败", "agent", agentName, "err", err)
			} else {
				app.logger.Info("飞书渠道已注册", "agent", agentName)
			}
		}
	case "dingtalk":
		if channels.DingTalk.Enabled {
			display := toDisplayConfig(channels.DingTalk.ShowToolMessages, channels.DingTalk.ShowThinking, channels.DingTalk.StreamOutput)
			dingtalkChan := channel.NewDingTalkChannel(channels.DingTalk.ClientID, channels.DingTalk.ClientSecret, channels.DingTalk.BotPrefix, display)
			if err := app.Gateway.RegisterChannelForAgent(agentName, dingtalkChan); err != nil {
				app.logger.Error("注册钉钉渠道失败", "agent", agentName, "err", err)
			} else {
				app.logger.Info("钉钉渠道已注册", "agent", agentName)
			}
		}
	case "wecom":
		if channels.WeCom.Enabled {
			display := toDisplayConfig(channels.WeCom.ShowToolMessages, channels.WeCom.ShowThinking, channels.WeCom.StreamOutput)
			wecomChan := channel.NewWeComChannel(channels.WeCom.BotID, channels.WeCom.Secret, channels.WeCom.BotPrefix, display)
			if err := app.Gateway.RegisterChannelForAgent(agentName, wecomChan); err != nil {
				app.logger.Error("注册企业微信渠道失败", "agent", agentName, "err", err)
			} else {
				app.logger.Info("企业微信渠道已注册", "agent", agentName)
			}
		}
	case "wechat":
		if channels.WeChat.Enabled {
			display := toDisplayConfig(channels.WeChat.ShowToolMessages, channels.WeChat.ShowThinking, channels.WeChat.StreamOutput)
			wechatChan := channel.NewWeChatChannel(
				channels.WeChat.BotToken,
				channels.WeChat.BotPrefix,
				channels.WeChat.BaseURL,
				channels.WeChat.MediaDir,
				channels.WeChat.BotTokenFile,
				display,
			)
			if err := app.Gateway.RegisterChannelForAgent(agentName, wechatChan); err != nil {
				app.logger.Error("注册微信渠道失败", "agent", agentName, "err", err)
			} else {
				app.logger.Info("微信渠道已注册", "agent", agentName)
			}
		}
	default:
		app.logger.Warn("未知的渠道类型", "channel", channelName)
	}
}
