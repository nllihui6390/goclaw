package multiagent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AgentProcessor Agent 处理接口（避免循环导入）
type AgentProcessor interface {
	Process(ctx context.Context, sessionID, message string) (string, error)
	GetInfo() string
}

// ListAgentsTool 列出所有 Agent
type ListAgentsTool struct {
	agents map[string]AgentProcessor
}

func NewListAgentsTool(agents map[string]AgentProcessor) *ListAgentsTool {
	return &ListAgentsTool{agents: agents}
}

func (t *ListAgentsTool) Name() string {
	return "list_agents"
}

func (t *ListAgentsTool) Description() string {
	return "列出系统中所有可用的 Agent 及其描述。"
}

func (t *ListAgentsTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *ListAgentsTool) Execute(_ context.Context, _ map[string]interface{}) (string, error) {
	var result strings.Builder
	result.WriteString("可用 Agent 列表:\n\n")
	for name, ag := range t.agents {
		info := ag.GetInfo()
		if len(info) > 50 {
			info = info[:50] + "..."
		}
		result.WriteString(fmt.Sprintf("- **%s**: %s\n", name, info))
	}
	return result.String(), nil
}

// ChatWithAgentTool 与其他 Agent 对话
type ChatWithAgentTool struct {
	agents map[string]AgentProcessor
}

func NewChatWithAgentTool(agents map[string]AgentProcessor) *ChatWithAgentTool {
	return &ChatWithAgentTool{agents: agents}
}

func (t *ChatWithAgentTool) Name() string {
	return "chat_with_agent"
}

func (t *ChatWithAgentTool) Description() string {
	return "与指定 Agent 进行对话，等待其回复。用于多 Agent 协作场景。"
}

func (t *ChatWithAgentTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"agent_name": map[string]interface{}{
				"type":        "string",
				"description": "目标 Agent 名称",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "要发送给目标 Agent 的消息内容",
			},
			"caller_id": map[string]interface{}{
				"type":        "string",
				"description": "发送方 Agent 名称（用于标识消息来源）",
			},
		},
		"required": []string{"agent_name", "message"},
	}
}

func (t *ChatWithAgentTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	agentName, ok := params["agent_name"].(string)
	if !ok || agentName == "" {
		return "", fmt.Errorf("缺少 agent_name 参数")
	}
	message, ok := params["message"].(string)
	if !ok || message == "" {
		return "", fmt.Errorf("缺少 message 参数")
	}
	callerID, _ := params["caller_id"].(string)

	if callerID != "" {
		message = fmt.Sprintf("[Agent %s requesting] %s", callerID, message)
	}

	ag, exists := t.agents[agentName]
	if !exists {
		return "", fmt.Errorf("Agent '%s' 不存在", agentName)
	}

	sessionID := fmt.Sprintf("inter_agent:%s_to_%s_%d", callerID, agentName, time.Now().Unix())

	response, err := ag.Process(ctx, sessionID, message)
	if err != nil {
		return "", fmt.Errorf("Agent '%s' 处理失败: %v", agentName, err)
	}

	return fmt.Sprintf("[Agent %s 回复] %s", agentName, response), nil
}

// SubmitToAgentTool 向 Agent 提交后台任务
type SubmitToAgentTool struct {
	agents map[string]AgentProcessor
}

func NewSubmitToAgentTool(agents map[string]AgentProcessor) *SubmitToAgentTool {
	return &SubmitToAgentTool{agents: agents}
}

func (t *SubmitToAgentTool) Name() string {
	return "submit_to_agent"
}

func (t *SubmitToAgentTool) Description() string {
	return "向指定 Agent 提交后台任务，不等待回复。返回任务 ID 用于后续查询结果。"
}

func (t *SubmitToAgentTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"agent_name": map[string]interface{}{
				"type":        "string",
				"description": "目标 Agent 名称",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "要提交的任务内容",
			},
			"caller_id": map[string]interface{}{
				"type":        "string",
				"description": "发送方 Agent 名称",
			},
		},
		"required": []string{"agent_name", "message"},
	}
}

func (t *SubmitToAgentTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	agentName, _ := params["agent_name"].(string)
	message, _ := params["message"].(string)
	callerID, _ := params["caller_id"].(string)

	if agentName == "" || message == "" {
		return "", fmt.Errorf("缺少 agent_name 或 message 参数")
	}

	ag, exists := t.agents[agentName]
	if !exists {
		return "", fmt.Errorf("Agent '%s' 不存在", agentName)
	}

	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	sessionID := fmt.Sprintf("bg_task:%s_%s", callerID, taskID)

	if callerID != "" {
		message = fmt.Sprintf("[Agent %s 后台任务] %s", callerID, message)
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		ag.Process(bgCtx, sessionID, message)
	}()

	return fmt.Sprintf("任务已提交，ID: %s, Agent: %s, 会话: %s\n使用 check_agent_task 查询结果", taskID, agentName, sessionID), nil
}

// CheckAgentTaskTool 查询 Agent 任务结果
type CheckAgentTaskTool struct{}

func NewCheckAgentTaskTool() *CheckAgentTaskTool {
	return &CheckAgentTaskTool{}
}

func (t *CheckAgentTaskTool) Name() string {
	return "check_agent_task"
}

func (t *CheckAgentTaskTool) Description() string {
	return "查询后台提交的 Agent 任务结果。返回 Agent 的回复内容。"
}

func (t *CheckAgentTaskTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "提交任务时返回的会话 ID",
			},
		},
		"required": []string{"session_id"},
	}
}

func (t *CheckAgentTaskTool) Execute(_ context.Context, params map[string]interface{}) (string, error) {
	sessionID, _ := params["session_id"].(string)
	if sessionID == "" {
		return "", fmt.Errorf("缺少 session_id 参数")
	}
	return fmt.Sprintf("请使用会话管理功能查询会话 %s 的消息记录", sessionID), nil
}