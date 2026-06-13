package service

import (
	"go-claw/config"
	"go-claw/internal/mcp"
)

// MCPService MCP 服务
type MCPService struct {
	configSvc   *ConfigService
	workspaceDir string
}

// MCPServerInfo MCP Server 信息（前端展示）
type MCPServerInfo struct {
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	URL         string            `json:"url"`
	Args        []string          `json:"args"`
	Env         map[string]string `json:"env"`
	Enabled     bool              `json:"enabled"`
	Connected   bool              `json:"connected"`   // 连接状态
	ToolsCount  int               `json:"tools_count"` // 工具数量
}

// NewMCPService 创建 MCP 服务
func NewMCPService(configSvc *ConfigService) *MCPService {
	return &MCPService{
		configSvc:    configSvc,
		workspaceDir: configSvc.WorkspaceBase(),
	}
}

// ListServers 列出指定 agent 的 MCP Server
func (s *MCPService) ListServers(agentName string) []MCPServerInfo {
	agentCfg, err := config.LoadAgentConfig(s.workspaceDir, agentName)
	if err != nil {
		return []MCPServerInfo{}
	}

	servers := agentCfg.MCP.Servers
	result := make([]MCPServerInfo, 0, len(servers))
	for _, srv := range servers {
		info := MCPServerInfo{
			Name:    srv.Name,
			Command: srv.Command,
			URL:     srv.URL,
			Args:    srv.Args,
			Env:     srv.Env,
			Enabled: srv.Enabled,
			Connected: false, // 连接状态需要从 Manager 获取
			ToolsCount: 0,
		}
		result = append(result, info)
	}
	return result
}

// CreateServer 创建 MCP Server
func (s *MCPService) CreateServer(agentName string, serverConfig config.MCPServerConfig) error {
	agentCfg, err := config.LoadAgentConfig(s.workspaceDir, agentName)
	if err != nil {
		return err
	}

	// 检查是否已存在
	for _, srv := range agentCfg.MCP.Servers {
		if srv.Name == serverConfig.Name {
			return nil // 已存在，跳过
		}
	}

	agentCfg.MCP.Servers = append(agentCfg.MCP.Servers, serverConfig)
	return config.SaveAgentConfig(s.workspaceDir, agentName, agentCfg)
}

// UpdateServer 更新 MCP Server
func (s *MCPService) UpdateServer(agentName, serverName string, serverConfig config.MCPServerConfig) error {
	agentCfg, err := config.LoadAgentConfig(s.workspaceDir, agentName)
	if err != nil {
		return err
	}

	// 找到并更新
	for i, srv := range agentCfg.MCP.Servers {
		if srv.Name == serverName {
			agentCfg.MCP.Servers[i] = serverConfig
			break
		}
	}

	return config.SaveAgentConfig(s.workspaceDir, agentName, agentCfg)
}

// DeleteServer 删除 MCP Server
func (s *MCPService) DeleteServer(agentName, serverName string) error {
	agentCfg, err := config.LoadAgentConfig(s.workspaceDir, agentName)
	if err != nil {
		return err
	}

	// 找到并删除
	newServers := make([]config.MCPServerConfig, 0, len(agentCfg.MCP.Servers))
	for _, srv := range agentCfg.MCP.Servers {
		if srv.Name != serverName {
			newServers = append(newServers, srv)
		}
	}
	agentCfg.MCP.Servers = newServers

	return config.SaveAgentConfig(s.workspaceDir, agentName, agentCfg)
}

// ToggleServer 启用/禁用 MCP Server
func (s *MCPService) ToggleServer(agentName, serverName string) (bool, error) {
	agentCfg, err := config.LoadAgentConfig(s.workspaceDir, agentName)
	if err != nil {
		return false, err
	}

	// 找到并切换状态
	var newEnabled bool
	for i, srv := range agentCfg.MCP.Servers {
		if srv.Name == serverName {
			newEnabled = !srv.Enabled
			agentCfg.MCP.Servers[i].Enabled = newEnabled
			break
		}
	}

	if err := config.SaveAgentConfig(s.workspaceDir, agentName, agentCfg); err != nil {
		return false, err
	}

	return newEnabled, nil
}

// ListTools 列出 MCP Server 的工具
func (s *MCPService) ListTools(manager *mcp.Manager, serverName string) []mcp.Tool {
	if manager == nil {
		return []mcp.Tool{}
	}

	tools := manager.ListAllTools()
	if tools == nil {
		return []mcp.Tool{}
	}

	if serverTools, ok := tools[serverName]; ok {
		return serverTools
	}
	return []mcp.Tool{}
}