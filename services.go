package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go-claw/internal/agent"
	glog "go-claw/pkg/log"
)

// ─────────── Wails3 Services ───────────

// ChatService 对话服务，前端通过 Wails3 bridge 直接调用 Go 函数
type ChatService struct {
	mu     sync.Mutex
	agents map[string]*agent.Agent
}

// SetAgents 注入 Agent 实例
func (c *ChatService) SetAgents(agents map[string]*agent.Agent) {
	c.agents = agents
}

// SendMessage 对话接口，返回完整响应（Wails3 桌面模式）
func (c *ChatService) SendMessage(sessionID, content, agentName string) string {
	c.mu.Lock()
	ag := c.agents["default"]
	if agentName != "" {
		if a, ok := c.agents[agentName]; ok {
			ag = a
		}
	}
	c.mu.Unlock()
	if ag == nil {
		return "Agent 未初始化"
	}
	result, err := ag.Process(context.Background(), sessionID, content)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return result
}

// GetChatHistory 获取指定会话的历史消息
func (c *ChatService) GetChatHistory(sessionID, agentName string) string {
	c.mu.Lock()
	ag := c.agents["default"]
	if agentName != "" {
		if a, ok := c.agents[agentName]; ok {
			ag = a
		}
	}
	c.mu.Unlock()

	if ag == nil {
		return "[]"
	}

	// 先尝试从内存中的 SessionManager 获取
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
		data, _ := json.Marshal(result)
		return string(data)
	}

	// 从 Store 文件加载
	st := ag.GetStore()
	if st == nil {
		return "[]"
	}

	sessData, err := st.GetSession(context.Background(), sessionID)
	if err != nil || sessData == nil {
		return "[]"
	}

	result := make([]map[string]string, 0, len(sessData.Messages))
	for _, m := range sessData.Messages {
		if m.Role == "user" || m.Role == "assistant" {
			result = append(result, map[string]string{
				"role":    m.Role,
				"content": m.Content,
			})
		}
	}
	data, _ := json.Marshal(result)
	return string(data)
}

// AppService 管理服务
type AppService struct {
	agents  map[string]*agent.Agent
	sendMsg func(ctx context.Context, sessionID, message string) error
}

// SetAgents 注入 Agent 实例
func (a *AppService) SetAgents(agents map[string]*agent.Agent) {
	a.agents = agents
}

// SetSender 注入消息发送器（用于定时任务手动执行）
func (a *AppService) SetSender(sender func(ctx context.Context, sessionID, message string) error) {
	a.sendMsg = sender
}

func (a *AppService) GetConfig() string {
	data, _ := os.ReadFile("config.json")
	return string(data)
}

func (a *AppService) SaveConfig(configJSON string) string {
	os.WriteFile("config.json", []byte(configJSON), 0644)
	return `{"status":"saved"}`
}

// UpdateAgent 保存/更新 Agent 配置（写入 config.json）
func (a *AppService) UpdateAgent(name, agentJSON string) string {
	cfg := readConfigJSON()
	var agentData map[string]interface{}
	if err := json.Unmarshal([]byte(agentJSON), &agentData); err != nil {
		return `{"error":"invalid json"}`
	}
	agentData["name"] = name

	agents, _ := cfg["agents"].([]interface{})
	found := false
	for i, a := range agents {
		ag, _ := a.(map[string]interface{})
		if ag["name"] == name {
			agents[i] = agentData
			found = true
			break
		}
	}
	if !found {
		agents = append(agents, agentData)
	}
	cfg["agents"] = agents
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile("config.json", data, 0644)
	return `{"status":"updated"}`
}

// DeleteAgent 删除 Agent 配置
func (a *AppService) DeleteAgent(name string) string {
	if name == "default" {
		return `{"error":"default agent cannot be deleted"}`
	}
	cfg := readConfigJSON()
	agents, _ := cfg["agents"].([]interface{})
	filtered := make([]interface{}, 0, len(agents))
	for _, a := range agents {
		ag, _ := a.(map[string]interface{})
		if ag["name"] != name {
			filtered = append(filtered, a)
		}
	}
	cfg["agents"] = filtered
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile("config.json", data, 0644)
	return `{"status":"deleted"}`
}

// UpdateChannel 更新渠道配置
func (a *AppService) UpdateChannel(name, configJSON string) string {
	cfg := readConfigJSON()
	var chCfg map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &chCfg); err != nil {
		return `{"error":"invalid json"}`
	}
	cfg["channels"].(map[string]interface{})[name] = chCfg
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile("config.json", data, 0644)
	return `{"status":"updated"}`
}

// GetProviders 获取供应商/模型列表
func (a *AppService) GetProviders() string {
	cfg := readConfigJSON()
	providers := cfg["providers"]
	data, _ := json.Marshal(providers)
	return string(data)
}

// GetTools 获取工具列表
func (a *AppService) GetTools() string {
	// 返回已注册的工具列表
	result := []map[string]string{}
	toolNames := []string{"weather", "exec", "write_file", "read_file", "edit_file", "append_file",
		"send_file", "browser_use", "get_current_time", "set_user_timezone", "cron_status",
		"system_info", "network_check", "http_request", "web_search", "calculate", "run_code",
		"list_files", "read_pdf", "ocr_image", "generate_image", "database_query", "manage_config"}
	for _, t := range toolNames {
		result = append(result, map[string]string{"name": t})
	}
	data, _ := json.Marshal(result)
	return string(data)
}

// GetSkills 获取 Skills 列表
func (a *AppService) GetSkills() string {
	skills := []map[string]any{}
	skillDir := "skills"

	// 扫描全局 skills 目录
	if entries, err := os.ReadDir(skillDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				skillFile := filepath.Join(skillDir, e.Name(), "SKILL.md")
				if data, err := os.ReadFile(skillFile); err == nil {
					skill := parseSkillMDData(data, skillDir, e.Name())
					if skill != nil {
						skills = append(skills, skill)
					}
				}
			}
		}
	}

	// 扫描每个 agent 的 skills 目录
	if agentDirs, err := os.ReadDir("clawdata/workspaces"); err == nil {
		for _, ad := range agentDirs {
			if ad.IsDir() {
				dir := filepath.Join("clawdata/workspaces", ad.Name(), "skills")
				if entries, err := os.ReadDir(dir); err == nil {
					for _, e := range entries {
						if e.IsDir() {
							skillFile := filepath.Join(dir, e.Name(), "SKILL.md")
							if data, err := os.ReadFile(skillFile); err == nil {
								skill := parseSkillMDData(data, dir, e.Name())
								if skill != nil {
									skills = append(skills, skill)
								}
							}
						}
					}
				}
			}
		}
	}

	result := map[string]any{"skill_dir": skillDir, "skills": skills, "total": len(skills)}
	data, _ := json.Marshal(result)
	return string(data)
}

func parseSkillMDData(data []byte, dir, folder string) map[string]any {
	content := string(data)
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return nil
	}
	yamlContent := strings.TrimSpace(parts[1])
	markdownBody := strings.TrimSpace(parts[2])

	skill := map[string]any{
		"folder": folder, "path": filepath.Join(dir, folder),
		"markdown": markdownBody, "has_scripts": false,
	}
	for _, line := range strings.Split(yamlContent, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			skill["name"] = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		} else if strings.HasPrefix(line, "description:") {
			skill["description"] = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		} else if strings.Contains(line, "emoji:") {
			skill["emoji"] = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "emoji:"))
		}
	}
	scriptsDir := filepath.Join(dir, folder, "scripts")
	if info, err := os.Stat(scriptsDir); err == nil && info.IsDir() {
		if files, _ := os.ReadDir(scriptsDir); len(files) > 0 {
			skill["has_scripts"] = true
			scripts := []string{}
			for _, f := range files {
				if !f.IsDir() {
					scripts = append(scripts, f.Name())
				}
			}
			skill["scripts"] = scripts
		}
	}
	sections := []string{}
	for _, line := range strings.Split(markdownBody, "\n") {
		if strings.HasPrefix(line, "## ") {
			sections = append(sections, strings.TrimPrefix(line, "## "))
		}
	}
	skill["sections"] = sections
	return skill
}

// GetLogs 返回最新日志内容（最多 50KB）
func (a *AppService) GetLogs() string {
	data, err := os.ReadFile("logs/app.log")
	if err != nil {
		return "暂无日志"
	}
	if len(data) > 50000 {
		data = data[len(data)-50000:]
	}
	return string(data)
}

// GetStatus 返回系统运行状态
func (a *AppService) GetStatus() string {
	status := map[string]string{
		"status": "running",
	}
	data, _ := json.Marshal(status)
	return string(data)
}

// GetAgents 返回 Agent 列表（从 config.json 读取）
func (a *AppService) GetAgents() string {
	data, err := os.ReadFile("config.json")
	if err != nil {
		return "[]"
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "[]"
	}
	agents, _ := cfg["agents"].([]interface{})
	result, _ := json.Marshal(agents)
	return string(result)
}

// GetSessions 返回会话列表（扫描文件目录）
func (a *AppService) GetSessions() string {
	sessions := []map[string]string{}
	dataDir := "clawdata/workspaces"
	if dirs, err := os.ReadDir(dataDir); err == nil {
		for _, dir := range dirs {
			if dir.IsDir() {
				sessDir := dataDir + "/" + dir.Name() + "/sessions"
				if files, err := os.ReadDir(sessDir); err == nil {
					for _, f := range files {
						if strings.HasSuffix(f.Name(), ".json") && f.Name() != "memories.json" {
							sessionID := strings.TrimSuffix(f.Name(), ".json")
							sessions = append(sessions, map[string]string{
								"id":    sessionID,
								"agent": dir.Name(),
							})
						}
					}
				}
			}
		}
	}
	result, _ := json.Marshal(sessions)
	return string(result)
}

// DeleteSession 删除指定会话文件
func (a *AppService) DeleteSession(sessionID string) string {
	dataDir := "clawdata/workspaces"
	if dirs, err := os.ReadDir(dataDir); err == nil {
		for _, dir := range dirs {
			if dir.IsDir() {
				filePath := dataDir + "/" + dir.Name() + "/sessions/" + sessionID + ".json"
				os.Remove(filePath)
			}
		}
	}
	return `{"status":"deleted"}`
}

// GetCronJobs 获取定时任务列表（从 clawdata/cron_jobs.json 读取）
func (a *AppService) GetCronJobs() string {
	dataFile := "clawdata/cron_jobs.json"
	data, err := os.ReadFile(dataFile)
	if err != nil {
		return "[]"
	}
	var jobs []map[string]interface{}
	if err := json.Unmarshal(data, &jobs); err != nil {
		return "[]"
	}
	// 返回时清理 last_run 等不需要的字段，保持简洁
	result := make([]map[string]interface{}, 0, len(jobs))
	for _, j := range jobs {
		result = append(result, j)
	}
	res, _ := json.Marshal(result)
	return string(res)
}

// SaveCronJob 保存单个定时任务（追加或更新到 cron_jobs.json）
func (a *AppService) SaveCronJob(jobJSON string) string {
	dataFile := "clawdata/cron_jobs.json"
	var newJob map[string]interface{}
	if err := json.Unmarshal([]byte(jobJSON), &newJob); err != nil {
		return `{"error":"invalid json"}`
	}

	var jobs []map[string]interface{}
	data, err := os.ReadFile(dataFile)
	if err == nil {
		json.Unmarshal(data, &jobs)
	}

	// 如果任务有 id，尝试更新；否则追加
	if jobID, ok := newJob["id"].(string); ok && jobID != "" {
		for i, j := range jobs {
			if j["id"] == jobID {
				jobs[i] = newJob
				data, _ = json.MarshalIndent(jobs, "", "  ")
				os.WriteFile(dataFile, data, 0644)
				return `{"status":"updated"}`
			}
		}
	}

	// 新任务
	jobs = append(jobs, newJob)
	data, _ = json.MarshalIndent(jobs, "", "  ")
	os.WriteFile(dataFile, data, 0644)
	return `{"status":"created"}`
}

// DeleteCronJob 删除定时任务
func (a *AppService) DeleteCronJob(id string) string {
	dataFile := "clawdata/cron_jobs.json"
	data, err := os.ReadFile(dataFile)
	if err != nil {
		return `{"status":"deleted"}`
	}
	var jobs []map[string]interface{}
	if err := json.Unmarshal(data, &jobs); err != nil {
		return `{"status":"deleted"}`
	}
	filtered := make([]map[string]interface{}, 0, len(jobs))
	for _, j := range jobs {
		if j["id"] != id {
			filtered = append(filtered, j)
		}
	}
	data, _ = json.MarshalIndent(filtered, "", "  ")
	os.WriteFile(dataFile, data, 0644)
	return `{"status":"deleted"}`
}

// RunCronJob 异步立即执行定时任务
func (a *AppService) RunCronJob(id string) string {
	// 从文件读取任务配置
	dataFile := "clawdata/cron_jobs.json"
	data, err := os.ReadFile(dataFile)
	if err != nil {
		return `{"error":"无法读取任务列表"}`
	}
	var jobs []map[string]interface{}
	if err := json.Unmarshal(data, &jobs); err != nil {
		return `{"error":"任务列表解析失败"}`
	}

	// 查找任务
	var job map[string]interface{}
	for _, j := range jobs {
		if j["id"] == id {
			job = j
			break
		}
	}
	if job == nil {
		return `{"error":"任务不存在"}`
	}

	name, _ := job["name"].(string)
	jobType, _ := job["type"].(string)
	content, _ := job["content"].(string)
	sessionID, _ := job["session_id"].(string)
	agentName, _ := job["agent_name"].(string)

	// 异步执行
	go func() {
		logger := glog.Logger()
		logger.Info("[Cron] 手动执行任务", "id", id, "name", name, "type", jobType)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		switch jobType {
		case "text":
			// 文本消息直接发送到渠道
			if a.sendMsg != nil {
				if err := a.sendMsg(ctx, sessionID, content); err != nil {
					logger.Warn("[Cron] 文本任务发送失败", "id", id, "err", err)
				} else {
					logger.Info("[Cron] 文本任务已发送", "id", id, "session", sessionID)
				}
			} else {
				logger.Warn("[Cron] 消息发送器未注入")
			}

		case "agent":
			ag := a.agents["default"]
			if agentName != "" {
				if ag2, ok := a.agents[agentName]; ok {
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
			if a.sendMsg != nil {
				if err := a.sendMsg(ctx, sessionID, result); err != nil {
					logger.Warn("[Cron] Agent 结果发送失败", "id", id, "session", sessionID, "err", err)
				} else {
					logger.Info("[Cron] Agent 结果已发送", "id", id, "session", sessionID)
				}
			}

		default:
			logger.Warn("[Cron] 未知任务类型", "type", jobType)
		}
	}()

	return `{"status":"executed"}`
}

// GetCronEnabled 获取定时任务启用状态
func (a *AppService) GetCronEnabled() string {
	cfg := readConfigJSON()
	cronCfg, _ := cfg["cron"].(map[string]interface{})
	enabled := true
	if v, ok := cronCfg["enabled"]; ok {
		enabled = v == true
	}
	data, _ := json.Marshal(map[string]bool{"enabled": enabled})
	return string(data)
}

// SetCronEnabled 设置定时任务启用状态
func (a *AppService) SetCronEnabled(enabled string) string {
	data, _ := os.ReadFile("config.json")
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return `{"status":"ok"}`
	}
	if cfg["cron"] == nil {
		cfg["cron"] = map[string]interface{}{}
	}
	cronCfg := cfg["cron"].(map[string]interface{})
	cronCfg["enabled"] = enabled == "true"
	data, _ = json.MarshalIndent(cfg, "", "  ")
	os.WriteFile("config.json", data, 0644)
	return `{"status":"ok"}`
}

// readConfigJSON 辅助函数：读取并解析 config.json
func readConfigJSON() map[string]interface{} {
	data, err := os.ReadFile("config.json")
	if err != nil {
		return map[string]interface{}{}
	}
	var cfg map[string]interface{}
	json.Unmarshal(data, &cfg)
	return cfg
}