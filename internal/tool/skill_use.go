package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-claw/internal/skill"
)

// SkillUseTool 技能调用工具 — 对标 QwenPaw 的 register_agent_skill()。
// AI 调用此工具获取 SKILL.md 全文和执行指引，而非通过 system prompt 注入。
type SkillUseTool struct {
	skillDir string
	// 预构建的技能名称→目录映射
	skillDirs map[string]string
	// 技能名称列表（供 listAvailableSkills 使用）
	skillNames []string
}

// NewSkillUseTool 创建技能调用工具
func NewSkillUseTool(skillDir string, reg *skill.SkillRegistry) *SkillUseTool {
	t := &SkillUseTool{
		skillDir:  skillDir,
		skillDirs: make(map[string]string),
	}
	if reg != nil {
		for _, s := range reg.List() {
			t.skillNames = append(t.skillNames, s.Name)
			t.skillDirs[s.Name] = s.SkillPath
		}
	}
	return t
}

func (t *SkillUseTool) Name() string        { return "skill_use" }
func (t *SkillUseTool) Description() string { return "调用指定技能获取详细执行指引。传入 skill_name 和 task 参数。" }

func (t *SkillUseTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"skill_name": map[string]interface{}{
				"type":        "string",
				"description": "技能名称，如 tushare-data、Excel / XLSX",
			},
			"task": map[string]interface{}{
				"type":        "string",
				"description": "用户要完成的具体任务",
			},
		},
		"required": []string{"skill_name", "task"},
	}
}

func (t *SkillUseTool) Execute(_ context.Context, params map[string]interface{}) (string, error) {
	name, _ := params["skill_name"].(string)
	task, _ := params["task"].(string)

	if name == "" {
		return t.listAvailableSkills(), nil
	}

	// 查找 skill 目录
	var skillDir string
	if d, ok := t.skillDirs[name]; ok {
		skillDir = d
	} else {
		// fallback: 遍历 skills/ 目录匹配 SKILL.md 中的 name
		entries, err := os.ReadDir(t.skillDir)
		if err != nil {
			return fmt.Sprintf("无法读取技能目录: %v", err), nil
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			mdPath := filepath.Join(t.skillDir, entry.Name(), "SKILL.md")
			s, parseErr := skill.ParseSkill(mdPath)
			if parseErr == nil && s.Name == name {
				skillDir = filepath.Join(t.skillDir, entry.Name())
				break
			}
		}
		if skillDir == "" {
			return fmt.Sprintf("未找到技能 '%s'。可用技能：\n%s", name, t.listNames()), nil
		}
	}

	// 读取并解析 SKILL.md
	mdPath := filepath.Join(skillDir, "SKILL.md")
	s, err := skill.ParseSkill(mdPath)
	if err != nil {
		// 解析失败则返回原文前 2000 字符
		content, readErr := os.ReadFile(mdPath)
		if readErr != nil {
			return fmt.Sprintf("读取技能文件失败: %v", readErr), nil
		}
		text := string(content)
		if len(text) > 2000 {
			text = text[:2000] + "..."
		}
		return fmt.Sprintf("# 技能: %s\n\n## 用户任务\n%s\n\n## SKILL.md (截断)\n%s\n\n请根据以上内容完成任务。", name, task, text), nil
	}

	// 列出可用脚本
	var scripts []string
	scriptsDir := filepath.Join(skillDir, "scripts")
	if entries, err := os.ReadDir(scriptsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				scripts = append(scripts, e.Name())
			}
		}
	}

	// 构建精简执行计划
	// 将相对路径转为绝对路径（相对于 skillDir）
	absScriptsDir := filepath.Join(skillDir, "scripts")

	var result strings.Builder
	result.WriteString(fmt.Sprintf("# %s %s\n\n", s.Emoji(), s.Name))
	result.WriteString(fmt.Sprintf("## 用户任务\n%s\n\n", task))

	// 直接给出可执行命令
	if len(scripts) > 0 {
		result.WriteString("## 执行步骤\n")
		result.WriteString(fmt.Sprintf("1. 先检查环境：`python %s/check_env.py`\n", absScriptsDir))
		result.WriteString(fmt.Sprintf("2. 再执行任务脚本\n\n"))
		result.WriteString("## 可用脚本 (绝对路径)\n")
		for _, sc := range scripts {
			result.WriteString(fmt.Sprintf("- `python %s/%s`\n", absScriptsDir, sc))
		}
		result.WriteString("\n")
	} else {
		result.WriteString("## 执行步骤\n")
		result.WriteString(fmt.Sprintf("读取技能文档了解详情：read_file(%s/SKILL.md)\n\n", skillDir))
	}

	result.WriteString("## 技能简介\n")
	result.WriteString(s.Description)
	result.WriteString("\n\n---\n")
	result.WriteString("直接复制上述命令执行。不要再次调用 skill_use。不要读 SKILL.md 除非命令执行失败需要排查。")

	return result.String(), nil
}

func (t *SkillUseTool) listAvailableSkills() string {
	if len(t.skillNames) == 0 {
		return "没有可用的技能"
	}
	return fmt.Sprintf("可用技能：\n%s\n\n用法: skill_use(skill_name=\"技能名\", task=\"任务描述\")",
		t.listNames())
}

func (t *SkillUseTool) listNames() string {
	var items []string
	for _, n := range t.skillNames {
		items = append(items, "  - "+n)
	}
	return strings.Join(items, "\n")
}
