package bootstrap

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"

	"go-claw/config"
	"go-claw/internal/agent"
	"go-claw/internal/memory"
	"go-claw/internal/skill"
	"go-claw/internal/store"
	"go-claw/internal/tool"
	"go-claw/internal/workspace"
)

// SyncAgents 同步 Agent 配置（热加载）
// 对比新旧配置，增删改 Agent
func (app *App) SyncAgents(newCfg *config.Config) {
	app.logger.Info("开始同步 Agent 配置")
	// workspace 目录
	workspaceDir := filepath.Join(newCfg.Gateway.DataDir, newCfg.Gateway.Workspace)
	// 获取当前已注册的 agent 名称
	currentAgents := app.Gateway.GetAgents()
	currentNames := make(map[string]bool)
	for name := range currentAgents {
		currentNames[name] = true
	}
	// 新配置中的 agent 名称（从 profiles）
	newNames := make(map[string]bool)
	for name, profile := range newCfg.Agents.Profiles {
		if profile.Enabled {
			newNames[name] = true
		}
	}
	// 1. 删除不再存在或禁用的 Agent
	for name := range currentNames {
		if !newNames[name] {
			app.Gateway.UnregisterAgent(name)
			app.logger.Info("Agent 已移除", "name", name)
		}
	}

	// 2. 新增或更新 Agent
	for name := range newNames {
		app.createOrUpdateAgentFromJSON(name, workspaceDir, newCfg)
	}

	// 3. 更新默认 Agent
	if newCfg.Agents.DefaultAgent != "" {
		app.Gateway.SetDefaultAgent(newCfg.Agents.DefaultAgent)
	}

	// 4. 更新 App.Config
	app.Config = newCfg

	app.logger.Info("Agent 配置同步完成", "total", len(newNames))
}

// 注销并且删除指定 Agent - 并且删除 agent 工作空间目录
func (app *App) DeleteAgent(name string) error {
	app.Gateway.UnregisterAgent(name)
	// 删除 agent 工作空间目录
	workspaceDir := filepath.Join(app.Config.Gateway.DataDir, app.Config.Gateway.Workspace)
	agentWorkspaceDir := filepath.Join(workspaceDir, name)
	os.RemoveAll(agentWorkspaceDir)
	slog.Info("已移除 Agent", "name", name)
	return nil
}

// 热加载单个 Agent 配置（热加载）
func (app *App) ReloadAgent(agentName string) error {
	workspaceDir := filepath.Join(app.Config.Gateway.DataDir, app.Config.Gateway.Workspace)
	// 注销 agent
	app.Gateway.UnregisterAgent(agentName)
	// 重新创建或更新 agent
	app.createOrUpdateAgentFromJSON(agentName, workspaceDir, app.Config)
	return nil
}

// createOrUpdateAgentFromJSON 从 agent.json 创建或更新单个 Agent
func (app *App) createOrUpdateAgentFromJSON(agentName, workspaceDir string, rootCfg *config.Config) {
	// 加载 agent.json
	agentCfg, err := config.LoadAgentConfig(workspaceDir, agentName)
	if err != nil {
		app.logger.Warn("加载 Agent 配置失败，使用默认配置", "agent", agentName, "err", err)
		agentCfg = config.GetDefaultAgentConfig(agentName, rootCfg.Gateway.DefaultProvider, rootCfg.Gateway.DefaultModel)
	}

	// 确保 agent 工作空间目录存在
	agentWorkspaceDir := filepath.Join(workspaceDir, agentName)
	agentSessionsDir := filepath.Join(agentWorkspaceDir, "sessions")
	initDataDirs(agentWorkspaceDir, agentSessionsDir, app.logger)

	tools := loadTools(agentCfg.Tools)

	// 解析配置：从 provider 获取
	model, baseURL, apiKey, providerType := rootCfg.ResolveAgentConfig(agentCfg)
	supportsImage, supportsVideo := rootCfg.ResolveAgentModelConfig(agentCfg)

	// 创建该 agent 专属的工作空间加载器
	wsLoader := workspace.NewLoaderWithAgent(agentWorkspaceDir, agentName)

	// 创建该 agent 专属的存储
	agentStore, err := store.NewFileStore(agentSessionsDir)
	if err != nil {
		app.logger.Error("初始化 Agent 存储失败", "agent", agentName, "err", err)
		return
	}

	// 加载该 agent 启用的技能
	agentSkillReg := skill.NewRegistry(filepath.Join(app.DataDir, "skills"))
	enabledSkills := loadEnabledSkills(agentWorkspaceDir)
	if len(enabledSkills) > 0 {
		agentSkillReg.LoadEnabled(filepath.Join(app.DataDir, "skills"), enabledSkills)
	}

	ag := agent.NewAgent(&agent.Config{
		Name:                  agentName,
		SystemPrompt:          agentCfg.SystemPrompt,
		Model:                 model,
		APIKey:                apiKey,
		BaseURL:               baseURL,
		ProviderType:          providerType,
		Tools:                 tools,
		MaxIterations:         agentCfg.MaxIterations,
		MaxTokens:             agentCfg.MaxTokens,
		Memory:                memory.NewSimpleMemory(agentStore),
		Store:                 agentStore,
		WorkspaceLoader:       wsLoader,
		WorkspaceDir:          agentWorkspaceDir,
		SkillRegistry:         agentSkillReg,
		CompactThresholdRatio: agentCfg.CompactThresholdRatio,
		ReserveThresholdRatio: agentCfg.ReserveThresholdRatio,
		ToolResultMaxBytes:    agentCfg.ToolResultMaxBytes,
		ToolResultExemptTools: agentCfg.ToolResultExemptTools,
		ToolResultExemptExts:  agentCfg.ToolResultExemptExts,
		SupportsImage:         supportsImage,
		SupportsVideo:         supportsVideo,
		ToolGuard:             app.ToolGuard,
		// 动态配置提供器：每次调用 LLM 时实时读取 app.Config（指针），改配置即生效
		ConfigProvider: func() (string, string, string, string) {
			// app.Config 是指针，外部 global.SetConfig 更新后这里自动读到新值
			curAgentCfg, _ := config.LoadAgentConfig(workspaceDir, agentName)
			m, b, k, p := app.Config.ResolveAgentConfig(curAgentCfg)
			return m, k, b, p
		},
	})
	app.Gateway.RegisterAgent(agentName, ag)
	app.logger.Info("Agent 已注册/更新", "name", agentName, "provider", agentCfg.Provider, "model", model, "skills", len(enabledSkills))
}

// loadEnabledSkills 从 agent 工作空间读取 enabled_skills.json
func loadEnabledSkills(agentWorkspaceDir string) []string {
	file := filepath.Join(agentWorkspaceDir, "enabled_skills.json")
	data, err := os.ReadFile(file)
	if err != nil {
		return []string{}
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return []string{}
	}
	return list
}

// initAgents 注册所有 Agent
func (app *App) initAgents() {
	workspaceDir := filepath.Join(app.DataDir, app.Workspace)

	// 确保全局技能目录存在
	globalSkillsDir := filepath.Join(app.DataDir, "skills")
	os.MkdirAll(globalSkillsDir, 0755)

	// 扫描 workspace 目录发现所有 agent
	agentNames, err := config.ListAgentConfigs(workspaceDir)
	if err != nil {
		app.logger.Warn("扫描 Agent 目录失败", "err", err)
		agentNames = []string{}
	}

	// 如果没有发现任何 agent，创建默认 agent
	if len(agentNames) == 0 {
		defaultAgent := app.Config.GetDefaultAgent()
		if defaultAgent == "" {
			defaultAgent = "default"
		}
		os.MkdirAll(filepath.Join(workspaceDir, defaultAgent), 0755)
		agentCfg := config.GetDefaultAgentConfig(defaultAgent, app.Config.Gateway.DefaultProvider, app.Config.Gateway.DefaultModel)
		if err := config.SaveAgentConfig(workspaceDir, defaultAgent, agentCfg); err != nil {
			app.logger.Error("创建默认 Agent 配置失败", "err", err)
		} else {
			agentNames = []string{defaultAgent}
			// 更新根配置的 profiles
			app.Config.Agents.Profiles = map[string]config.AgentProfileRef{defaultAgent: {Enabled: true}}
			app.Config.Agents.Order = []string{defaultAgent}
			app.Config.Agents.DefaultAgent = defaultAgent
		}
	}

	// 每个 agent 拥有独立的配置，按各自的 agent.json 加载
	for _, agentName := range agentNames {
		// 检查是否在 profiles 中且启用
		if profile, ok := app.Config.Agents.Profiles[agentName]; ok {
			if !profile.Enabled {
				app.logger.Info("Agent 已禁用，跳过初始化", "name", agentName)
				continue
			}
		}
		app.createOrUpdateAgentFromJSON(agentName, workspaceDir, app.Config)
	}
}

// ReloadAgentSkills 重载指定 agent 的技能注册表（动态生效）
func (app *App) ReloadAgentSkills(agentName string, enabledSkills []string) {
	agents := app.Gateway.GetAgents()
	ag, exists := agents[agentName]
	if !exists {
		app.logger.Warn("ReloadAgentSkills: Agent 不存在", "agent", agentName)
		return
	}

	newReg := skill.NewRegistry(filepath.Join(app.DataDir, "skills"))
	if len(enabledSkills) > 0 {
		newReg.LoadEnabled(filepath.Join(app.DataDir, "skills"), enabledSkills)
	}
	ag.SetSkillRegistry(newReg)
	app.logger.Info("Agent Skill 已动态重载", "agent", agentName, "skills", len(enabledSkills))
}

// initDataDirs 初始化数据目录结构和人设文件
func initDataDirs(workspaceDir, sessionsDir string, logger *slog.Logger) {
	dirs := []string{workspaceDir, sessionsDir}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			logger.Error("创建目录失败", "dir", d, "err", err)
		}
	}
	// 使用共享的人设文件初始化逻辑
	workspace.InitPersonaFiles(workspaceDir)

	logger.Info("数据目录已初始化", "workspace", workspaceDir)
}

// loadTools 使用注册表加载工具（合并默认工具 + agent 配置的显式工具）
func loadTools(toolNames []string) []tool.Tool {
	// 收集所有要加载的工具名：默认工具 + agent 显式配置
	defaults := tool.GlobalRegistry.DefaultTools()
	allNames := make(map[string]bool)
	for _, n := range defaults {
		allNames[n] = true
	}
	for _, n := range toolNames {
		allNames[n] = true
	}

	var tools []tool.Tool
	for name := range allNames {
		t, err := tool.GlobalRegistry.Create(name)
		if err != nil {
			continue
		}
		if name == "weather" {
			tools = append(tools, tool.NewWeatherToolWithConfig(tool.WeatherAPIConfig{
				Type:   "hefeng",
				APIKey: os.Getenv("HEFENG_API_KEY"),
			}))
			continue
		}
		tools = append(tools, t)
	}
	return tools
}

// GetAgentConfig 获取指定 agent 的配置（从 agent.json 加载）
func (app *App) GetAgentConfig(agentName string) (*config.AgentConfig, error) {
	workspaceDir := filepath.Join(app.DataDir, app.Workspace)
	return config.LoadAgentConfig(workspaceDir, agentName)
}

// SaveAgentConfig 保存指定 agent 的配置
func (app *App) SaveAgentConfig(agentName string, agentCfg *config.AgentConfig) error {
	workspaceDir := filepath.Join(app.DataDir, app.Workspace)
	// 确保 agent 目录存在
	os.MkdirAll(filepath.Join(workspaceDir, agentName), 0755)
	return config.SaveAgentConfig(workspaceDir, agentName, agentCfg)
}

// ListAgentConfigs 返回所有 agent 配置名称
func (app *App) ListAgentConfigs() ([]string, error) {
	workspaceDir := filepath.Join(app.DataDir, app.Workspace)
	return config.ListAgentConfigs(workspaceDir)
}
