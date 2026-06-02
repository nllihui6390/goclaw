package bootstrap

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"go-claw/internal/agent"
	"go-claw/internal/memory"
	"go-claw/internal/skill"
	"go-claw/internal/store"
	"go-claw/internal/tool"
	"go-claw/internal/workspace"

	"github.com/fsnotify/fsnotify"
)

// initAgents 注册所有 Agent
func (app *App) initAgents() {
	// 全局技能目录（所有 agent 共享）
	globalSkillsDir := app.DataDir + "/skills"
	os.MkdirAll(globalSkillsDir, 0755)

	for _, agentCfg := range app.Config.Agents {
		tools := loadTools(agentCfg.Tools)

		// 解析配置：从provider获取
		model, baseURL, apiKey, providerType := app.Config.ResolveAgentConfig(&agentCfg)
	supportsImage, supportsVideo := app.Config.ResolveAgentModelConfig(&agentCfg)

		// 每个 agent 有独立的工作空间目录: clawdata/workspaces/<agent-name>/
		agentWorkspaceDir := app.DataDir + "/" + app.Workspace + "/" + agentCfg.Name
		agentSessionsDir := agentWorkspaceDir + "/sessions"
		agentSkillsDir := agentWorkspaceDir + "/skills"

		// 初始化 agent 的工作空间目录和人设文件
		initDataDirs(agentWorkspaceDir, agentSessionsDir, agentSkillsDir, app.logger)

		// 创建该 agent 专属的工作空间加载器
		wsLoader := workspace.NewLoaderWithAgent(agentWorkspaceDir, agentCfg.Name)

		// 创建该 agent 专属的存储
		agentStore, err := store.NewFileStore(agentSessionsDir)
		if err != nil {
			app.logger.Error("初始化 Agent 存储失败", "agent", agentCfg.Name, "err", err)
			continue
		}

		// 初始化该 agent 专属的 Skill 系统（全局 + agent 特定）
		var skillReg *skill.Registry
		skillReg = skill.NewRegistry(globalSkillsDir)
		skillReg.AddDir(agentSkillsDir)
		if err := skillReg.LoadAll(); err != nil {
			app.logger.Warn("加载全局 Skill 目录失败", "err", err)
		}
		globalCount := len(skillReg.List())
		if err := skillReg.LoadFromDir(agentSkillsDir); err != nil {
			app.logger.Warn("加载 Agent Skill 目录失败", "agent", agentCfg.Name, "err", err)
		}
		agentCount := len(skillReg.List()) - globalCount

		app.skillRegistries[agentCfg.Name] = skillReg
		app.skillRegistryDirs[agentCfg.Name] = agentSkillsDir

		if len(skillReg.List()) > 0 {
			app.logger.Info("Agent Skill 已加载（Prompt-based 模式）", "agent", agentCfg.Name, "global", globalCount, "agent_specific", agentCount, "total", len(skillReg.List()))
		}

		ag := agent.NewAgent(&agent.Config{
			Name:                  agentCfg.Name,
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
			SkillRegistry:         skillReg,
			CompactThresholdRatio: agentCfg.CompactThresholdRatio,
			ReserveThresholdRatio: agentCfg.ReserveThresholdRatio,
			ToolResultMaxBytes:    agentCfg.ToolResultMaxBytes,
			ToolResultExemptTools: agentCfg.ToolResultExemptTools,
			ToolResultExemptExts:  agentCfg.ToolResultExemptExts,
			SupportsImage:         supportsImage,
			SupportsVideo:         supportsVideo,
		})
		app.Gateway.RegisterAgent(agentCfg.Name, ag)
		app.logger.Info("Agent已注册", "name", agentCfg.Name, "provider", agentCfg.Provider, "model", model, "workspace", agentWorkspaceDir)
	}

	// Skill 热加载
	if len(app.skillRegistries) > 0 {
		app.startSkillWatcher(globalSkillsDir)
	}
}

// startSkillWatcher 启动技能目录监控
func (app *App) startSkillWatcher(globalSkillsDir string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		app.logger.Error("创建 Skill watcher 失败", "err", err)
		return
	}

	watcher.Add(globalSkillsDir)
	app.logger.Info("Skill 热加载已启动", "global_dir", globalSkillsDir)

	for agentName, agentDir := range app.skillRegistryDirs {
		watcher.Add(agentDir)
		app.logger.Info("Skill 热加载监控 Agent 目录", "agent", agentName, "dir", agentDir)
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
					time.Sleep(500 * time.Millisecond)
					eventDir := filepath.Dir(event.Name)

					if eventDir == globalSkillsDir || filepath.Dir(eventDir) == globalSkillsDir {
						app.logger.Info("全局 Skill 目录变化，重载所有 Agent 技能", "path", event.Name)
						for agentName, reg := range app.skillRegistries {
							if err := reg.ReloadAll(); err != nil {
								app.logger.Warn("重载 Agent Skill 失败", "agent", agentName, "err", err)
							} else {
								app.logger.Info("Agent Skill 已重载", "agent", agentName, "count", len(reg.List()))
							}
						}
					} else {
						for agentName, agentDir := range app.skillRegistryDirs {
							if eventDir == agentDir || filepath.Dir(eventDir) == agentDir {
								reg := app.skillRegistries[agentName]
								if err := reg.ReloadAll(); err != nil {
									app.logger.Warn("重载 Agent Skill 失败", "agent", agentName, "err", err)
								} else {
									app.logger.Info("Agent Skill 已重载", "agent", agentName, "count", len(reg.List()))
								}
								break
							}
						}
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				app.logger.Error("Skill watcher 错误", "err", err)
			}
		}
	}()
}

// initDataDirs 初始化数据目录结构和人设文件
func initDataDirs(workspaceDir, sessionsDir, skillsDir string, logger *slog.Logger) {
	dirs := []string{workspaceDir, sessionsDir, skillsDir}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			logger.Error("创建目录失败", "dir", d, "err", err)
		}
	}

	personalityFiles := map[string]string{
		"AGENTS.md": `# AGENTS.md

## 安全规则

- 不要泄露敏感数据（API Key、密码、私钥等）
- 执行删除命令前先确认
- 外部操作（发送消息、调用外部 API）前先询问用户

## 工具使用

- 优先使用工具完成任务，不要仅描述打算做什么
- 工具调用失败时，分析原因并重试或换方案
- 复杂任务拆分成多步，逐步完成

## 沟通风格

- 简洁高效，避免冗余
- 重要信息用格式化输出（表格、列表）
`,
		"HEARTBEAT.md": `# HEARTBEAT.md

周期任务提示（可选）。
当启用 heartbeat 功能时，此文件内容会定期发送给 AI 执行。
`,
		"MEMORY.md": `# MEMORY.md

长期记忆存储。
记录需要长期记住的信息：项目配置、重要决策、经验教训。
可通过 memory 工具或直接编辑更新。
`,
		"PROFILE.md": `# PROFILE.md

## 身份

- 名称: AI 助手
- 类型: AI Agent
- 風格: 简洁、高效、可靠

## 用户

- 称呼: 用户
- 上下文: 通用助手场景

## 偏好

- 输出格式: 中文优先
- 回复风格: 简洁但完整
- 工具使用: 主动使用，不等待明确指令
`,
		"SOUL.md": `# SOUL.md

## 核心原则

**真正有用** - 不是表演式帮忙，而是真正解决问题
**有主见** - 可以表达观点，不只是迎合
**主动** - 能做的先做，不事事询问
**赢得信任** - 通过能力证明，不是空话

## 边界

- 隐私：不主动读取敏感文件，不泄露用户信息
- 安全：危险操作先确认，解释风险
- 效率：避免无意义的来回确认

## 态度

- 是助手，不是仆人 - 平等协作
- 是工具，不是玩具 - 认真对待每个请求
- 是伙伴，不是机器 - 有温度但不过度
`,
	}

	bootstrapCompletedPath := filepath.Join(workspaceDir, ".bootstrap_completed")
	bootstrapPath := filepath.Join(workspaceDir, "BOOTSTRAP.md")
	if _, err := os.Stat(bootstrapCompletedPath); os.IsNotExist(err) {
		if _, err := os.Stat(bootstrapPath); os.IsNotExist(err) {
			bootstrapContent := `# BOOTSTRAP.md

欢迎使用 go-claw！这是你的首次对话。

请帮我完成以下初始设置：

1. **你的身份偏好**: 你希望我叫你什么？我们之间的沟通语言是中文还是英文？
2. **我的服务重点**: 你最希望我帮你做什么？（如：编程助手、数据分析、信息查询等）
3. **沟通风格**: 你喜欢简洁直接的回答，还是详细解释？

我会根据你的回答更新 PROFILE.md，完成后此引导文件将自动标记为已完成。`
			if err := os.WriteFile(bootstrapPath, []byte(bootstrapContent), 0644); err != nil {
				logger.Warn("创建 BOOTSTRAP.md 失败", "err", err)
			} else {
				logger.Info("创建首次引导文件", "file", "BOOTSTRAP.md")
			}
		}
	}

	for name, content := range personalityFiles {
		filePath := filepath.Join(workspaceDir, name)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				logger.Warn("创建人设文件失败", "file", name, "err", err)
			} else {
				logger.Info("创建人设文件", "file", name)
			}
		}
	}

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