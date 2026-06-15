package skill

import (
	"context"
	"fmt"
	"strings"
)

// Executor 技能执行器
type Executor struct {
	registry *Registry
}

// NewExecutor 创建执行器
func NewExecutor(registry *Registry) *Executor {
	return &Executor{registry: registry}
}

// Execute 执行技能
func (e *Executor) Execute(ctx context.Context, skillName string, params map[string]interface{}) (string, error) {
	skill, ok := e.registry.Get(skillName)
	if !ok {
		return "", fmt.Errorf("skill not found: %s", skillName)
	}

	// 检查依赖
	for _, req := range skill.Requires {
		if !checkDependency(req) {
			return "", fmt.Errorf("missing dependency: %s", req)
		}
	}

	// 技能执行：返回提示词供 AI 使用
	return formatSkillPrompt(skill), nil
}

// ExecuteWithParams 执行技能并替换变量
func (e *Executor) ExecuteWithParams(ctx context.Context, skillName string, params map[string]interface{}) (string, error) {
	skill, ok := e.registry.Get(skillName)
	if !ok {
		return "", fmt.Errorf("skill not found: %s", skillName)
	}

	// 替换变量 {{param}}
	prompt := skill.Workflow
	for key, value := range params {
		placeholder := "{{" + key + "}}"
		prompt = strings.ReplaceAll(prompt, placeholder, fmt.Sprintf("%v", value))
	}

	return prompt, nil
}

// MatchAndExecute 匹配并执行最适合的技能
func (e *Executor) MatchAndExecute(ctx context.Context, query string, params map[string]interface{}) (string, error) {
	matches := e.registry.Match(query, 1)
	if len(matches) == 0 {
		return "", fmt.Errorf("no matching skill for: %s", query)
	}

	return e.ExecuteWithParams(ctx, matches[0].Skill.Name, params)
}

// checkDependency 检查依赖
func checkDependency(bin string) bool {
	// 简化实现：假设所有依赖都满足
	// 实际使用时可调用 shell 检查
	return true
}

// SkillSummary 技能摘要
type SkillSummary struct {
	Name        string
	Description string
	Emoji       string
}

// GetSummaries 获取技能摘要列表
func (r *Registry) GetSummaries() []SkillSummary {
	result := make([]SkillSummary, 0, len(r.skills))
	for _, skill := range r.skills {
		result = append(result, SkillSummary{
			Name:        skill.Name,
			Description: skill.Description,
			Emoji:       skill.Emoji,
		})
	}
	return result
}