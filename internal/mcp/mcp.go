package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	glog "go-claw/pkg/log"
)

// ServerConfig MCP 服务器配置
type ServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`     // 启动命令 (stdio 模式)
	URL     string            `json:"url"`         // SSE 服务器地址
	Args    []string          `json:"args"`        // 命令参数
	Env     map[string]string `json:"env"`         // 环境变量
	Enabled bool              `json:"enabled"`     // 是否启用
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
	JSONRPC string                 `json:"jsonrpc"` // "2.0"
	ID      int                    `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
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
	config  ServerConfig
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.Reader
	nextID  int
	mu      sync.Mutex
	pending map[int]chan *JSONRPCResponse
	tools   []Tool
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
	}

	// 初始化：获取工具列表
	if err := c.initialize(ctx); err != nil {
		return fmt.Errorf("初始化 MCP 服务器失败: %v", err)
	}

	logger.Info("[MCP] 已连接", "name", c.config.Name, "tools_count", len(c.tools))
	return nil
}

// initialize 初始化连接
func (c *Client) initialize(ctx context.Context) error {
	// 发送 initialize 请求
	_, err := c.sendRequest(ctx, "initialize", map[string]interface{}{
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

	// 发送 initialized 通知
	c.sendNotification("notifications/initialized", nil)

	// 获取工具列表
	result, err := c.sendRequest(ctx, "tools/list", nil)
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
	result, err := c.sendRequest(ctx, "tools/call", map[string]interface{}{
		"name":      toolName,
		"arguments": args,
	})
	if err != nil {
		return nil, err
	}

	var toolResult ToolResult
	if err := json.Unmarshal(result, &toolResult); err != nil {
		// 如果无法解析为 ToolResult，返回文本内容
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
		c.cmd.Process.Signal(context.Done())
		c.cmd.Wait()
	}
	glog.Logger().Info("[MCP] 已断开", "name", c.config.Name)
}

// sendRequest 发送 JSON-RPC 请求
func (c *Client) sendRequest(ctx context.Context, method string, params map[string]interface{}) (json.RawMessage, error) {
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
	if c.stdin != nil {
		c.stdin.Write(data)
		c.stdin.Write([]byte("\n"))
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
func (c *Client) sendNotification(method string, params map[string]interface{}) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	data, _ := json.Marshal(req)
	if c.stdin != nil {
		c.stdin.Write(data)
		c.stdin.Write([]byte("\n"))
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