package bootstrap

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go-claw/internal/acp"
	"go-claw/internal/agent"
	"go-claw/internal/cron"
	"go-claw/internal/gateway"
	"go-claw/internal/inbox"
	"go-claw/internal/mcp"
	"go-claw/internal/multiagent"
	"go-claw/internal/proactive"
	"go-claw/internal/security"
	"go-claw/internal/tool"
	glog "go-claw/pkg/log"
)

// initInbox 初始化 Inbox 系统
func (app *App) initInbox() {
	inbox.NewStore(app.DataDir)
	// 设置全局数据目录（用于工具的临时文件）
	tool.SetGlobalDataDir(app.DataDir)
	app.logger.Info("Inbox 系统已初始化")
}

// initMCP 初始化 MCP 集成
func (app *App) initMCP() {
	if !app.Config.MCP.Enabled {
		return
	}
	app.MCPMgr = mcp.NewManager()
	for _, serverCfg := range app.Config.MCP.Servers {
		app.MCPMgr.Register(mcp.ServerConfig{
			Name:    serverCfg.Name,
			Command: serverCfg.Command,
			URL:     serverCfg.URL,
			Args:    serverCfg.Args,
			Env:     serverCfg.Env,
			Enabled: serverCfg.Enabled,
		})
	}
	app.MCPMgr.ConnectAll(nil)
	app.logger.Info("MCP 集成已启动", "servers", len(app.Config.MCP.Servers))
}

// initACP 初始化 ACP 协议
func (app *App) initACP() {
	if !app.Config.ACP.Enabled {
		return
	}
	_ = acp.NewService()
	app.logger.Info("ACP 协议已初始化", "agents", len(app.Config.ACP.Agents))
}

// initCron 初始化 Cron 系统
func (app *App) initCron() {
	if !app.Config.Cron.Enabled {
		return
	}
	app.CronMgr = cron.NewManager(gatewayCronExecutor{gw: app.Gateway, agents: app.Gateway.GetAgents()}, filepath.Join(app.DataDir, "cron_jobs.json"))

	if len(app.CronMgr.ListJobs()) == 0 && len(app.Config.Cron.Jobs) > 0 {
		for i, job := range app.Config.Cron.Jobs {
			jobType := cron.JobTypeText
			if job.Type == "agent" {
				jobType = cron.JobTypeAgent
			}
			jobID := fmt.Sprintf("cron_%d_%s", i+1, job.Name)
			sessionID := job.SessionID
			if sessionID == "" {
				defaultChannel := app.Config.Cron.DefaultChannel
				if defaultChannel == "" {
					defaultChannel = "console"
				}
				defaultUser := app.Config.Cron.DefaultUser
				if defaultUser == "" {
					defaultUser = "cron"
				}
				sessionID = defaultChannel + ":" + defaultUser
			}
			content := job.Content
			if jobType == cron.JobTypeAgent && job.AgentPrompt != "" && content == "" {
				content = job.AgentPrompt
			}
			agentName := job.AgentName
			if jobType == cron.JobTypeAgent && agentName == "" {
				agentName = "default"
			}
			app.CronMgr.AddJob(&cron.Job{
				ID:          jobID,
				Name:        job.Name,
				Schedule:    job.Schedule,
				Type:        jobType,
				Content:     content,
				AgentName:   agentName,
				SessionID:   sessionID,
				ActiveStart: job.ActiveStart,
				ActiveEnd:   job.ActiveEnd,
				Enabled:     true,
			})
		}
	}
	app.CronMgr.Start()
	tool.SetGlobalCronManager(app.CronMgr)
	app.logger.Info("Cron 系统已启动", "jobs", len(app.CronMgr.ListJobs()))
}

// initSecurity 初始化工具安全守卫
func (app *App) initSecurity() {
	if !app.Config.Security.Enabled {
		return
	}
	toolGuard := security.NewToolGuard()
	if app.Config.Security.DenyShellInject {
		toolGuard.AddGuardian(security.NewShellEvasionGuardian())
	}
	if app.Config.Security.DenySensitivePath {
		toolGuard.AddGuardian(security.NewFileGuardian())
	}
	if app.Config.Security.GuardBrowser {
		toolGuard.AddGuardian(security.NewRuleGuardian())
	}
	app.logger.Info("工具安全守卫已启用",
		"shell_inject", app.Config.Security.DenyShellInject,
		"sensitive_path", app.Config.Security.DenySensitivePath,
		"browser", app.Config.Security.GuardBrowser)
}

// initMultiAgentTools 注册多 Agent 协作工具
func (app *App) initMultiAgentTools() {
	agentProcessors := make(map[string]multiagent.AgentProcessor)
	for name, ag := range app.Gateway.GetAgents() {
		agentProcessors[name] = ag
	}
	tool.GlobalRegistry.Register("chat_with_agent", func() tool.Tool {
		return multiagent.NewChatWithAgentTool(agentProcessors)
	})
	tool.GlobalRegistry.Register("submit_to_agent", func() tool.Tool {
		return multiagent.NewSubmitToAgentTool(agentProcessors)
	})
	tool.GlobalRegistry.Register("list_agents", func() tool.Tool {
		return multiagent.NewListAgentsTool(agentProcessors)
	})
	app.logger.Info("多 Agent 协作工具已注册")
}

// initProactive 启动主动模式
func (app *App) initProactive() {
	if !app.Config.Proactive.Enabled {
		return
	}
	idleMinutes := app.Config.Proactive.IdleMinutes
	if idleMinutes == 0 {
		idleMinutes = 30
	}
	agentName := app.Config.Proactive.AgentName
	if agentName == "" {
		agentName = "default"
	}
	app.ProactiveMgr = proactive.NewManager(proactive.Config{
		Enabled:     true,
		IdleMinutes: idleMinutes,
		AgentName:   agentName,
	}, app.Gateway.GetAgents(), nil, app.Gateway)
	app.ProactiveMgr.Start()
	app.logger.Info("主动模式已启动", "idle_minutes", idleMinutes, "agent", agentName)
}

// startSessionCleanup 启动会话清理协程
func (app *App) startSessionCleanup() {
	if app.Config.Gateway.SessionTTL <= 0 {
		return
	}
	app.logger.Info("会话清理已启用", "ttl_minutes", app.Config.Gateway.SessionTTL)
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			app.Gateway.CleanupExpiredSessions(app.Config.Gateway.SessionTTL)
			app.logger.Info("会话清理完成")
		}
	}()
}

// gatewayCronExecutor 实现 cron.Executor 接口
type gatewayCronExecutor struct {
	gw     *gateway.Gateway
	agents map[string]*agent.Agent
}

// ExecuteText 执行文本类型定时任务
func (e gatewayCronExecutor) ExecuteText(ctx context.Context, sessionID, content string) error {
	glog.Logger().Info("[Cron] 执行文本任务", "session_id", sessionID, "content", content)
	return e.gw.SendProactiveMessage(ctx, sessionID, content)
}

// ExecuteAgent 执行 Agent 类型定时任务
func (e gatewayCronExecutor) ExecuteAgent(ctx context.Context, agentName, sessionID, content string) (string, error) {
	ag, exists := e.agents[agentName]
	if !exists {
		return "", fmt.Errorf("Agent '%s' 不存在", agentName)
	}
	cronSessionID := "cron:" + agentName + "_" + time.Now().Format("20060102_150405")

	result, err := ag.Process(ctx, cronSessionID, content)
	if err != nil {
		return "", err
	}

	if sessionID != "" && !strings.HasPrefix(sessionID, "console:") {
		if sendErr := e.gw.SendProactiveMessage(ctx, sessionID, result); sendErr != nil {
			glog.Logger().Warn("[Cron] 发送 Agent 结果到渠道失败", "session_id", sessionID, "err", sendErr)
		} else {
			glog.Logger().Info("[Cron] Agent 结果已发送", "session_id", sessionID, "result_len", len(result))
		}
	}

	return result, nil
}