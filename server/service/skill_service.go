package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SkillRegistryEntry 技能池中的单条记录
type SkillRegistryEntry struct {
	Name        string   `json:"name"`
	Folder      string   `json:"folder"`
	Description string   `json:"description"`
	Emoji       string   `json:"emoji,omitempty"`
	HasScripts  bool     `json:"has_scripts"`
	Scripts     []string `json:"scripts,omitempty"`
	Sections    []string `json:"sections,omitempty"`
	DiscoveredAt string  `json:"discovered_at,omitempty"`
}

// SkillRegistry 技能池注册表
type SkillRegistry struct {
	Version int                `json:"version"`
	Skills  []SkillRegistryEntry `json:"skills"`
}

// SkillInfo 技能详细信息（用于 API 返回）
type SkillInfo struct {
	Folder      string   `json:"folder"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Emoji       string   `json:"emoji,omitempty"`
	Markdown    string   `json:"markdown"`
	HasScripts  bool     `json:"has_scripts"`
	Scripts     []string `json:"scripts,omitempty"`
	Sections    []string `json:"sections"`
}

// SkillService 技能管理服务
type SkillService struct {
	mu     sync.RWMutex
	config *ConfigService
	// 技能启用列表变化时的回调，用于动态重载 agent 技能
	OnSkillsChanged func(agentName string, enabledSkills []string)
}

// NewSkillService 创建技能服务
func NewSkillService(config *ConfigService) *SkillService {
	return &SkillService{config: config}
}

// dataDir 获取数据根目录
func (s *SkillService) dataDir() string {
	cfg := s.config.Get()
	gateway, _ := cfg["gateway"].(map[string]interface{})
	dataDir := "clawdata"
	if gateway != nil {
		if v, ok := gateway["data_dir"].(string); ok && v != "" {
			dataDir = v
		}
	}
	return dataDir
}

// skillDir 获取技能存放目录
func (s *SkillService) skillDir() string {
	cfg := s.config.Get()
	skillsCfg, _ := cfg["skills"].(map[string]interface{})
	if skillsCfg != nil {
		if v, ok := skillsCfg["skill_dir"].(string); ok && v != "" {
			return filepath.Join(s.dataDir(), v)
		}
	}
	return filepath.Join(s.dataDir(), "skills")
}

// GetSkillDir 获取技能目录（公开方法）
func (s *SkillService) GetSkillDir() string {
	return s.skillDir()
}

// registryFile 技能池注册表文件路径
func (s *SkillService) registryFile() string {
	return filepath.Join(s.dataDir(), "skills_registry.json")
}

// workspaceBase 工作空间根目录
func (s *SkillService) workspaceBase() string {
	return s.config.WorkspaceBase()
}

// ─────────── 技能池（全量） ───────────

// GetPool 获取全量技能池
func (s *SkillService) GetPool() (*SkillRegistry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.registryFile())
	if err != nil {
		// 文件不存在，返回空注册表
		return &SkillRegistry{Version: 1, Skills: []SkillRegistryEntry{}}, nil
	}

	var reg SkillRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return &SkillRegistry{Version: 1, Skills: []SkillRegistryEntry{}}, nil
	}
	return &reg, nil
}

// SavePool 保存技能池注册表
func (s *SkillService) SavePool(reg *SkillRegistry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	os.MkdirAll(filepath.Dir(s.registryFile()), 0755)
	data, _ := json.MarshalIndent(reg, "", "  ")
	return os.WriteFile(s.registryFile(), data, 0644)
}

// Scan 扫描技能目录，更新技能池注册表
func (s *SkillService) Scan() (*SkillRegistry, error) {
	dir := s.skillDir()
	os.MkdirAll(dir, 0755)

	// 读取现有注册表（保留已有记录）
	existingReg, _ := s.GetPool()
	existingMap := make(map[string]*SkillRegistryEntry)
	for i := range existingReg.Skills {
		existingMap[existingReg.Skills[i].Folder] = &existingReg.Skills[i]
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return existingReg, nil
	}

	newEntries := []SkillRegistryEntry{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillFile := filepath.Join(dir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}

		parsed := s.parseSkillMD(data)
		if parsed == nil {
			continue
		}

		entryRecord := SkillRegistryEntry{
			Name:        parsed.Name,
			Folder:      entry.Name(),
			Description: parsed.Description,
			Emoji:       parsed.Emoji,
			HasScripts:  parsed.HasScripts,
			Scripts:     parsed.Scripts,
			Sections:    parsed.Sections,
		}

		// 如果已存在，保留 discovered_at
		if existing, ok := existingMap[entry.Name()]; ok {
			entryRecord.DiscoveredAt = existing.DiscoveredAt
		} else {
			entryRecord.DiscoveredAt = time.Now().Format(time.RFC3339)
		}

		newEntries = append(newEntries, entryRecord)
	}

	reg := &SkillRegistry{Version: 1, Skills: newEntries}
	s.SavePool(reg)
	return reg, nil
}

// PoolJSON 技能池 JSON
func (s *SkillService) PoolJSON() string {
	reg, _ := s.GetPool()
	result := map[string]interface{}{
		"skill_dir": s.skillDir(),
		"skills":    reg.Skills,
		"total":     len(reg.Skills),
	}
	data, _ := json.Marshal(result)
	return string(data)
}

// ─────────── Agent 启用技能 ───────────

// enabledSkillsFile 获取指定 agent 的启用技能文件路径
func (s *SkillService) enabledSkillsFile(agentName string) string {
	return filepath.Join(s.workspaceBase(), agentName, "enabled_skills.json")
}

// GetEnabledSkills 获取指定 agent 已启用的技能名列表
func (s *SkillService) GetEnabledSkills(agentName string) []string {
	file := s.enabledSkillsFile(agentName)
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

// SetEnabledSkills 设置指定 agent 的启用技能列表
func (s *SkillService) SetEnabledSkills(agentName string, skills []string) error {
	file := s.enabledSkillsFile(agentName)
	os.MkdirAll(filepath.Dir(file), 0755)
	data, _ := json.MarshalIndent(skills, "", "  ")
	err := os.WriteFile(file, data, 0644)
	if err != nil {
		return err
	}

	// 触发回调，动态重载 agent 技能
	if s.OnSkillsChanged != nil {
		s.OnSkillsChanged(agentName, skills)
	}
	return nil
}

// GetEnabledSkillsJSON 获取指定 agent 启用技能详情（JSON）
func (s *SkillService) GetEnabledSkillsJSON(agentName string) string {
	enabledNames := s.GetEnabledSkills(agentName)
	reg, _ := s.GetPool()

	// 从技能池中筛选已启用的技能详情
	skillDir := s.skillDir()
	var enabledSkills []SkillInfo
	for _, name := range enabledNames {
		for _, entry := range reg.Skills {
			if entry.Name == name || entry.Folder == name {
				// 读取 markdown 正文
				skillFile := filepath.Join(skillDir, entry.Folder, "SKILL.md")
				markdown := ""
				if data, err := os.ReadFile(skillFile); err == nil {
					content := string(data)
					parts := strings.SplitN(content, "---", 3)
					if len(parts) >= 3 {
						markdown = strings.TrimSpace(parts[2])
					}
				}
				enabledSkills = append(enabledSkills, SkillInfo{
					Folder:      entry.Folder,
					Name:        entry.Name,
					Description: entry.Description,
					Emoji:       entry.Emoji,
					Markdown:    markdown,
					HasScripts:  entry.HasScripts,
					Scripts:     entry.Scripts,
					Sections:    entry.Sections,
				})
				break
			}
		}
	}

	result := map[string]interface{}{
		"agent":   agentName,
		"enabled": enabledNames,
		"skills":  enabledSkills,
		"total":   len(enabledSkills),
	}
	data, _ := json.Marshal(result)
	return string(data)
}

// SetEnabledSkillsJSON 设置指定 agent 的启用技能（JSON 字符串）
func (s *SkillService) SetEnabledSkillsJSON(agentName, skillsJSON string) error {
	var list []string
	if err := json.Unmarshal([]byte(skillsJSON), &list); err != nil {
		return err
	}
	return s.SetEnabledSkills(agentName, list)
}

// ─────────── 解析 SKILL.md ───────────

// parsedSkill 解析 SKILL.md 的结果
type parsedSkill struct {
	Name        string
	Description string
	Emoji       string
	HasScripts  bool
	Scripts     []string
	Sections    []string
}

// parseSkillMD 解析 SKILL.md 文件内容
func (s *SkillService) parseSkillMD(data []byte) *parsedSkill {
	content := string(data)
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return nil
	}

	yamlContent := strings.TrimSpace(parts[1])
	markdownBody := strings.TrimSpace(parts[2])

	skill := &parsedSkill{
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

	if skill.Name == "" {
		return nil
	}

	dir := s.skillDir()
	// 检查是否有 scripts 目录
	scriptsDir := filepath.Join(dir, skill.Name, "scripts")
	// 也通过 folder 查找
	if skill.Name != "" {
		scriptsDirByFolder := filepath.Join(dir, skill.Name, "scripts")
		if info, err := os.Stat(scriptsDirByFolder); err == nil && info.IsDir() {
			scriptsDir = scriptsDirByFolder
		}
	}

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