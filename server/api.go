package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-claw/internal/store"
)

// SetCronExecutor 设置定时任务执行回调（由 main 注入）
var cronExecutor func(id string)

func SetCronExecutor(fn func(id string)) {
	cronExecutor = fn
}

// executeCronJobByID 异步立即执行指定定时任务
func executeCronJobByID(id string) {
	if cronExecutor == nil {
		return
	}
	cronExecutor(id)
}

// handleConfig 读取/保存 config.json（GET/PUT）
func handleConfig(rw http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data, err := os.ReadFile("config.json")
		if err != nil {
			writeError(rw, http.StatusInternalServerError, "读取配置失败")
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.Write(data)
		return
	}
	if r.Method == http.MethodPut {
		var config map[string]any
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeError(rw, http.StatusBadRequest, "无效的JSON")
			return
		}
		data, _ := json.MarshalIndent(config, "", "  ")
		if err := os.WriteFile("config.json", data, 0644); err != nil {
			writeError(rw, http.StatusInternalServerError, "保存失败")
			return
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
}

// handleConfigReload 触发配置热重载（POST）
func handleConfigReload(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(rw, http.StatusOK, map[string]string{"status": "reload triggered"})
}

// readConfig 读取 config.json
func readConfig() map[string]any {
	data, err := os.ReadFile("config.json")
	if err != nil {
		return map[string]any{}
	}
	var cfg map[string]any
	json.Unmarshal(data, &cfg)
	return cfg
}

func writeConfig(cfg map[string]any) {
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile("config.json", data, 0644)
}

// handleAgents 返回 Agent 列表（GET）- 从 config.json 读取
func handleAgents(rw http.ResponseWriter, r *http.Request) {
	cfg := readConfig()
	agents, _ := cfg["agents"].([]interface{})
	writeJSON(rw, http.StatusOK, agents)
}

// handleAgentByID 更新/删除指定 Agent 配置（PUT/DELETE）
func handleAgentByID(rw http.ResponseWriter, r *http.Request) {
	// 解析 URL: /api/v1/agents/{name}
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		writeError(rw, http.StatusBadRequest, "agent name required")
		return
	}

	if r.Method == http.MethodDelete {
		// 内置 default agent 不允许删除
		if name == "default" {
			writeError(rw, http.StatusForbidden, "default agent cannot be deleted")
			return
		}
		cfg := readConfig()
		agents, _ := cfg["agents"].([]interface{})
		filtered := make([]interface{}, 0, len(agents))
		for _, a := range agents {
			ag, _ := a.(map[string]interface{})
			if ag["name"] != name {
				filtered = append(filtered, a)
			}
		}
		cfg["agents"] = filtered
		writeConfig(cfg)
		writeJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}

	if r.Method != http.MethodPut {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 读取请求体
	var updateData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		writeError(rw, http.StatusBadRequest, "invalid JSON")
		return
	}

	// 确保 name 字段不被覆盖
	updateData["name"] = name

	// 读取 config.json
	cfg := readConfig()
	agents, _ := cfg["agents"].([]interface{})

	// 查找并更新
	found := false
	for i, a := range agents {
		ag, _ := a.(map[string]interface{})
		if ag["name"] == name {
			agents[i] = updateData
			found = true
			break
		}
	}

	// 不存在则追加（新增 Agent）
	if !found {
		agents = append(agents, updateData)
	}

	cfg["agents"] = agents
	writeConfig(cfg)

	writeJSON(rw, http.StatusOK, updateData)
}

// handleChannels 返回渠道列表及连接状态（GET）- 从 config.json 读取
func handleChannels(rw http.ResponseWriter, r *http.Request) {
	cfg := readConfig()
	channelsCfg, _ := cfg["channels"].(map[string]interface{})

	channels := []map[string]any{}
	channelTypes := map[string]string{
		"console": "console", "webhook": "http", "websocket": "ws",
		"lark": "lark", "dingtalk": "dingtalk", "wecom": "wecom", "wechat": "wechat",
	}

	for name, chCfg := range channelsCfg {
		ch, _ := chCfg.(map[string]interface{})
		enabled, _ := ch["enabled"].(bool)
		status := "disconnected"
		if enabled {
			status = "connected"
		}
		channels = append(channels, map[string]any{
			"name": name, "type": channelTypes[name],
			"enabled": enabled, "status": status, "config": ch,
		})
	}
	writeJSON(rw, http.StatusOK, channels)
}

// handleChannelByID 更新指定渠道配置（PUT）
func handleChannelByID(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
}

// handleProviders 返回 LLM 供应商列表（GET）- 从 config.json 读取
func handleProviders(rw http.ResponseWriter, r *http.Request) {
	cfg := readConfig()
	providers, _ := cfg["providers"].(map[string]interface{})
	result := []map[string]any{}
	for name, pCfg := range providers {
		p, _ := pCfg.(map[string]interface{})
		models, _ := p["models"].([]interface{})
		result = append(result, map[string]any{
			"name": name, "type": p["type"],
			"base_url": p["base_url"], "api_key": maskAPIKey(p["api_key"]),
			"models": models,
		})
	}
	writeJSON(rw, http.StatusOK, result)
}

// maskAPIKey 部分隐藏 API Key
func maskAPIKey(key interface{}) string {
	if key == nil {
		return ""
	}
	s, _ := key.(string)
	if len(s) < 8 {
		return s
	}
	return s[:4] + "..." + s[len(s)-4:]
}

// handleTools 返回工具列表（GET）
func handleTools(rw http.ResponseWriter, r *http.Request) {
	cfg := readConfig()
	agents, _ := cfg["agents"].([]interface{})
	toolsMap := map[string]bool{}
	for _, ag := range agents {
		a, _ := ag.(map[string]interface{})
		ts, _ := a["tools"].([]interface{})
		for _, t := range ts {
			toolName, _ := t.(string)
			toolsMap[toolName] = true
		}
	}
	tools := []map[string]any{}
	toolDescs := map[string]string{
		"weather": "天气查询工具", "exec": "执行Shell命令",
		"write_file": "写入文件", "read_file": "读取文件",
		"edit_file": "编辑文件", "append_file": "追加文件内容",
		"send_file": "发送文件", "browser_use": "浏览器自动化",
		"get_current_time": "获取当前时间", "set_user_timezone": "设置时区",
		"cron_status": "定时任务状态",
	}
	for name := range toolsMap {
		tools = append(tools, map[string]any{
			"name": name, "description": toolDescs[name], "skill_group": "builtin",
		})
	}
	writeJSON(rw, http.StatusOK, tools)
}

// handleSkills 返回 Skill 技能列表（GET）
func handleSkills(rw http.ResponseWriter, r *http.Request) {
	cfg := readConfig()
	skillsCfg, _ := cfg["skills"].(map[string]interface{})

	skillDir := "skills"
	if skillsCfg != nil {
		if v, ok := skillsCfg["skill_dir"].(string); ok {
			skillDir = v
		}
	}

	// 从 skill 目录扫描并解析所有 SKILL.md
	skills := scanSkills(skillDir)

	// 也扫描每个 agent 的 skills 目录
	agentDirs, _ := os.ReadDir("clawdata/workspaces")
	for _, ad := range agentDirs {
		if ad.IsDir() {
			agentSkillDir := filepath.Join("clawdata/workspaces", ad.Name(), "skills")
			if info, err := os.Stat(agentSkillDir); err == nil && info.IsDir() {
				skills = append(skills, scanSkills(agentSkillDir)...)
			}
		}
	}

	result := map[string]any{
		"skill_dir": skillDir,
		"skills":    skills,
		"total":     len(skills),
	}
	writeJSON(rw, http.StatusOK, result)
}

// scanSkills 扫描目录下所有 SKILL.md 并解析
func scanSkills(dir string) []map[string]any {
	var skills []map[string]any
	entries, err := os.ReadDir(dir)
	if err != nil {
		return skills
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(dir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}
		skill := parseSkillMD(data, dir, entry.Name())
		if skill != nil {
			skills = append(skills, skill)
		}
	}
	return skills
}

// parseSkillMD 解析 SKILL.md 文件内容
func parseSkillMD(data []byte, dir, folder string) map[string]any {
	content := string(data)
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return nil
	}

	// 简单解析 YAML frontmatter
	yamlContent := strings.TrimSpace(parts[1])
	markdownBody := strings.TrimSpace(parts[2])

	skill := map[string]any{
		"folder":      folder,
		"path":        filepath.Join(dir, folder),
		"markdown":    markdownBody,
		"has_scripts": false,
	}

	// 解析 YAML 字段（简单按行解析）
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

	// 检查是否有 scripts 目录
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

	// 解析 markdown 章节标题
	sections := []string{}
	for _, line := range strings.Split(markdownBody, "\n") {
		if strings.HasPrefix(line, "## ") {
			sections = append(sections, strings.TrimPrefix(line, "## "))
		}
	}
	skill["sections"] = sections

	return skill
}

// handleCronJobs 定时任务列表（GET）/ 添加任务（POST）
func handleCronJobs(rw http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile("clawdata/cron_jobs.json")
		if err != nil {
			writeJSON(rw, http.StatusOK, []interface{}{})
			return
		}
		var jobs []interface{}
		json.Unmarshal(data, &jobs)
		writeJSON(rw, http.StatusOK, jobs)
	case http.MethodPost:
		var newJob map[string]interface{}
		json.NewDecoder(r.Body).Decode(&newJob)
		dataFile := "clawdata/cron_jobs.json"
		data, _ := os.ReadFile(dataFile)
		var jobs []map[string]interface{}
		json.Unmarshal(data, &jobs)
		jobs = append(jobs, newJob)
		data, _ = json.MarshalIndent(jobs, "", "  ")
		os.WriteFile(dataFile, data, 0644)
		writeJSON(rw, http.StatusOK, map[string]string{"status": "created"})
	}
}

// handleCronJobByID 更新/删除/立即执行定时任务（PUT/DELETE/POST）
func handleCronJobByID(rw http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/cron/jobs/"), "/")
	id := parts[0]

	switch r.Method {
	case http.MethodPost:
		// 异步立即执行定时任务
		go executeCronJobByID(id)
		writeJSON(rw, http.StatusOK, map[string]string{"status": "executed"})
	case http.MethodDelete:
		dataFile := "clawdata/cron_jobs.json"
		data, err := os.ReadFile(dataFile)
		if err == nil {
			var jobs []map[string]interface{}
			json.Unmarshal(data, &jobs)
			filtered := make([]map[string]interface{}, 0, len(jobs))
			for _, j := range jobs {
				if j["id"] != id {
					filtered = append(filtered, j)
				}
			}
			data, _ = json.MarshalIndent(filtered, "", "  ")
			os.WriteFile(dataFile, data, 0644)
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
	case http.MethodPut:
		var updatedJob map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updatedJob)
		dataFile := "clawdata/cron_jobs.json"
		data, _ := os.ReadFile(dataFile)
		var jobs []map[string]interface{}
		json.Unmarshal(data, &jobs)
		for i, j := range jobs {
			if j["id"] == id {
				jobs[i] = updatedJob
				break
			}
		}
		data, _ = json.MarshalIndent(jobs, "", "  ")
		os.WriteFile(dataFile, data, 0644)
		writeJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
	default:
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleLogs 返回最新日志内容（GET，最多 50KB）
func handleLogs(rw http.ResponseWriter, r *http.Request) {
	data, _ := os.ReadFile("logs/app.log")
	if len(data) > 50000 {
		data = data[len(data)-50000:]
	}
	rw.Header().Set("Content-Type", "text/plain")
	rw.Write(data)
}

// handleStatus 返回系统运行状态（GET）
func handleStatus(rw http.ResponseWriter, r *http.Request) {
	writeJSON(rw, http.StatusOK, map[string]any{
		"status": "running", "uptime": startTime.Format(time.RFC3339),
	})
}

// handleSessions 返回会话列表（GET）
func handleSessions(rw http.ResponseWriter, r *http.Request) {
	sessions := []map[string]string{}
	dataDir := "clawdata/workspaces"
	if dirs, err := os.ReadDir(dataDir); err == nil {
		for _, dir := range dirs {
			if dir.IsDir() {
				sessDir := dataDir + "/" + dir.Name() + "/sessions"
				if files, err := os.ReadDir(sessDir); err == nil {
					for _, f := range files {
						// 跳过 memories 文件和目录
						if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") || strings.HasPrefix(f.Name(), "memories") {
							continue
						}
						filePath := filepath.Join(sessDir, f.Name())
						data, err := os.ReadFile(filePath)
						if err != nil {
							continue
						}
						var sessData map[string]interface{}
						if err := json.Unmarshal(data, &sessData); err != nil {
							continue
						}
						// 会话文件必须包含 messages 字段
						if _, hasMessages := sessData["messages"]; !hasMessages {
							continue
						}
						sessionID, _ := sessData["session_id"].(string)
						if sessionID == "" {
							sessionID, _ = sessData["id"].(string)
						}
						if sessionID == "" {
							continue
						}
						userID, _ := sessData["user_id"].(string)
						if userID == "" {
							parts := strings.SplitN(sessionID, ":", 2)
							if len(parts) == 2 {
								userID = parts[1]
							}
						}
						name, _ := sessData["name"].(string)
						channel, _ := sessData["channel"].(string)
						createdAt, _ := sessData["created_at"].(string)
						updatedAt, _ := sessData["updated_at"].(string)
						sessions = append(sessions, map[string]string{
							"id":         sessionID,
							"session_id": sessionID,
							"name":       name,
							"user_id":    userID,
							"agent":      dir.Name(),
							"channel":    channel,
							"created_at": createdAt,
							"updated_at": updatedAt,
						})
					}
				}
			}
		}
	}
	writeJSON(rw, http.StatusOK, sessions)
}

// handleSessionByID 查看/删除指定会话（GET/DELETE）
func handleSessionByID(rw http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")
	sessionID = strings.TrimSuffix(sessionID, "/")
	if sessionID == "" {
		writeJSON(rw, http.StatusBadRequest, map[string]string{"error": "session id required"})
		return
	}

	if r.Method == http.MethodDelete {
		dataDir := "clawdata/workspaces"
		safeName := store.SafeFileName(sessionID) + ".json"
		if dirs, err := os.ReadDir(dataDir); err == nil {
			for _, dir := range dirs {
				if dir.IsDir() {
					filePath := filepath.Join(dataDir, dir.Name(), "sessions", safeName)
					os.Remove(filePath)
				}
			}
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}

	// GET: 返回会话详情
	dataDir := "clawdata/workspaces"
	if dirs, err := os.ReadDir(dataDir); err == nil {
		for _, dir := range dirs {
			if dir.IsDir() {
				safeName := store.SafeFileName(sessionID) + ".json"
				filePath := filepath.Join(dataDir, dir.Name(), "sessions", safeName)
				if data, err := os.ReadFile(filePath); err == nil {
					var sessData map[string]interface{}
					if json.Unmarshal(data, &sessData) == nil {
						writeJSON(rw, http.StatusOK, sessData)
						return
					}
				}
			}
		}
	}
	writeJSON(rw, http.StatusNotFound, map[string]string{"error": "session not found"})
}

// ─────────── Agent 文件管理 ───────────

// handleAgentFiles 列出/读写 Agent 工作空间文件
func handleAgentFiles(rw http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agent-files/")
	parts := strings.SplitN(path, "/", 2)
	agentName := parts[0]
	fileName := ""
	if len(parts) > 1 {
		fileName = parts[1]
	}

	agentDir := filepath.Join("clawdata", "workspaces", agentName)
	if _, err := os.Stat(agentDir); os.IsNotExist(err) {
		writeError(rw, http.StatusNotFound, "agent workspace not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if fileName == "" {
			files := []map[string]any{}
			entries, err := os.ReadDir(agentDir)
			if err != nil {
				writeError(rw, http.StatusInternalServerError, "read dir failed")
				return
			}
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				info, _ := e.Info()
				files = append(files, map[string]any{
					"name": e.Name(),
					"size": info.Size(),
				})
			}
			writeJSON(rw, http.StatusOK, files)
		} else {
			filePath := filepath.Join(agentDir, fileName)
			data, err := os.ReadFile(filePath)
			if err != nil {
				writeError(rw, http.StatusNotFound, "file not found")
				return
			}
			rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
			rw.Write(data)
		}

	case http.MethodPut:
		if fileName == "" {
			writeError(rw, http.StatusBadRequest, "filename required")
			return
		}
		filePath := filepath.Join(agentDir, fileName)
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		content := body["content"]
		os.WriteFile(filePath, []byte(content), 0644)
		writeJSON(rw, http.StatusOK, map[string]string{"status": "saved"})

	default:
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
	}
}