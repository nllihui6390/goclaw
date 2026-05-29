package bootstrap

import (
	"go-claw/internal/channel"
)

// initChannels 注册所有渠道
func (app *App) initChannels() {
	if app.Config.Channels.Console.Enabled {
		consoleChan := channel.NewConsoleChannel()
		if err := app.Gateway.RegisterChannel(consoleChan); err != nil {
			app.logger.Error("注册控制台渠道失败", "err", err)
		}
	}

	if app.Config.Channels.Webhook.Enabled {
		webhookChan := channel.NewWebhookChannel(app.Config.Channels.Webhook.Port, app.Config.Auth.Token)
		if err := app.Gateway.RegisterChannel(webhookChan); err != nil {
			app.logger.Error("注册Webhook渠道失败", "err", err)
		}
	}

	if app.Config.Channels.WebSocket.Enabled {
		wsChan := channel.NewWebSocketChannel(app.Config.Channels.WebSocket.Port)
		if err := app.Gateway.RegisterChannel(wsChan); err != nil {
			app.logger.Error("注册WebSocket渠道失败", "err", err)
		}
	}

	if app.Config.Channels.Lark.Enabled {
		larkChan := channel.NewLarkChannel(
			app.Config.Channels.Lark.AppID,
			app.Config.Channels.Lark.AppSecret,
		)
		if err := app.Gateway.RegisterChannel(larkChan); err != nil {
			app.logger.Error("注册飞书渠道失败", "err", err)
		}
	}

	if app.Config.Channels.DingTalk.Enabled {
		dingtalkChan := channel.NewDingTalkChannel(
			app.Config.Channels.DingTalk.ClientID,
			app.Config.Channels.DingTalk.ClientSecret,
		)
		if err := app.Gateway.RegisterChannel(dingtalkChan); err != nil {
			app.logger.Error("注册钉钉渠道失败", "err", err)
		}
	}

	if app.Config.Channels.WeCom.Enabled {
		wecomChan := channel.NewWeComChannel(
			app.Config.Channels.WeCom.BotID,
			app.Config.Channels.WeCom.Secret,
		)
		if err := app.Gateway.RegisterChannel(wecomChan); err != nil {
			app.logger.Error("注册企业微信渠道失败", "err", err)
		}
	}

	app.Gateway.SetDefaultAgent("default")
}