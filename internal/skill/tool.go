package skill

import (
	"context"
	"encoding/json"
	"fmt"
)

// SkillUseTool Skill 调用工具（让 AI 可以主动调用 Skill）
type SkillUseTool struct {
	executor *Executor
}

func NewSkillUseTool(executor *Executor) *SkillUseTool {
	return &SkillUseTool{executor: executor}
}

func (t *SkillUseTool) Name() string {
	return "skill_use"
}

func (t *SkillUseTool) Description() string {
	desc := "技能调用工具。可调用已安装的技能来完成特定任务。"
	skills := t.executor.registry.List()
	if len(skills) > 0 {
		desc += "\n\n可用技能:\n"
		for _, s := range skills {
			desc += fmt.Sprintf("- %s %s: %s\n", s.Emoji(), s.Name, truncate(s.Description, 60))
		}
	}
	return desc
}

func (t *SkillUseTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "技能名称",
			},
			"args": map[string]interface{}{
				"type":        "object",
				"description": "技能参数 (键值对)",
			},
		},
		"required": []string{"name"},
	}
}

func (t *SkillUseTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	name, _ := params["name"].(string)
	if name == "" {
		// 无名称时，列出可用技能
		return t.executor.ListSkills(), nil
	}

	// 解析参数
	args := make(map[string]string)
	if argsRaw, ok := params["args"].(map[string]interface{}); ok {
		for k, v := range argsRaw {
			args[k] = fmt.Sprintf("%v", v)
		}
	}
	// 也支持扁平参数（如 action="navigate" 直接作为参数）
	for k, v := range params {
		if k == "name" || k == "args" {
			continue
		}
		args[k] = fmt.Sprintf("%v", v)
	}

	result, err := t.executor.Execute(ctx, name, args)
	if err != nil {
		return "", err
	}

	// 限制输出长度
	if len(result) > 3000 {
		result = result[:3000] + "\n...(已截断)"
	}

	return result, nil
}

// ToJSON 将参数转为 JSON 字符串
func ToJSON(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}