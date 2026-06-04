package bootstrap

import (
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

// initChannels 注册所有渠道（包括 console）
func (app *App) initChannels() {
	// console（桌面模式下 UI 就是控制台）
	if app.Config.Channels.Console.Enabled {
		consoleChan := channel.NewConsoleChannel("desktop", "", channel.DefaultDisplayConfig())
		app.Gateway.RegisterChannelWithoutServer(consoleChan)
	}

	if app.Config.Channels.Lark.Enabled {
		display := toDisplayConfig(
			app.Config.Channels.Lark.ShowToolMessages,
			app.Config.Channels.Lark.ShowThinking,
			app.Config.Channels.Lark.StreamOutput,
		)
		larkChan := channel.NewLarkChannel(
			app.Config.Channels.Lark.AppID,
			app.Config.Channels.Lark.AppSecret,
			display,
		)
		if err := app.Gateway.RegisterChannel(larkChan); err != nil {
			app.logger.Error("注册飞书渠道失败", "err", err)
		}
	}

	if app.Config.Channels.DingTalk.Enabled {
		display := toDisplayConfig(
			app.Config.Channels.DingTalk.ShowToolMessages,
			app.Config.Channels.DingTalk.ShowThinking,
			app.Config.Channels.DingTalk.StreamOutput,
		)
		dingtalkChan := channel.NewDingTalkChannel(
			app.Config.Channels.DingTalk.ClientID,
			app.Config.Channels.DingTalk.ClientSecret,
			display,
		)
		if err := app.Gateway.RegisterChannel(dingtalkChan); err != nil {
			app.logger.Error("注册钉钉渠道失败", "err", err)
		}
	}

	if app.Config.Channels.WeCom.Enabled {
		display := toDisplayConfig(
			app.Config.Channels.WeCom.ShowToolMessages,
			app.Config.Channels.WeCom.ShowThinking,
			app.Config.Channels.WeCom.StreamOutput,
		)
		wecomChan := channel.NewWeComChannel(
			app.Config.Channels.WeCom.BotID,
			app.Config.Channels.WeCom.Secret,
			display,
		)
		if err := app.Gateway.RegisterChannel(wecomChan); err != nil {
			app.logger.Error("注册企业微信渠道失败", "err", err)
		}
	}


	if app.Config.Channels.WeChat.Enabled {
		display := toDisplayConfig(
			app.Config.Channels.WeChat.ShowToolMessages,
			app.Config.Channels.WeChat.ShowThinking,
			app.Config.Channels.WeChat.StreamOutput,
		)
		wechatChan := channel.NewWeChatChannel(
			app.Config.Channels.WeChat.BotToken,
			app.Config.Channels.WeChat.BotPrefix,
			app.Config.Channels.WeChat.BaseURL,
			app.Config.Channels.WeChat.MediaDir,
			app.Config.Channels.WeChat.BotTokenFile,
			display,
		)
		if err := app.Gateway.RegisterChannel(wechatChan); err != nil {
			app.logger.Error("注册微信渠道失败", "err", err)
		}
	}

	app.Gateway.SetDefaultAgent("default")
}

// SyncChannels 根据新配置同步渠道（热加载时调用）
func (app *App) SyncChannels(newCfg *config.Config) {
	// console（桌面模式下 console 就是 UI，启用即视为已连接）
	if newCfg.Channels.Console.Enabled && !app.Gateway.HasChannel("console") {
		// 注册虚拟 console 渠道（仅用于状态显示）
		consoleChan := channel.NewConsoleChannel("desktop", "", channel.DefaultDisplayConfig())
		app.Gateway.RegisterChannelWithoutServer(consoleChan)
		app.logger.Info("控制台渠道已热加载注册")
	} else if !newCfg.Channels.Console.Enabled && app.Gateway.HasChannel("console") {
		app.Gateway.UnregisterChannel("console")
		app.logger.Info("控制台渠道已热加载注销")
	}

	// 飞书
	if newCfg.Channels.Lark.Enabled && !app.Gateway.HasChannel("lark") {
		display := toDisplayConfig(newCfg.Channels.Lark.ShowToolMessages, newCfg.Channels.Lark.ShowThinking, newCfg.Channels.Lark.StreamOutput)
		ch := channel.NewLarkChannel(newCfg.Channels.Lark.AppID, newCfg.Channels.Lark.AppSecret, display)
		if err := app.Gateway.RegisterChannel(ch); err != nil {
			app.logger.Error("注册飞书渠道失败", "err", err)
		} else {
			app.logger.Info("飞书渠道已热加载注册")
		}
	} else if !newCfg.Channels.Lark.Enabled && app.Gateway.HasChannel("lark") {
		app.Gateway.UnregisterChannel("lark")
		app.logger.Info("飞书渠道已热加载注销")
	}

	// 钉钉
	if newCfg.Channels.DingTalk.Enabled && !app.Gateway.HasChannel("dingtalk") {
		display := toDisplayConfig(newCfg.Channels.DingTalk.ShowToolMessages, newCfg.Channels.DingTalk.ShowThinking, newCfg.Channels.DingTalk.StreamOutput)
		ch := channel.NewDingTalkChannel(newCfg.Channels.DingTalk.ClientID, newCfg.Channels.DingTalk.ClientSecret, display)
		if err := app.Gateway.RegisterChannel(ch); err != nil {
			app.logger.Error("注册钉钉渠道失败", "err", err)
		} else {
			app.logger.Info("钉钉渠道已热加载注册")
		}
	} else if !newCfg.Channels.DingTalk.Enabled && app.Gateway.HasChannel("dingtalk") {
		app.Gateway.UnregisterChannel("dingtalk")
		app.logger.Info("钉钉渠道已热加载注销")
	}

	// 企业微信
	if newCfg.Channels.WeCom.Enabled && !app.Gateway.HasChannel("wecom") {
		display := toDisplayConfig(newCfg.Channels.WeCom.ShowToolMessages, newCfg.Channels.WeCom.ShowThinking, newCfg.Channels.WeCom.StreamOutput)
		ch := channel.NewWeComChannel(newCfg.Channels.WeCom.BotID, newCfg.Channels.WeCom.Secret, display)
		if err := app.Gateway.RegisterChannel(ch); err != nil {
			app.logger.Error("注册企业微信渠道失败", "err", err)
		} else {
			app.logger.Info("企业微信渠道已热加载注册")
		}
	} else if !newCfg.Channels.WeCom.Enabled && app.Gateway.HasChannel("wecom") {
		app.Gateway.UnregisterChannel("wecom")
		app.logger.Info("企业微信渠道已热加载注销")
	}

	// 微信
	if newCfg.Channels.WeChat.Enabled && !app.Gateway.HasChannel("wechat") {
		display := toDisplayConfig(newCfg.Channels.WeChat.ShowToolMessages, newCfg.Channels.WeChat.ShowThinking, newCfg.Channels.WeChat.StreamOutput)
		ch := channel.NewWeChatChannel(newCfg.Channels.WeChat.BotToken, newCfg.Channels.WeChat.BotPrefix, newCfg.Channels.WeChat.BaseURL, newCfg.Channels.WeChat.MediaDir, newCfg.Channels.WeChat.BotTokenFile, display)
		if err := app.Gateway.RegisterChannel(ch); err != nil {
			app.logger.Error("注册微信渠道失败", "err", err)
		} else {
			app.logger.Info("微信渠道已热加载注册")
		}
	} else if !newCfg.Channels.WeChat.Enabled && app.Gateway.HasChannel("wechat") {
		app.Gateway.UnregisterChannel("wechat")
		app.logger.Info("微信渠道已热加载注销")
	}

	// 更新当前配置引用
	app.Config = newCfg
}