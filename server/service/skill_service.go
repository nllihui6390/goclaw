package service

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// SkillCredential 凭证配置
type SkillCredential struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	HowToGet    string `json:"how_to_get,omitempty"`
}

// SkillPackage 包依赖
type SkillPackage struct {
	Name string `json:"name"`
}

// SkillEnvVar 环境变量
type SkillEnvVar struct {
	Name      string `json:"name"`
	Required  bool   `json:"required,omitempty"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

// SkillRequirements 运行要求
type SkillRequirements struct {
	Python             string          `json:"python,omitempty"`
	Packages           []SkillPackage  `json:"packages,omitempty"`
	EnvironmentVariables []SkillEnvVar  `json:"environment_variables,omitempty"`
	NetworkAccess      bool            `json:"network_access,omitempty"`
}

// SkillRegistryEntry 技能池中的单条记录
type SkillRegistryEntry struct {
	Name         string            `json:"name"`
	Folder       string            `json:"folder"`
	Description  string            `json:"description"`
	Author       string            `json:"author,omitempty"`
	Version      string            `json:"version,omitempty"`
	Emoji        string            `json:"emoji,omitempty"`
	Credentials  []SkillCredential `json:"credentials,omitempty"`
	Requirements *SkillRequirements `json:"requirements,omitempty"`
	HasScripts   bool              `json:"has_scripts"`
	Scripts      []string          `json:"scripts,omitempty"`
	Sections     []string          `json:"sections,omitempty"`
	DiscoveredAt string            `json:"discovered_at,omitempty"`
}

// SkillRegistry 技能池注册表
type SkillRegistry struct {
	Version int                  `json:"version"`
	Skills  []SkillRegistryEntry `json:"skills"`
}

// SkillInfo 技能详细信息（用于 API 返回）
type SkillInfo struct {
	Folder       string             `json:"folder"`
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Author       string             `json:"author,omitempty"`
	Version      string             `json:"version,omitempty"`
	Emoji        string             `json:"emoji,omitempty"`
	Credentials  []SkillCredential  `json:"credentials,omitempty"`
	Requirements *SkillRequirements `json:"requirements,omitempty"`
	Markdown     string             `json:"markdown"`
	HasScripts   bool               `json:"has_scripts"`
	Scripts      []string           `json:"scripts,omitempty"`
	Sections     []string           `json:"sections"`
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
			Name:         parsed.Name,
			Folder:       entry.Name(),
			Description:  parsed.Description,
			Author:       parsed.Author,
			Version:      parsed.Version,
			Emoji:        parsed.Emoji,
			Credentials:  parsed.Credentials,
			Requirements: parsed.Requirements,
			HasScripts:   parsed.HasScripts,
			Scripts:      parsed.Scripts,
			Sections:     parsed.Sections,
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
					Folder:       entry.Folder,
					Name:         entry.Name,
					Description:  entry.Description,
					Author:       entry.Author,
					Version:      entry.Version,
					Emoji:        entry.Emoji,
					Credentials:  entry.Credentials,
					Requirements: entry.Requirements,
					Markdown:     markdown,
					HasScripts:   entry.HasScripts,
					Scripts:      entry.Scripts,
					Sections:     entry.Sections,
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

// skillYAMLFrontmatter YAML 前置元数据结构
type skillYAMLFrontmatter struct {
	Name        string             `yaml:"name"`
	Description string             `yaml:"description"`
	Author      string             `yaml:"author"`
	Version     string             `yaml:"version"`
	Credentials []SkillCredential  `yaml:"credentials"`
	Requirements *SkillRequirements `yaml:"requirements"`
	Metadata    struct {
		OpenClaw struct {
			Emoji string `yaml:"emoji"`
		} `yaml:"openclaw"`
		ClawdBot struct {
			Emoji string `yaml:"emoji"`
		} `yaml:"clawdbot"`
	} `yaml:"metadata"`
}

// parsedSkill 解析 SKILL.md 的结果
type parsedSkill struct {
	Name         string
	Description  string
	Author       string
	Version      string
	Emoji        string
	Credentials  []SkillCredential
	Requirements *SkillRequirements
	HasScripts   bool
	Scripts      []string
	Sections     []string
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

	// 使用 YAML 解析器解析前置元数据
	var frontmatter skillYAMLFrontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &frontmatter); err != nil {
		// YAML 解析失败，尝试简单的行解析作为降级
		return s.parseSkillMDSimple(yamlContent, markdownBody)
	}

	if frontmatter.Name == "" {
		return nil
	}

	skill := &parsedSkill{
		Name:         frontmatter.Name,
		Description:  frontmatter.Description,
		Author:       frontmatter.Author,
		Version:      frontmatter.Version,
		Emoji:        frontmatter.Metadata.OpenClaw.Emoji,
		Credentials:  frontmatter.Credentials,
		Requirements: frontmatter.Requirements,
		HasScripts:   false,
	}
	// ClawdBot emoji 兜底（sill 格式兼容）
	if skill.Emoji == "" {
		skill.Emoji = frontmatter.Metadata.ClawdBot.Emoji
	}

	// 检查 scripts 目录
	skillDir := s.skillDir()
	scriptsDir := filepath.Join(skillDir, frontmatter.Name, "scripts")
	// 也通过 folder 名查找
	if info, err := os.Stat(scriptsDir); err == nil && info.IsDir() {
		if files, _ := os.ReadDir(scriptsDir); len(files) > 0 {
			skill.HasScripts = true
			for _, f := range files {
				if !f.IsDir() {
					skill.Scripts = append(skill.Scripts, f.Name())
				}
			}
		}
	}

	// 解析 markdown 章节标题
	for _, line := range strings.Split(markdownBody, "\n") {
		if strings.HasPrefix(line, "## ") {
			skill.Sections = append(skill.Sections, strings.TrimPrefix(line, "## "))
		}
	}

	return skill
}

// parseSkillMDSimple 简单解析（YAML 解析失败时的降级方案）
func (s *SkillService) parseSkillMDSimple(yamlContent, markdownBody string) *parsedSkill {
	skill := &parsedSkill{}

	// 简单的行解析
	for _, line := range strings.Split(yamlContent, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			skill.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		} else if strings.HasPrefix(line, "description:") {
			skill.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		} else if strings.HasPrefix(line, "author:") {
			skill.Author = strings.TrimSpace(strings.TrimPrefix(line, "author:"))
		} else if strings.HasPrefix(line, "version:") {
			skill.Version = strings.TrimSpace(strings.TrimPrefix(line, "version:"))
		} else if strings.Contains(line, "emoji:") {
			// 处理嵌套的 emoji: metadata.openclaw.emoji 或 metadata.clawdbot.emoji
			parts := strings.Split(line, "emoji:")
			if len(parts) > 1 {
				skill.Emoji = strings.TrimSpace(parts[len(parts)-1])
			}
		} else if strings.HasPrefix(line, "metadata:") {
			// sill 格式: metadata 行是 JSON 字符串，如 metadata: {"clawdbot":{"emoji":"📘"}}
			metaJSON := strings.TrimSpace(strings.TrimPrefix(line, "metadata:"))
			if strings.HasPrefix(metaJSON, "{") {
				var meta map[string]interface{}
				if json.Unmarshal([]byte(metaJSON), &meta) == nil {
					for _, key := range []string{"clawdbot", "openclaw"} {
						if sub, ok := meta[key].(map[string]interface{}); ok {
							if emoji, ok := sub["emoji"].(string); ok && emoji != "" {
								skill.Emoji = emoji
							}
						}
					}
				}
			}
		}
	}

	if skill.Name == "" {
		return nil
	}

	// 解析 markdown 章节标题
	for _, line := range strings.Split(markdownBody, "\n") {
		if strings.HasPrefix(line, "## ") {
			skill.Sections = append(skill.Sections, strings.TrimPrefix(line, "## "))
		}
	}

	return skill
}

// ImportFromZip 从 zip 数据导入技能
// 返回导入结果 map
func (s *SkillService) ImportFromZip(data []byte) (map[string]interface{}, error) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "skill_upload_")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir) // 清理临时目录

	// 解压 zip
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}

	// 解压文件
	for _, f := range reader.File {
		info := f.FileInfo()
		if info.IsDir() || strings.HasPrefix(f.Name, "__MACOSX") || strings.Contains(f.Name, ".DS_Store") {
			continue
		}

		targetPath := filepath.Join(tmpDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(tmpDir)+string(os.PathSeparator)) {
			continue // 防止路径遍历
		}

		os.MkdirAll(filepath.Dir(targetPath), 0755)
		rc, err := f.Open()
		if err != nil {
			continue
		}
		dst, _ := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		io.Copy(dst, rc)
		dst.Close()
		rc.Close()
	}

	// 扫描临时目录寻找 SKILL.md
	var imported []SkillRegistryEntry

	// 情况1: SKILL.md 直接位于 zip 根目录（无子目录）
	rootSkillFile := filepath.Join(tmpDir, "SKILL.md")
	if _, err := os.Stat(rootSkillFile); err == nil {
		skillData, _ := os.ReadFile(rootSkillFile)
		parsed := s.parseSkillMD(skillData)
		if parsed != nil && parsed.Name != "" {
			if e := s.importParsedSkill(parsed, tmpDir); e != nil {
				imported = append(imported, *e)
			} else {
				return nil, fmt.Errorf("技能 '%s' 已存在", parsed.Name)
			}
		}
	}

	// 情况2: SKILL.md 位于子目录中（标准结构）
	entries, _ := os.ReadDir(tmpDir)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(tmpDir, entry.Name())
		skillFile := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			continue
		}

		skillData, _ := os.ReadFile(skillFile)
		parsed := s.parseSkillMD(skillData)
		if parsed == nil || parsed.Name == "" {
			continue
		}

		if e := s.importParsedSkill(parsed, skillDir); e != nil {
			imported = append(imported, *e)
		} else if len(imported) == 0 {
			return nil, fmt.Errorf("技能 '%s' 已存在", parsed.Name)
		}
	}

	if len(imported) == 0 {
		return nil, fmt.Errorf("未找到有效技能（缺少 SKILL.md 或 name 字段）")
	}

	// 更新技能池注册表
	reg, _ := s.GetPool()
	reg.Skills = append(reg.Skills, imported...)
	s.SavePool(reg)

	return map[string]interface{}{
		"message": "导入成功",
		"skills":  imported,
		"total":   len(imported),
	}, nil
}

// importParsedSkill 将解析后的 SKILL.md 写入技能目录，返回导入条目；如果技能已存在则返回 nil
func (s *SkillService) importParsedSkill(parsed *parsedSkill, sourceDir string) *SkillRegistryEntry {
	targetSkillDir := filepath.Join(s.skillDir(), parsed.Name)
	if _, err := os.Stat(targetSkillDir); err == nil {
		return nil // 已存在
	}

	os.MkdirAll(targetSkillDir, 0755)
	copyDirContents(sourceDir, targetSkillDir)

	return &SkillRegistryEntry{
		Name:         parsed.Name,
		Folder:       parsed.Name,
		Description:  parsed.Description,
		Author:       parsed.Author,
		Version:      parsed.Version,
		Emoji:        parsed.Emoji,
		Credentials:  parsed.Credentials,
		Requirements: parsed.Requirements,
		HasScripts:   parsed.HasScripts,
		Scripts:      parsed.Scripts,
		Sections:     parsed.Sections,
		DiscoveredAt: time.Now().Format(time.RFC3339),
	}
}

// copyFile 复制单个文件
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// copyDirContents 复制目录内容（不包括目录本身，只复制子项）
func copyDirContents(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			os.MkdirAll(dstPath, 0755)
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyDir 复制目录内容
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			os.MkdirAll(dstPath, 0755)
			copyDir(srcPath, dstPath)
		} else {
			data, _ := os.ReadFile(srcPath)
			os.WriteFile(dstPath, data, 0644)
		}
	}
	return nil
}