package agent

import (
	"context"
	"strings"

	goModel "github.com/nllihui6390/go-agent/model"

	"go-claw/internal/skill"
	"go-claw/internal/token_usage"
	glog "go-claw/pkg/log"
)

// Runtime Agent运行时（仅管理 ChatModel，所有 Agent 循环逻辑由 go-agent 处理）
type Runtime struct {
	config       *AgentConfig
	chatModel    goModel.ChatModel // go-agent 模型
	workspaceDir string            // 用于缓存大工具结果
}

// NewRuntime 创建运行时
func NewRuntime(cfg *AgentConfig) *Runtime {
	r := &Runtime{
		config: cfg,
	}
	r.chatModel = newChatModel(cfg)
	return r
}

// newChatModel 从当前运行时配置创建 go-agent ChatModel
func newChatModel(cfg *AgentConfig) goModel.ChatModel {
	rtCfg := getRuntimeConfig(cfg)
	switch rtCfg.ProviderType {
	case "ollama":
		return goModel.NewOllamaModel(goModel.ModelConfig{
			Model:   rtCfg.Model,
			APIKey:  rtCfg.APIKey,
			BaseURL: rtCfg.BaseURL,
			Timeout: 180,
		})
	default:
		return goModel.NewOpenAIModel(goModel.ModelConfig{
			Model:   rtCfg.Model,
			APIKey:  rtCfg.APIKey,
			BaseURL: rtCfg.BaseURL,
			Timeout: 180,
		})
	}
}

// refreshChatModel 根据动态配置刷新 ChatModel（热切换）
func (r *Runtime) refreshChatModel() {
	if r.config.ConfigProvider == nil {
		return
	}
	r.chatModel = newChatModel(r.config)
}

// SetWorkspaceDir 设置工作空间目录（用于缓存文件）
func (r *Runtime) SetWorkspaceDir(dir string) {
	r.workspaceDir = dir
}

// SetSkillRegistry 设置技能注册中心（用于热重载）
func (r *Runtime) SetSkillRegistry(reg *skill.Registry) {
	r.config.SkillRegistry = reg
}

// runtimeConfig 运行时配置（model、apiKey、baseURL、providerType）
type runtimeConfig struct {
	Model        string
	APIKey       string
	BaseURL      string
	ProviderType string
}

// getRuntimeConfig 获取当前运行时配置（优先从 ConfigProvider 动态获取，降级使用静态值）
func getRuntimeConfig(cfg *AgentConfig) *runtimeConfig {
	if cfg.ConfigProvider != nil {
		model, apiKey, baseURL, providerType := cfg.ConfigProvider()
		return &runtimeConfig{
			Model:        model,
			APIKey:       apiKey,
			BaseURL:      baseURL,
			ProviderType: providerType,
		}
	}
	return &runtimeConfig{
		Model:        cfg.Model,
		APIKey:       cfg.APIKey,
		BaseURL:      cfg.BaseURL,
		ProviderType: cfg.ProviderType,
	}
}

// ─────────── 通用 LLM 调用（供 supervisor/dream/memory 使用） ───────────

// CallLLM 通用 LLM 调用（简化版，直接使用 go-agent ChatModel）
// 供 SupervisorAgent 路由、DreamOptimizer 记忆整理、ExtractMemory 使用
func (r *Runtime) CallLLM(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	logger := glog.Logger()
	rtCfg := getRuntimeConfig(r.config)
	logger.Debug("[Runtime] CallLLM", "provider", rtCfg.ProviderType, "model", rtCfg.Model)

	r.refreshChatModel()

	messages := []goModel.Msg{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	resp, err := r.chatModel.Call(ctx, messages)
	if err != nil {
		logger.Error("[Runtime] ChatModel 调用失败", "err", err)
		return "", err
	}

	// 记录 Token 使用量
	if resp.Usage.TotalTokens > 0 {
		token_usage.Record(rtCfg.ProviderType, rtCfg.Model, resp.Usage.InputTokens, resp.Usage.OutputTokens)
	}

	return resp.Content, nil
}

// CallLLMWithMessages 通用 LLM 调用（带完整消息列表）
// 供 SupervisorAgent 路由使用，支持自定义消息格式
func (r *Runtime) CallLLMWithMessages(ctx context.Context, messages []goModel.Msg) (string, error) {
	logger := glog.Logger()

	r.refreshChatModel()

	resp, err := r.chatModel.Call(ctx, messages)
	if err != nil {
		logger.Error("[Runtime] ChatModel 调用失败", "err", err)
		return "", err
	}

	return resp.Content, nil
}

// ─────────── 记忆提取（使用 go-agent ChatModel） ───────────

// ExtractMemory 调用 LLM 提取对话中的关键信息
func (r *Runtime) ExtractMemory(ctx context.Context, userMsg, assistantMsg string) string {
	logger := glog.Logger()

	extractPrompt := `请从以下对话中提取值得长期记住的关键信息。
只提取以下类型的内容：
1. 用户表达的重要偏好、习惯、身份信息
2. 关键决策或结论
3. 重要的事实或数据
4. 未完成但重要的待办事项

如果对话中没有值得记住的信息，返回空字符串。
每条信息用简短的一句话描述，不要添加解释。格式：
- 信息1
- 信息2

对话内容：`

	result, err := r.CallLLM(ctx, extractPrompt, userMsg+"\n\nAI回复: "+assistantMsg)
	if err != nil {
		logger.Warn("[Runtime] 记忆提取失败", "err", err)
		return ""
	}

	// 如果提取结果为空或无意义，不存储
	content := stripThinkTags(result)
	if content == "" || content == "无" || content == "没有值得记住的信息" {
		return ""
	}

	logger.Info("[Runtime] 记忆提取完成", "len", len(content))
	return content
}

// stripThinkTags 剥离 DeepSeek 等模型的内部推理标签
func stripThinkTags(content string) string {
	result := content
	for {
		start := strings.Index(result, "<think>")
		if start == -1 {
			break
		}
		sub := result[start:]
		end := strings.Index(sub, "</think>")
		if end == -1 {
			break
		}
		result = result[:start] + sub[end+len("</think>"):]
	}
	return strings.TrimSpace(result)
}