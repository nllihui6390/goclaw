package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// HandleSkillPool 返回全量技能池（GET）
func HandleSkillPool(rw http.ResponseWriter, r *http.Request) {
	result := skillSvc.PoolJSON()
	rw.Header().Set("Content-Type", "application/json")
	rw.Write([]byte(result))
}

// HandleSkillScan 扫描技能目录并更新技能池（POST）
func HandleSkillScan(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	reg, err := skillSvc.Scan()
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	result := map[string]interface{}{
		"skill_dir": skillSvc.GetSkillDir(),
		"skills":    reg.Skills,
		"total":     len(reg.Skills),
		"message":   "扫描完成",
	}
	writeJSON(rw, http.StatusOK, result)
}

// HandleSkillEnabled 获取或设置指定 agent 的启用技能（GET/PUT）
func HandleSkillEnabled(rw http.ResponseWriter, r *http.Request) {
	agent := r.URL.Query().Get("agent")
	if agent == "" {
		writeError(rw, http.StatusBadRequest, "agent parameter required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		result := skillSvc.GetEnabledSkillsJSON(agent)
		rw.Header().Set("Content-Type", "application/json")
		rw.Write([]byte(result))

	case http.MethodPut:
		var skills []string
		if err := json.NewDecoder(r.Body).Decode(&skills); err != nil {
			writeError(rw, http.StatusBadRequest, "invalid JSON: expected array of skill names")
			return
		}
		if err := skillSvc.SetEnabledSkills(agent, skills); err != nil {
			writeError(rw, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(rw, http.StatusOK, map[string]string{"status": "updated"})

	default:
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleSkillUpload 上传技能 zip 文件（POST multipart/form-data）
func HandleSkillUpload(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 解析 multipart form (最大 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		writeError(rw, http.StatusBadRequest, "无法解析上传文件: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(rw, http.StatusBadRequest, "缺少文件字段")
		return
	}
	defer file.Close()

	// 验证文件类型
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
		writeError(rw, http.StatusBadRequest, "只支持 .zip 文件")
		return
	}

	// 读取文件内容到内存
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, "读取文件失败")
		return
	}

	// 调用服务层导入 zip
	result, err := skillSvc.ImportFromZip(data)
	if err != nil {
		if strings.Contains(err.Error(), "已存在") {
			writeJSON(rw, http.StatusConflict, map[string]string{"error": err.Error()})
		} else {
			writeError(rw, http.StatusBadRequest, err.Error())
		}
		return
	}

	writeJSON(rw, http.StatusOK, result)
}

// extractZipSkills 从 zip 数据中提取技能
// 返回临时目录路径和发现的技能名称列表
func extractZipSkills(data []byte, destDir string) ([]string, error) {
	// 创建 zip reader
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}

	// 解压到目标目录
	var skillNames []string
	for _, f := range reader.File {
		info := f.FileInfo()
		// 跳过目录和特殊文件
		if info.IsDir() || strings.HasPrefix(f.Name, "__MACOSX") || strings.Contains(f.Name, ".DS_Store") {
			continue
		}

		// 安全检查：防止路径遍历
		targetPath := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destDir)) {
			continue
		}

		// 创建目录结构
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			continue
		}

		// 解压文件
		rc, err := f.Open()
		if err != nil {
			continue
		}
		dst, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			rc.Close()
			continue
		}
		io.Copy(dst, rc)
		dst.Close()
		rc.Close()
	}

	// 扫描解压后的目录，寻找包含 SKILL.md 的技能目录
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(destDir, entry.Name())
		skillFile := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillFile); err == nil {
			skillNames = append(skillNames, entry.Name())
		}
	}

	return skillNames, nil
}