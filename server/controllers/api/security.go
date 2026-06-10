package api

import (
	"encoding/json"
	"net/http"
)

// HandleSecurityConfig 获取/更新安全配置（GET/PUT）
func HandleSecurityConfig(rw http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// 从配置服务获取完整配置
		configJSON := configSvc.GetJSON()
		var fullConfig map[string]interface{}
		if err := json.Unmarshal([]byte(configJSON), &fullConfig); err != nil {
			http.Error(rw, "Failed to parse config", http.StatusInternalServerError)
			return
		}

		// 提取 security 部分
		securityCfg, ok := fullConfig["security"]
		if !ok {
			securityCfg = map[string]interface{}{
				"enabled":             false,
				"deny_shell_inject":   false,
				"deny_sensitive_path": false,
				"guard_browser":       false,
				"allowed_paths":       []string{},
			}
		}

		rw.Header().Set("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(securityCfg)
		return
	}

	if r.Method == http.MethodPut {
		var securityCfg map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&securityCfg); err != nil {
			http.Error(rw, "Invalid request body", http.StatusBadRequest)
			return
		}

		// 获取完整配置
		configJSON := configSvc.GetJSON()
		var fullConfig map[string]interface{}
		if err := json.Unmarshal([]byte(configJSON), &fullConfig); err != nil {
			http.Error(rw, "Failed to parse config", http.StatusInternalServerError)
			return
		}

		// 更新 security 部分
		fullConfig["security"] = securityCfg

		// 保存完整配置
		if err := configSvc.Save(fullConfig); err != nil {
			http.Error(rw, "Failed to save config: "+err.Error(), http.StatusInternalServerError)
			return
		}

		rw.Header().Set("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(map[string]string{"status": "ok"})
		return
	}

	http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
}
