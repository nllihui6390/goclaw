package bootstrap

import (
	"path/filepath"

	"go-claw/config"
	"go-claw/internal/channel"
)

// toDisplayConfig 从渠道配置提取显示控制配置
func toDisplayConfig(showToolMessages, showThinking, streamOutput bool) channel.DisplayConfig {
	return channel.DisplayConfig{
		ShowToolMessages: showToolMessages,
		ShowThinking:    showThinking,
		StreamOutput:    streamOutput,
	}
}

// initChannels 注册所有渠道
// Console：共享基础设施，使用 default agent 的配置
// Bot 渠道：per-agent，每个 agent 可有自己的 lark/dingtalk/wecom/wechat
func (app *App) initChannels() {
	workspaceDir := filepath.Join(app.DataDir, app.Workspace)
	defaultAgent := app.Config.GetDefaultAgent()

	// 1. Console 渠道（共享，使用 default agent 的 console 配置）
	consoleConfig := app.loadConsoleConfigForAgent(workspaceDir, defaultAgent)
	if consoleConfig.Enabled {
		display := toDisplayConfig(consoleConfig.ShowToolMessages, consoleConfig.ShowThinking, consoleConfig.StreamOutput)
		consoleChan := channel.NewConsoleChannel("desktop", "", display)
		app.Gateway.RegisterChannelWithoutServer(consoleChan)
		app.logger.Info("Console 渠道已注册", "agent", defaultAgent)
	}

	// 2. Bot 渠道（per-agent）——扫描 workspace 目录发现所有 agent
	app.initBotChannels(workspaceDir)

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
func (app *App) initBotChannels(workspaceDir string) {
	agentNames, err := config.ListAgentConfigs(workspaceDir)
	if err != nil {
		app.logger.Warn("扫描 agent 目录失败", "err", err)
		return
	}

	for _, agentName := range agentNames {
		// 检查 profiles 中的 enabled 状态
		if profile, ok := app.Config.Agents.Profiles[agentName]; ok {
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
		larkChan := channel.NewLarkChannel(channels.Lark.AppID, channels.Lark.AppSecret, display)
		if err := app.Gateway.RegisterChannelForAgent(agentName, larkChan); err != nil {
			app.logger.Error("注册飞书渠道失败", "agent", agentName, "err", err)
		} else {
			app.logger.Info("飞书渠道已注册", "agent", agentName)
		}
	}

	// 钉钉
	if channels.DingTalk.Enabled {
		display := toDisplayConfig(channels.DingTalk.ShowToolMessages, channels.DingTalk.ShowThinking, channels.DingTalk.StreamOutput)
		dingtalkChan := channel.NewDingTalkChannel(channels.DingTalk.ClientID, channels.DingTalk.ClientSecret, display)
		if err := app.Gateway.RegisterChannelForAgent(agentName, dingtalkChan); err != nil {
			app.logger.Error("注册钉钉渠道失败", "agent", agentName, "err", err)
		} else {
			app.logger.Info("钉钉渠道已注册", "agent", agentName)
		}
	}

	// 企业微信
	if channels.WeCom.Enabled {
		display := toDisplayConfig(channels.WeCom.ShowToolMessages, channels.WeCom.ShowThinking, channels.WeCom.StreamOutput)
		wecomChan := channel.NewWeComChannel(channels.WeCom.BotID, channels.WeCom.Secret, display)
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
	workspaceDir := filepath.Join(newCfg.Gateway.DataDir, newCfg.Gateway.Workspace)
	defaultAgent := newCfg.GetDefaultAgent()

	// 1. Console 渠道（共享）
	newConsoleConfig := app.loadConsoleConfigForAgent(workspaceDir, defaultAgent)
	if newConsoleConfig.Enabled && !app.Gateway.HasChannel("console") {
		display := toDisplayConfig(newConsoleConfig.ShowToolMessages, newConsoleConfig.ShowThinking, newConsoleConfig.StreamOutput)
		consoleChan := channel.NewConsoleChannel("desktop", "", display)
		app.Gateway.RegisterChannelWithoutServer(consoleChan)
		app.logger.Info("Console 渠道已热加载注册", "agent", defaultAgent)
	} else if !newConsoleConfig.Enabled && app.Gateway.HasChannel("console") {
		app.Gateway.UnregisterChannel("console")
		app.logger.Info("Console 渠道已热加载注销")
	}

	// 2. Bot 渠道（per-agent）
	// 先注销所有旧的 per-agent 渠道
	app.unregisterAllBotChannels()

	// 再注册所有新的 per-agent 渠道
	app.initBotChannels(workspaceDir)

	// 更新当前配置引用
	app.Config = newCfg
}

// unregisterAllBotChannels 注销所有 per-agent bot 渠道
func (app *App) unregisterAllBotChannels() {
	registeredChannels := app.Gateway.GetRegisteredChannels()
	for _, key := range registeredChannels {
		// 只处理 per-agent 渠道（格式为 agent:channel）
		if containsColon(key) {
			app.Gateway.UnregisterChannel(key)
			app.logger.Info("Bot 渠道已注销", "key", key)
		}
	}
}

// containsColon 检查字符串是否包含冒号（用于区分 per-agent 渠道）
func containsColon(s string) bool {
	for _, c := range s {
		if c == ':' {
			return true
		}
	}
	return false
}

// SyncSingleChannel 精准同步单个渠道（注销旧的 + 注册新的）
// 仅针对变更的渠道操作，不影响其他渠道
func (app *App) SyncSingleChannel(newCfg *config.Config, agentName, channelName string) {
	workspaceDir := filepath.Join(newCfg.Gateway.DataDir, newCfg.Gateway.Workspace)
	channelKey := agentName + ":" + channelName

	// Console 渠道特殊处理（全局共享）
	if channelName == "console" {
		consoleConfig := app.loadConsoleConfigForAgent(workspaceDir, agentName)
		if consoleConfig.Enabled && !app.Gateway.HasChannel("console") {
			// 启用且未注册 → 注册
			display := toDisplayConfig(consoleConfig.ShowToolMessages, consoleConfig.ShowThinking, consoleConfig.StreamOutput)
			consoleChan := channel.NewConsoleChannel("desktop", "", display)
			app.Gateway.RegisterChannelWithoutServer(consoleChan)
			app.logger.Info("Console 渠道已注册", "agent", agentName)
		} else if !consoleConfig.Enabled && app.Gateway.HasChannel("console") {
			// 禁用且已注册 → 注销
			app.Gateway.UnregisterChannel("console")
			app.logger.Info("Console 渠道已注销")
		}
		app.Config = newCfg
		return
	}

	// Bot 渠道（per-agent）精准同步
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
			larkChan := channel.NewLarkChannel(channels.Lark.AppID, channels.Lark.AppSecret, display)
			if err := app.Gateway.RegisterChannelForAgent(agentName, larkChan); err != nil {
				app.logger.Error("注册飞书渠道失败", "agent", agentName, "err", err)
			} else {
				app.logger.Info("飞书渠道已注册", "agent", agentName)
			}
		}
	case "dingtalk":
		if channels.DingTalk.Enabled {
			display := toDisplayConfig(channels.DingTalk.ShowToolMessages, channels.DingTalk.ShowThinking, channels.DingTalk.StreamOutput)
			dingtalkChan := channel.NewDingTalkChannel(channels.DingTalk.ClientID, channels.DingTalk.ClientSecret, display)
			if err := app.Gateway.RegisterChannelForAgent(agentName, dingtalkChan); err != nil {
				app.logger.Error("注册钉钉渠道失败", "agent", agentName, "err", err)
			} else {
				app.logger.Info("钉钉渠道已注册", "agent", agentName)
			}
		}
	case "wecom":
		if channels.WeCom.Enabled {
			display := toDisplayConfig(channels.WeCom.ShowToolMessages, channels.WeCom.ShowThinking, channels.WeCom.StreamOutput)
			wecomChan := channel.NewWeComChannel(channels.WeCom.BotID, channels.WeCom.Secret, display)
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