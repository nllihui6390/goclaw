package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	glog "go-claw/pkg/log"
	"go-claw/pkg/utils"
)

// ServerConfig MCP 服务器配置
type ServerConfig struct {
	Key         string            `json:"key"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Enabled     bool              `json:"enabled"`
	Transport   string            `json:"transport"` // stdio / streamable_http / sse
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Env         map[string]string `json:"env"`
	Cwd         string            `json:"cwd"`
}

// Tool MCP 工具定义
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ToolResult 工具执行结果
type ToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError"`
}

// ContentItem 内容项
type ContentItem struct {
	Type string `json:"type"` // "text", "image"
	Text string `json:"text,omitempty"`
}

// JSONRPCRequest JSON-RPC 请求
type JSONRPCRequest struct {
	JSONRPC string                  `json:"jsonrpc"`
	ID      int                     `json:"id"`
	Method  string                  `json:"method"`
	Params  *map[string]interface{} `json:"params,omitempty"`
}

// JSONRPCNotification JSON-RPC 通知（无 id）
type JSONRPCNotification struct {
	JSONRPC string                  `json:"jsonrpc"`
	Method  string                  `json:"method"`
	Params  *map[string]interface{} `json:"params,omitempty"`
}

// JSONRPCResponse JSON-RPC 响应
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError RPC 错误
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Client MCP 客户端
type Client struct {
	config     ServerConfig
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.Reader
	httpClient *http.Client
	sessionID  string // mcp-session-id（HTTP 模式）
	nextID     int
	mu         sync.RWMutex
	pending    map[int]chan *JSONRPCResponse
	tools      []Tool
}

// NewClient 创建 MCP 客户端
func NewClient(config ServerConfig) *Client {
	return &Client{
		config:  config,
		pending: make(map[int]chan *JSONRPCResponse),
	}
}

// Connect 连接到 MCP 服务器
func (c *Client) Connect(ctx context.Context) error {
	logger := glog.Logger()
	logger.Info("[MCP] 连接服务器", "name", c.config.Name)

	if c.config.Command != "" {
		// stdio 模式
		args := c.config.Args
		cmd := exec.CommandContext(ctx, c.config.Command, args...)

		// 设置环境变量
		for k, v := range c.config.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}

		stdin, err := cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("创建 stdin 管道失败: %v", err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("创建 stdout 管道失败: %v", err)
		}

		c.cmd = cmd
		c.stdin = stdin
		c.stdout = stdout

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("启动 MCP 服务器失败: %v", err)
		}

		// 启动消息读取协程
		go c.readResponses()
	} else if c.config.URL != "" {
		// HTTP/SSE 模式
		c.httpClient = &http.Client{Timeout: 60 * time.Second}

		// SSE 模式：先 GET 获取 endpoint，再 POST
		if c.config.Transport == "sse" {
			endpointURL, err := c.sseConnect(ctx)
			if err != nil {
				return fmt.Errorf("SSE 连接失败: %v", err)
			}
			if endpointURL != "" {
				c.config.URL = endpointURL
			}
		}
	}

	// 初始化：获取工具列表
	if err := c.initialize(ctx); err != nil {
		// 如果 POST 失败且不是 SSE 模式，尝试 SSE
		if c.httpClient != nil && c.config.Transport != "sse" {
			glog.Logger().Info("[MCP] POST 初始化失败，尝试 SSE 模式", "name", c.config.Name)
			endpointURL, sseErr := c.sseConnect(ctx)
			if sseErr == nil && endpointURL != "" {
				c.config.URL = endpointURL
				if err2 := c.initialize(ctx); err2 != nil {
					return fmt.Errorf("SSE 初始化也失败: %v (原始: %v)", err2, err)
				}
				// SSE 成功，继续
			} else {
				return fmt.Errorf("初始化 MCP 服务器失败: %v", err)
			}
		} else {
			return fmt.Errorf("初始化 MCP 服务器失败: %v", err)
		}
	}

	logger.Info("[MCP] 已连接", "name", c.config.Name, "tools_count", len(c.tools))
	return nil
}

// sseConnect SSE 模式连接：GET 获取 endpoint
func (c *Client) sseConnect(ctx context.Context) (string, error) {
	logger := glog.Logger()
	logger.Info("[MCP] SSE 连接", "url", c.config.URL)

	req, err := http.NewRequestWithContext(ctx, "GET", c.config.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("SSE GET 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("SSE GET HTTP %d: %s", resp.StatusCode, utils.Truncate(string(body), 200))
	}

	// 解析 SSE 流，提取 endpoint（仅读取前面少量事件）
	scanner := bufio.NewScanner(resp.Body)
	var eventType, data string
	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineCount++
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		} else if strings.TrimSpace(line) == "" && eventType != "" {
			if eventType == "endpoint" && data != "" {
				logger.Info("[MCP] SSE endpoint 获取成功", "endpoint", data)
				return data, nil
			}
			eventType = ""
			data = ""
		}
		if lineCount > 50 {
			break
		}
	}
	// 没有找到 endpoint，使用原 URL 作为 POST 端点
	logger.Info("[MCP] SSE 未找到 endpoint 事件，使用原 URL")
	return "", nil
}

// initialize 初始化连接
func (c *Client) initialize(ctx context.Context) error {
	// 发送 initialize 请求
	_, err := c.sendRequest(ctx, "initialize", &map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "go-claw",
			"version": "1.0.0",
		},
	})
	if err != nil {
		return err
	}

	// 发送 initialized 通知（无 params）
	c.sendNotification("notifications/initialized", nil)

	// 获取工具列表
	emptyParams := map[string]interface{}{}
	result, err := c.sendRequest(ctx, "tools/list", &emptyParams)
	if err != nil {
		return err
	}

	// 解析工具列表
	var toolsList struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(result, &toolsList); err != nil {
		return err
	}
	c.tools = toolsList.Tools

	return nil
}

// CallTool 调用 MCP 工具
func (c *Client) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (*ToolResult, error) {
	result, err := c.callTool(ctx, toolName, args)

	// 如果 session 过期（HTTP 401 或 SessionExpired），重新连接后重试一次
	if err != nil && c.httpClient != nil && (strings.Contains(err.Error(), "SessionExpired") || strings.Contains(err.Error(), "HTTP 401")) {
		glog.Logger().Info("[MCP] Session 过期，重新连接", "name", c.config.Name)
		c.sessionID = ""
		if reconnErr := c.initialize(ctx); reconnErr != nil {
			return nil, fmt.Errorf("重新连接失败: %v (原始错误: %v)", reconnErr, err)
		}
		result, err = c.callTool(ctx, toolName, args)
	}

	return result, err
}

func (c *Client) callTool(ctx context.Context, toolName string, args map[string]interface{}) (*ToolResult, error) {
	result, err := c.sendRequest(ctx, "tools/call", &map[string]interface{}{
		"name":      toolName,
		"arguments": args,
	})
	if err != nil {
		return nil, err
	}

	var toolResult ToolResult
	if err := json.Unmarshal(result, &toolResult); err != nil {
		return &ToolResult{
			Content: []ContentItem{{Type: "text", Text: string(result)}},
		}, nil
	}

	return &toolResult, nil
}

// ListTools 列出可用工具
func (c *Client) ListTools() []Tool {
	return c.tools
}

// Disconnect 断开连接
func (c *Client) Disconnect() {
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Signal(os.Interrupt)
		c.cmd.Wait()
	}
	glog.Logger().Info("[MCP] 已断开", "name", c.config.Name)
}

// sendRequest 发送 JSON-RPC 请求
func (c *Client) sendRequest(ctx context.Context, method string, params *map[string]interface{}) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	ch := make(chan *JSONRPCResponse, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, _ := json.Marshal(req)
	glog.Logger().Info("[MCP] 发送请求", "method", method, "body", string(data))

	if c.stdin != nil {
		// stdio 模式
		c.stdin.Write(data)
		c.stdin.Write([]byte("\n"))
	} else if c.httpClient != nil {
		// HTTP 模式
		httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.URL, bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("创建 HTTP 请求失败: %v", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json, text/event-stream")
		for k, v := range c.config.Headers {
			httpReq.Header.Set(k, v)
		}
		if c.sessionID != "" {
			httpReq.Header.Set("mcp-session-id", c.sessionID)
		}
		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("HTTP 请求失败: %v", err)
		}
		defer resp.Body.Close()

		// 保存 session ID
		if sid := resp.Header.Get("mcp-session-id"); sid != "" {
			c.sessionID = sid
		}

		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		// 处理 SSE 格式响应：提取 data: 行的 JSON
		if strings.Contains(bodyStr, "event:") || strings.Contains(bodyStr, "data:") {
			bodyStr = extractSSEData(bodyStr)
			body = []byte(bodyStr)
		}
		glog.Logger().Info("[MCP] HTTP 响应",
			"status", resp.StatusCode,
			"method", method,
			"body", utils.Truncate(string(body), 500),
			"session_id", c.sessionID,
			"sent_session_id", httpReq.Header.Get("mcp-session-id"),
		)
		if resp.StatusCode != 200 && resp.StatusCode != 202 {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, utils.Truncate(string(body), 200))
		}

		// 202 Accepted 表示服务器接受了请求但没有响应体
		if resp.StatusCode == 202 || len(body) == 0 {
			if method == "tools/list" {
				return json.RawMessage(`{"tools":[]}`), nil
			}
			return json.RawMessage(`{}`), nil
		}

		var rpcResp JSONRPCResponse
		if err := json.Unmarshal(body, &rpcResp); err != nil {
			return nil, fmt.Errorf("解析 HTTP 响应失败: %v (body: %s)", err, utils.Truncate(string(body), 200))
		}
		if rpcResp.Error != nil {
			return nil, fmt.Errorf("MCP 错误 [%d]: %s (body: %s)", rpcResp.Error.Code, rpcResp.Error.Message, utils.Truncate(string(body), 200))
		}
		return rpcResp.Result, nil
	} else {
		return nil, fmt.Errorf("MCP 客户端未初始化传输")
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("MCP 错误 [%d]: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("MCP 请求超时")
	}
}

// sendNotification 发送 JSON-RPC 通知
func (c *Client) sendNotification(method string, params *map[string]interface{}) {
	req := JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	data, _ := json.Marshal(req)
	glog.Logger().Info("[MCP] 发送通知", "method", method, "body", string(data))
	if c.stdin != nil {
		c.stdin.Write(data)
		c.stdin.Write([]byte("\n"))
	} else if c.httpClient != nil {
		httpReq, err := http.NewRequest("POST", c.config.URL, bytes.NewReader(data))
		if err != nil {
			glog.Logger().Warn("[MCP] 通知创建请求失败", "method", method, "err", err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json, text/event-stream")
		if c.sessionID != "" {
			httpReq.Header.Set("mcp-session-id", c.sessionID)
		}
		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			glog.Logger().Warn("[MCP] 通知发送失败", "method", method, "err", err)
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		glog.Logger().Info("[MCP] 通知响应", "method", method, "status", resp.StatusCode, "body", string(body))
	}
}

// readResponses 读取响应协程
func (c *Client) readResponses() {
	if c.stdout == nil {
		return
	}

	scanner := bufio.NewScanner(c.stdout)
	for scanner.Scan() {
		line := scanner.Text()
		var resp JSONRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}

		c.mu.RLock()
		if ch, exists := c.pending[resp.ID]; exists {
			ch <- &resp
		}
		c.mu.RUnlock()
	}
}

// Manager MCP 服务器管理器
type Manager struct {
	clients map[string]*Client
	mu      sync.RWMutex
}

// NewManager 创建 MCP 管理器
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*Client),
	}
}

// Register 注册 MCP 服务器
func (m *Manager) Register(config ServerConfig) {
	if !config.Enabled {
		return
	}

	client := NewClient(config)
	m.mu.Lock()
	m.clients[config.Name] = client
	m.mu.Unlock()
}

// ConnectAll 连接所有已注册的服务器
func (m *Manager) ConnectAll(ctx context.Context) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, client := range m.clients {
		if err := client.Connect(ctx); err != nil {
			glog.Logger().Warn("[MCP] 连接失败", "name", name, "err", err)
		}
	}
}

// DisconnectAll 断开所有连接
func (m *Manager) DisconnectAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, client := range m.clients {
		client.Disconnect()
	}
}

// CallTool 通过指定服务器调用工具
func (m *Manager) CallTool(ctx context.Context, serverName, toolName string, args map[string]interface{}) (*ToolResult, error) {
	m.mu.RLock()
	client, exists := m.clients[serverName]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("MCP 服务器不存在: %s", serverName)
	}

	return client.CallTool(ctx, toolName, args)
}

// ListAllTools 列出所有可用工具
func (m *Manager) ListAllTools() map[string][]Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]Tool)
	for name, client := range m.clients {
		result[name] = client.ListTools()
	}
	return result
}

// GetClient 获取指定名称的 MCP 客户端
func (m *Manager) GetClient(name string) *Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clients[name]
}

// HasClients 检查是否有已注册的客户端
func (m *Manager) HasClients() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients) > 0
}

// MCPToolAdapter 将 MCP Tool 适配为 go-claw tool.Tool 接口
type MCPToolAdapter struct {
	mcpTool     Tool
	client      *Client
	serverName  string
	toolName    string
	description string
	params      map[string]interface{}
}

// NewMCPToolAdapter 创建 MCP 工具适配器
func NewMCPToolAdapter(serverName string, client *Client, mcpTool Tool) *MCPToolAdapter {
	return &MCPToolAdapter{
		mcpTool:     mcpTool,
		client:      client,
		serverName:  serverName,
		toolName:    mcpTool.Name,
		description: mcpTool.Description,
		params:      mcpTool.InputSchema,
	}
}

func (a *MCPToolAdapter) Name() string {
	return "mcp_" + a.serverName + "_" + a.toolName
}

func (a *MCPToolAdapter) Description() string {
	return "[MCP:" + a.serverName + "] " + a.description
}

func (a *MCPToolAdapter) Parameters() map[string]interface{} {
	if a.params == nil {
		return map[string]interface{}{}
	}
	return a.params
}

func (a *MCPToolAdapter) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	result, err := a.client.CallTool(ctx, a.toolName, params)
	if err != nil {
		return "", err
	}

	if result.IsError {
		text := ""
		for _, item := range result.Content {
			if item.Type == "text" {
				text += item.Text
			}
		}
		return "", fmt.Errorf("MCP 工具错误: %s", text)
	}

	text := ""
	for _, item := range result.Content {
		if item.Type == "text" {
			text += item.Text
		}
	}
	if text == "" {
		text = "工具执行完成（无文本输出）"
	}
	return text, nil
}

// CreateMCPToolsFromManager 从 MCP Manager 创建所有工具的适配器列表
// extractSSEData 从 SSE 格式响应中提取 JSON-RPC 数据
// 输入: "event: message\ndata: {"jsonrpc":"2.0",...}\n\n"
// 输出: "{"jsonrpc":"2.0",...}"
func extractSSEData(raw string) string {
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			val := strings.TrimPrefix(line, "data:")
			return strings.TrimSpace(val)
		}
	}
	return raw
}

func CreateMCPToolsFromManager(mgr *Manager, ctx context.Context) []*MCPToolAdapter {
	allTools := mgr.ListAllTools()
	var adapters []*MCPToolAdapter
	for serverName := range allTools {
		client := mgr.GetClient(serverName)
		if client == nil {
			continue
		}
		tools := allTools[serverName]
		for _, t := range tools {
			adapters = append(adapters, NewMCPToolAdapter(serverName, client, t))
		}
	}
	return adapters
}