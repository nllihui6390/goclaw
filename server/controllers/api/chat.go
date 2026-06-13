package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"go-claw/config"
	"go-claw/global"
	"go-claw/internal/channel"
)

// HandleCreateSession 创建新会话并返回 UUID（POST）
func HandleCreateSession(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var body map[string]string
	json.NewDecoder(r.Body).Decode(&body)
	agentName := body["agent"]
	if agentName == "" {
		agentName = "default"
	}

	result := chatSvc.CreateSession(agentName)
	rw.Header().Set("Content-Type", "application/json")
	rw.Write([]byte(result))
}

// HandleChatHistory 获取会话历史记录
func HandleChatHistory(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/chat/history/")
	sessionID := strings.TrimSuffix(path, "/")
	if sessionID == "" {
		json.NewEncoder(rw).Encode([]interface{}{})
		return
	}

	reqAgent := r.URL.Query().Get("agent")
	result := chatSvc.GetChatHistory(sessionID, reqAgent)

	rw.Header().Set("Content-Type", "application/json")
	rw.Write([]byte(result))
}

type chatRequest struct {
	Session string `json:"session"`
	Content string `json:"content"`
	Agent   string `json:"agent,omitempty"`
	Stream  bool   `json:"stream"`
}

// getConsoleChannel 从 Gateway 动态获取当前 Console 渠道（热加载注册/注销后保持同步）
func getConsoleChannel() *channel.ConsoleChannel {
	gw := global.GetGateway()
	if gw == nil {
		return nil
	}
	ch := gw.GetChannel("console")
	if ch == nil {
		return nil
	}
	cc, ok := ch.(*channel.ConsoleChannel)
	if !ok || cc.IsStopped() || !cc.IsEnabled() {
		return nil
	}
	return cc
}

// HandleChat POST /api/v1/chat — SSE 流式或阻塞式对话
func HandleChat(rw http.ResponseWriter, r *http.Request) {
	ch := getConsoleChannel()
	if ch == nil {
		writeError(rw, http.StatusServiceUnavailable, "console channel is disabled")
		return
	}
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(rw, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Content == "" {
		writeError(rw, http.StatusBadRequest, "content is required")
		return
	}

	if req.Agent == "" {
		req.Agent = r.Header.Get("X-Agent")
	}
	targetAgent := req.Agent
	if targetAgent == "" {
		if cfg := global.GetConfig(); cfg != nil {
			targetAgent = cfg.GetDefaultAgent()
		} else {
			targetAgent = "default"
		}
	}
	if !isAgentConsoleEnabled(targetAgent) {
		writeError(rw, http.StatusServiceUnavailable,
			fmt.Sprintf("console channel is disabled for agent %s", targetAgent))
		return
	}
	req.Agent = targetAgent

	msgID := fmt.Sprintf("rest-%d", time.Now().UnixNano())
	if req.Session == "" {
		req.Session = fmt.Sprintf("rest-user-%d", time.Now().UnixNano())
	}

	handleChatRequest(ch, msgID, req.Session, req.Content, req.Agent, req.Stream, rw, r)
}

func isAgentConsoleEnabled(agentName string) bool {
	cfg := global.GetConfig()
	if cfg == nil {
		return true
	}
	workspaceDir := filepath.Join(global.GetDataDir(), cfg.Gateway.Workspace)
	return config.IsAgentChannelEnabled(workspaceDir, agentName, "console")
}

func handleChatRequest(ch *channel.ConsoleChannel, msgID, session, content, agentName string, stream bool, rw http.ResponseWriter, r *http.Request) {
	// 渠道配置关闭流式输出时，强制走阻塞模式
	if stream && !ch.GetDisplay().StreamOutput {
		stream = false
	}
	if stream {
		streamCh, fileCh, cleanup := ch.PrepareStream(session)
		defer cleanup()

		go ch.PushMessage(channel.Message{
			ID: msgID, Channel: ch.GetName(), From: session,
			Content: content, Agent: agentName, Timestamp: time.Now().Unix(),
		})

		rw.Header().Set("Content-Type", "text/event-stream")
		rw.Header().Set("Cache-Control", "no-cache")
		rw.Header().Set("Connection", "keep-alive")
		rw.Header().Set("X-Accel-Buffering", "no") // 禁用 Nginx 缓冲
		rw.WriteHeader(http.StatusOK)
		flusher := rw.(http.Flusher)

		fmt.Fprintf(rw, "event: start\ndata: {\"session\":\"%s\",\"id\":\"%s\"}\n\n", session, msgID)
		flusher.Flush()

		// 定时发送 keepalive 注释，防止中间代理超时（每 15 秒）
		keepaliveTicker := time.NewTicker(15 * time.Second)
		defer keepaliveTicker.Stop()

		for {
			select {
			case chunk, ok := <-streamCh:
				if !ok {
					fmt.Fprintf(rw, "event: done\ndata: {}\n\n")
					flusher.Flush()
					return
				}
				data, _ := json.Marshal(map[string]string{"content": chunk})
				fmt.Fprintf(rw, "event: chunk\ndata: %s\n\n", data)
				flusher.Flush()
			case fileEvt := <-fileCh:
				switch fileEvt.Type {
				case channel.ToolEventFile:
					fileData, _ := json.Marshal(map[string]interface{}{
						"fileType": fileEvt.Args,
						"path":     fileEvt.Result,
						"filename": fileEvt.ToolName,
						"size":     0,
					})
					fmt.Fprintf(rw, "event: file\ndata: %s\n\n", fileData)
					flusher.Flush()
				case channel.ToolEventContent:
					contentData, _ := json.Marshal(map[string]interface{}{
						"blocks": fileEvt.Content,
					})
					fmt.Fprintf(rw, "event: content\ndata: %s\n\n", contentData)
					flusher.Flush()
				case channel.ToolEventThinking:
					thinkData, _ := json.Marshal(map[string]interface{}{
						"thinking": fileEvt.Thinking,
					})
					fmt.Fprintf(rw, "event: thinking\ndata: %s\n\n", thinkData)
					flusher.Flush()
				case channel.ToolEventCalling:
					callData, _ := json.Marshal(map[string]interface{}{
						"tool_name": fileEvt.ToolName,
						"args":      fileEvt.Args,
					})
					fmt.Fprintf(rw, "event: tool_call\ndata: %s\n\n", callData)
					flusher.Flush()
				case channel.ToolEventResult:
					resultData, _ := json.Marshal(map[string]interface{}{
						"tool_name": fileEvt.ToolName,
						"result":    fileEvt.Result,
					})
					fmt.Fprintf(rw, "event: tool_result\ndata: %s\n\n", resultData)
					flusher.Flush()
				case channel.ToolEventError:
					errorData, _ := json.Marshal(map[string]interface{}{
						"tool_name": fileEvt.ToolName,
						"error":     fileEvt.Error,
					})
					fmt.Fprintf(rw, "event: tool_error\ndata: %s\n\n", errorData)
					flusher.Flush()
				case channel.ToolEventGuard:
					guardData, _ := json.Marshal(map[string]interface{}{
						"tool_name":      fileEvt.ToolName,
						"args":           fileEvt.Args,
						"guard_message":  fileEvt.GuardMessage,
						"approval_id":    fileEvt.ApprovalID,
						"approval_state": fileEvt.ApprovalState,
					})
					fmt.Fprintf(rw, "event: guard\ndata: %s\n\n", guardData)
					flusher.Flush()
				}
			case <-keepaliveTicker.C:
				// SSE 注释作为心跳，保持连接活跃（不触发前端 event handler）
				fmt.Fprintf(rw, ": keepalive\n\n")
				flusher.Flush()
			case <-r.Context().Done():
				// 客户端断开连接（关闭浏览器、网络中断）
				return
			}
		}
	}

	respChan, cleanup := ch.PrepareBlocking(session)
	defer cleanup()

	if !ch.PushMessage(channel.Message{
		ID: msgID, Channel: ch.GetName(), From: session,
		Content: content, Agent: agentName, Timestamp: time.Now().Unix(),
	}) {
		writeError(rw, http.StatusServiceUnavailable, "console channel is stopped")
		return
	}

	select {
	case resp := <-respChan:
		writeJSON(rw, http.StatusOK, map[string]string{"response": resp.Content})
	case <-time.After(120 * time.Second):
		writeError(rw, http.StatusGatewayTimeout, "timeout")
	}
}
