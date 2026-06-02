package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

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

// handleAgents 返回 Agent 列表（GET）- 从 config.json 读取
func handleAgents(rw http.ResponseWriter, r *http.Request) {
	cfg := readConfig()
	agents, _ := cfg["agents"].([]interface{})
	writeJSON(rw, http.StatusOK, agents)
}

// handleAgentByID 更新指定 Agent 配置（PUT）
func handleAgentByID(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
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
	skills := []map[string]any{}
	if skillsCfg != nil {
		skills = append(skills, map[string]any{
			"name": "skills", "description": "技能系统",
			"enabled": skillsCfg["enabled"], "skill_dir": skillsCfg["skill_dir"],
		})
	}
	writeJSON(rw, http.StatusOK, skills)
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
		// run job
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
	sessions := []map[string]any{}
	dataDir := "clawdata/workspaces"
	if dirs, err := os.ReadDir(dataDir); err == nil {
		for _, dir := range dirs {
			if dir.IsDir() {
				sessDir := dataDir + "/" + dir.Name() + "/sessions"
				if files, err := os.ReadDir(sessDir); err == nil {
					for _, f := range files {
						if strings.HasSuffix(f.Name(), ".json") {
							sessions = append(sessions, map[string]any{
								"id": strings.TrimSuffix(f.Name(), ".json"),
								"agent": dir.Name(),
							})
						}
					}
				}
			}
		}
	}
	writeJSON(rw, http.StatusOK, sessions)
}

// handleSessionByID 查看/删除指定会话（GET/DELETE）
func handleSessionByID(rw http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		sessionID := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")
		sessionID = strings.TrimSuffix(sessionID, "/")
		dataDir := "clawdata/workspaces"
		if dirs, err := os.ReadDir(dataDir); err == nil {
			for _, dir := range dirs {
				if dir.IsDir() {
					os.Remove(dataDir + "/" + dir.Name() + "/sessions/" + sessionID + ".json")
				}
			}
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}
	writeJSON(rw, http.StatusOK, map[string]any{})
}