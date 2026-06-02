//go:build !production

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

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

	// 注入定时任务手动执行回调
	server.SetCronExecutor(func(id string) {
		handleCronRun(id)
	})

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

// handleCronRun 手动执行定时任务（异步）
func handleCronRun(id string) {
	// 读取任务
	data, err := os.ReadFile("clawdata/cron_jobs.json")
	if err != nil {
		return
	}
	var jobs []map[string]interface{}
	if err := json.Unmarshal(data, &jobs); err != nil {
		return
	}

	var job map[string]interface{}
	for _, j := range jobs {
		if j["id"] == id {
			job = j
			break
		}
	}
	if job == nil {
		return
	}

	jobType, _ := job["type"].(string)
	content, _ := job["content"].(string)
	sessionID, _ := job["session_id"].(string)
	agentName, _ := job["agent_name"].(string)
	jobName, _ := job["name"].(string)

	if sessionID == "" {
		sessionID = "console:cron"
	}

	go func() {
		logger := glog.Logger()
		logger.Info("[Cron] 手动执行任务", "id", id, "name", jobName, "type", jobType)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		switch jobType {
		case "text":
			// 文本消息直接发送到渠道
			if err := gatewayInstance.Gateway.SendProactiveMessage(ctx, sessionID, content); err != nil {
				logger.Warn("[Cron] 文本任务发送失败", "id", id, "err", err)
			} else {
				logger.Info("[Cron] 文本任务已发送", "id", id, "session", sessionID)
			}

		case "agent":
			agents := gatewayInstance.Gateway.GetAgents()
			ag := agents["default"]
			if agentName != "" {
				if ag2, ok := agents[agentName]; ok {
					ag = ag2
				}
			}
			if ag == nil {
				logger.Warn("[Cron] Agent 未找到", "agent", agentName)
				return
			}
			result, err := ag.Process(ctx, sessionID, content)
			if err != nil {
				logger.Warn("[Cron] Agent 任务执行失败", "id", id, "err", err)
				return
			}
			logger.Info("[Cron] Agent 任务执行完成", "id", id, "result_len", len(result))
			// 将结果发送到渠道
			if err := gatewayInstance.Gateway.SendProactiveMessage(ctx, sessionID, result); err != nil {
				logger.Warn("[Cron] Agent 结果发送失败", "id", id, "session", sessionID, "err", err)
			} else {
				logger.Info("[Cron] Agent 结果已发送", "id", id, "session", sessionID)
			}

		default:
			logger.Warn("[Cron] 未知任务类型", "type", jobType)
		}
	}()
}

func main() {
	runServer()
}
