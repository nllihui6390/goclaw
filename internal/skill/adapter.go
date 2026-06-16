package skill

import (
	"encoding/json"
	"fmt"

	goAgentSkill "github.com/nllihui6390/go-agent/skill"
)

// ConvertToGoAgentRegistry 将 go-claw 的 skill.Registry 转换为 go-agent 的 skill.Registry
// 用于注入到 go-agent 的 Config.Skills
//
// 转换策略：
//   - Name/Emoji 保持不变
//   - Description 追加 SKILL.md 路径提示（引导 AI 读取完整文件）
//   - Workflow 追加 Error Handling 章节（确保异常处理信息可见）
//   - Input/Output 直接映射
//   - Prompt/Error 存入 Metadata（go-agent formatSkillPrompt 不显示这两个字段）
func ConvertToGoAgentRegistry(clawReg *SkillRegistry) *goAgentSkill.Registry {
	if clawReg == nil {
		fmt.Println("[Adapter] clawReg is nil")
		return nil
	}

	skills := clawReg.List()
	fmt.Printf("[Adapter] clawReg.List() returned %d skills\n", len(skills))
	if len(skills) == 0 {
		return nil
	}

	goAgentReg := goAgentSkill.NewRegistry()
	for i, clawSkill := range skills {
		fmt.Printf("[Adapter] Processing skill %d: %s (emoji=%s)\n", i, clawSkill.Name, clawSkill.Emoji())
		// 对标 QwenPaw: 仅注入简要概要（name + description + 路径），
	// AI 通过 read_file 按需获取完整 SKILL.md
	desc := clawSkill.Description

	goAgentSkill := &goAgentSkill.Skill{
		Name:        clawSkill.Name,
		Description: desc,
		Emoji:       clawSkill.Emoji(),
		Prompt:      "",
		Workflow:    "",
		Input:       "",
		Output:      "",
		Error:       "",
		Requires:    clawSkill.Metadata.OpenClaw.Requires.Bins,
		Metadata: map[string]interface{}{
			"skill_path": clawSkill.SkillPath,
			"scripts":    clawSkill.Scripts,
		},
	}
		goAgentReg.Register(goAgentSkill)
		fmt.Printf("[Adapter] Registered skill %s, goAgentReg.Count()=%d\n", clawSkill.Name, goAgentReg.Count())
	}
	data, _ := json.Marshal(goAgentReg)
	fmt.Printf("[Adapter] Final goAgentReg JSON: %s\n", string(data))
	return goAgentReg
}

// truncateSkillBody 截断 skill body，保留前 N 个字符（按换行边界截断）
func truncateSkillBody(body string, maxLen int) string {
	if len(body) <= maxLen {
		return body
	}
	// 在 maxLen 位置附近找最近的换行符
	cut := maxLen
	for i := maxLen; i < len(body) && i < maxLen+200; i++ {
		if body[i] == '\n' {
			cut = i
			break
		}
	}
	return body[:cut] + "\n...(truncated)"
}
