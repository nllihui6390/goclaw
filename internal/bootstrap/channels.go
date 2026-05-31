package bootstrap

import (
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
func (app *App) initChannels() {
	if app.Config.Channels.Console.Enabled {
		display := toDisplayConfig(
			app.Config.Channels.Console.ShowToolMessages,
			app.Config.Channels.Console.ShowThinking,
			app.Config.Channels.Console.StreamOutput,
		)
		consoleChan := channel.NewConsoleChannel(display)
		if err := app.Gateway.RegisterChannel(consoleChan); err != nil {
			app.logger.Error("注册控制台渠道失败", "err", err)
		}
	}

	if app.Config.Channels.Webhook.Enabled {
		display := toDisplayConfig(
			app.Config.Channels.Webhook.ShowToolMessages,
			app.Config.Channels.Webhook.ShowThinking,
			app.Config.Channels.Webhook.StreamOutput,
		)
		webhookChan := channel.NewWebhookChannel(app.Config.Channels.Webhook.Port, app.Config.Auth.Token, display)
		if err := app.Gateway.RegisterChannel(webhookChan); err != nil {
			app.logger.Error("注册Webhook渠道失败", "err", err)
		}
	}

	if app.Config.Channels.WebSocket.Enabled {
		display := toDisplayConfig(
			app.Config.Channels.WebSocket.ShowToolMessages,
			app.Config.Channels.WebSocket.ShowThinking,
			app.Config.Channels.WebSocket.StreamOutput,
		)
		wsChan := channel.NewWebSocketChannel(app.Config.Channels.WebSocket.Port, display)
		if err := app.Gateway.RegisterChannel(wsChan); err != nil {
			app.logger.Error("注册WebSocket渠道失败", "err", err)
		}
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