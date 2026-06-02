//go:build !production

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"go-claw/internal/bootstrap"
	"go-claw/internal/channel"
	"go-claw/server"
	glog "go-claw/pkg/log"
)

// gatewayInstance 全局 Gateway 实例，供 HTTP API 访问
var gatewayInstance *bootstrap.App

// runServer 启动 go-claw 完整服务
// 初始化前端嵌入、go-claw 核心、Chat API、管理后台 HTTP 服务器
func runServer() {
	initFrontend()

	app, err := bootstrap.NewApp()
	if err != nil {
		glog.Logger().Error("初始化失败", "err", err)
		return
	}
	gatewayInstance = app

	webhookChan := channel.NewWebhookChannel("8080", "", channel.DefaultDisplayConfig())
	webServer := server.New(server.Config{Port: "8080"})
	webServer.Mux().HandleFunc("/api/v1/chat", webhookChan.HandleChat)
	webServer.Mux().HandleFunc("/api/v1/chat/history/", handleChatHistory)
	webServer.Start()
	app.Gateway.RegisterChannelWithoutServer(webhookChan)

	app.Run()
}

// handleChatHistory 获取会话历史记录
func handleChatHistory(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// 解析 sessionID: /api/v1/chat/history/{sessionID}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/chat/history/")
	sessionID := strings.TrimSuffix(path, "/")
	if sessionID == "" {
		json.NewEncoder(rw).Encode([]interface{}{})
		return
	}

	// 从 agent 的 SessionManager 或 Store 获取历史
	// 支持 ?agent=xxx 参数指定 agent（否则遍历所有 agent 查找）
	reqAgent := r.URL.Query().Get("agent")
	agents := gatewayInstance.Gateway.GetAgents()
	for _, ag := range agents {
		// 如果指定了 agent，只查询该 agent
		if reqAgent != "" && ag.Name() != reqAgent {
			continue
		}
		msgs, exists := ag.GetSessionMessages(sessionID)
		if exists {
			result := make([]map[string]string, 0, len(msgs))
			for _, m := range msgs {
				if m.Role == "user" || m.Role == "assistant" {
					result = append(result, map[string]string{
						"role":    m.Role,
						"content": m.Content,
					})
				}
			}
			rw.Header().Set("Content-Type", "application/json")
			json.NewEncoder(rw).Encode(result)
			return
		}

		// 尝试从 Store 加载
		st := ag.GetStore()
		if st != nil {
			sessData, err := st.GetSession(context.Background(), sessionID)
			if err == nil && sessData != nil {
				result := make([]map[string]string, 0, len(sessData.Messages))
				for _, m := range sessData.Messages {
					if m.Role == "user" || m.Role == "assistant" {
						result = append(result, map[string]string{
							"role":    m.Role,
							"content": m.Content,
						})
					}
				}
				rw.Header().Set("Content-Type", "application/json")
				json.NewEncoder(rw).Encode(result)
				return
			}
		}
	}

	// 没有找到历史
	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode([]interface{}{})
}

func main() {
	runServer()
}
