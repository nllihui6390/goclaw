package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// SkillInfo 技能信息
type SkillInfo struct {
	Folder     string   `json:"folder"`
	Path       string   `json:"path"`
	Name       string   `json:"name"`
	Description string  `json:"description"`
	Emoji      string   `json:"emoji,omitempty"`
	Markdown   string   `json:"markdown"`
	HasScripts bool     `json:"has_scripts"`
	Scripts    []string `json:"scripts,omitempty"`
	Sections   []string `json:"sections"`
}

// SkillService 技能管理服务
type SkillService struct {
	mu     sync.RWMutex
	config *ConfigService
}

// NewSkillService 创建技能服务
func NewSkillService(config *ConfigService) *SkillService {
	return &SkillService{config: config}
}

// GetSkillDir 获取技能目录配置
func (s *SkillService) GetSkillDir() string {
	cfg := s.config.Get()
	skillsCfg, _ := cfg["skills"].(map[string]interface{})
	if skillsCfg != nil {
		if v, ok := skillsCfg["skill_dir"].(string); ok && v != "" {
			return v
		}
	}
	return "skills"
}

// List 获取技能列表
func (s *SkillService) List() []SkillInfo {
	skills := []SkillInfo{}
	skillDir := s.GetSkillDir()

	// 扫描全局 skills 目录
	skills = append(skills, s.scanSkills(skillDir)...)

	// 扫描每个 agent 的 skills 目录
	wsBase := s.config.WorkspaceBase()
	agentDirs, _ := os.ReadDir(wsBase)
	for _, ad := range agentDirs {
		if ad.IsDir() {
			agentSkillDir := filepath.Join(wsBase, ad.Name(), "skills")
			if info, err := os.Stat(agentSkillDir); err == nil && info.IsDir() {
				skills = append(skills, s.scanSkills(agentSkillDir)...)
			}
		}
	}

	return skills
}

// scanSkills 扫描目录下所有 SKILL.md 并解析
func (s *SkillService) scanSkills(dir string) []SkillInfo {
	var skills []SkillInfo
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

		skill := s.parseSkillMD(data, dir, entry.Name())
		if skill != nil {
			skills = append(skills, *skill)
		}
	}

	return skills
}

// parseSkillMD 解析 SKILL.md 文件内容
func (s *SkillService) parseSkillMD(data []byte, dir, folder string) *SkillInfo {
	content := string(data)
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return nil
	}

	yamlContent := strings.TrimSpace(parts[1])
	markdownBody := strings.TrimSpace(parts[2])

	skill := &SkillInfo{
		Folder:     folder,
		Path:       filepath.Join(dir, folder),
		Markdown:   markdownBody,
		HasScripts: false,
	}

	// 解析 YAML 字段
	for _, line := range strings.Split(yamlContent, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			skill.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		} else if strings.HasPrefix(line, "description:") {
			skill.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		} else if strings.Contains(line, "emoji:") {
			skill.Emoji = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "emoji:"))
		}
	}

	// 检查是否有 scripts 目录
	scriptsDir := filepath.Join(dir, folder, "scripts")
	if info, err := os.Stat(scriptsDir); err == nil && info.IsDir() {
		if files, _ := os.ReadDir(scriptsDir); len(files) > 0 {
			skill.HasScripts = true
			scripts := []string{}
			for _, f := range files {
				if !f.IsDir() {
					scripts = append(scripts, f.Name())
				}
			}
			skill.Scripts = scripts
		}
	}

	// 解析 markdown 章节标题
	sections := []string{}
	for _, line := range strings.Split(markdownBody, "\n") {
		if strings.HasPrefix(line, "## ") {
			sections = append(sections, strings.TrimPrefix(line, "## "))
		}
	}
	skill.Sections = sections

	return skill
}

// ListJSON 获取技能列表 JSON 字符串
func (s *SkillService) ListJSON() string {
	skills := s.List()
	result := map[string]interface{}{
		"skill_dir": s.GetSkillDir(),
		"skills":    skills,
		"total":     len(skills),
	}
	data, _ := json.Marshal(result)
	return string(data)
}

// ListRaw 获取技能列表（原始 map 格式，兼容旧接口）
func (s *SkillService) ListRaw() []map[string]interface{} {
	skills := s.List()
	result := make([]map[string]interface{}, 0, len(skills))
	for _, skill := range skills {
		m := map[string]interface{}{
			"folder":      skill.Folder,
			"path":        skill.Path,
			"name":        skill.Name,
			"description": skill.Description,
			"markdown":    skill.Markdown,
			"has_scripts": skill.HasScripts,
			"sections":    skill.Sections,
		}
		if skill.Emoji != "" {
			m["emoji"] = skill.Emoji
		}
		if skill.HasScripts {
			m["scripts"] = skill.Scripts
		}
		result = append(result, m)
	}
	return result
}