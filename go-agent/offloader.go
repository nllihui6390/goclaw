package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// =============================================
// Offloader 协议（ Offloader 协议）
//
// Offloader 把 agent 已经从上下文中移除的内容持久化到外部存储：
// - 被压缩的消息
// - 被截断的工具输出
//
// AgentScope 提供两个方法：
// - offload_context(session_id, msgs) → 返回引用
// - offload_tool_result(session_id, tool_result) → 返回引用
// =============================================

// Offloader 卸载器接口
type Offloader interface {
	// OffloadContext 持久化被压缩的消息；返回引用（例如文件路径）
	OffloadContext(ctx context.Context, sessionID string, msgs []Msg) (string, error)

	// OffloadToolResult 持久化被截断的工具结果；返回引用
	OffloadToolResult(ctx context.Context, sessionID string, result ContentBlock) (string, error)

	// SaveSummary 持久化压缩摘要
	SaveSummary(ctx context.Context, sessionID string, summary *Summary) error

	// Initialize 初始化工作空间
	Initialize(ctx context.Context) error

	// Cleanup 清理工作空间
	Cleanup(ctx context.Context) error
}

// =============================================
// LocalWorkspace 本地文件系统实现
// （ 的 LocalWorkspace）
//
// 目录结构：
// {workdir}/
//   data/                  → 多模态文件去重存储（按 SHA-256 哈希）
//   sessions/
//     {session_id}/
//       context.jsonl       → 被压缩的消息追加写入
//       tool_result-{id}.txt → 每条截断的工具结果独立文件
//   skills/                 → skill 目录（与 offload 无关）
// =============================================

// LocalWorkspace 本地文件系统工作空间
type LocalWorkspace struct {
	Workdir     string
	mu          sync.Mutex
	initialized bool
}

// NewLocalWorkspace 创建本地工作空间
func NewLocalWorkspace(workdir string) *LocalWorkspace {
	return &LocalWorkspace{
		Workdir: workdir,
	}
}

// Initialize 初始化工作空间（创建目录结构）
func (w *LocalWorkspace) Initialize(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 创建目录结构
	dirs := []string{
		filepath.Join(w.Workdir, "data"),
		filepath.Join(w.Workdir, "sessions"),
		filepath.Join(w.Workdir, "skills"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	w.initialized = true
	return nil
}

// Cleanup 清理工作空间
func (w *LocalWorkspace) Cleanup(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 不删除目录，只是标记为未初始化
	w.initialized = false
	return nil
}

// OffloadContext 持久化被压缩的消息
func (w *LocalWorkspace) OffloadContext(ctx context.Context, sessionID string, msgs []Msg) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.initialized {
		if err := w.Initialize(ctx); err != nil {
			return "", err
		}
	}

	// 创建 session 目录
	sessionDir := filepath.Join(w.Workdir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return "", fmt.Errorf("create session directory: %w", err)
	}

	// 写入 context.jsonl（追加方式）
	contextFile := filepath.Join(sessionDir, "context.jsonl")

	// 如果文件已存在，追加；否则创建
	f, err := os.OpenFile(contextFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("open context file: %w", err)
	}
	defer f.Close()

	for _, msg := range msgs {
		// 序列化消息为 JSON
		jsonBytes, err := json.Marshal(msg)
		if err != nil {
			continue // 跳过无法序列化的消息
		}
		if _, err := f.WriteString(string(jsonBytes) + "\n"); err != nil {
			return "", fmt.Errorf("write context: %w", err)
		}
	}

	return contextFile, nil
}

// SaveSummary 持久化压缩摘要到 session 目录
func (w *LocalWorkspace) SaveSummary(ctx context.Context, sessionID string, summary *Summary) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.initialized {
		if err := w.Initialize(ctx); err != nil {
			return err
		}
	}

	sessionDir := filepath.Join(w.Workdir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}

	summaryFile := filepath.Join(sessionDir, "summary.json")
	data, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal summary: %w", err)
	}
	return os.WriteFile(summaryFile, data, 0644)
}

// OffloadToolResult 持久化被截断的工具结果
func (w *LocalWorkspace) OffloadToolResult(ctx context.Context, sessionID string, result ContentBlock) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.initialized {
		if err := w.Initialize(ctx); err != nil {
			return "", err
		}
	}

	// 创建 session 目录
	sessionDir := filepath.Join(w.Workdir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return "", fmt.Errorf("create session directory: %w", err)
	}

	// 提取工具结果文本
	var textContent string
	for _, block := range result.ToolResultOutput {
		if block.Type == BlockTypeText {
			textContent += block.Text
		} else if block.Type == BlockTypeData {
			// 数据块写入 data/ 目录（按内容哈希去重）
			if block.Source != nil {
				dataRef, err := w.writeDataFile(block.Source)
				if err != nil {
					continue
				}
				textContent += fmt.Sprintf("\n[data file: %s, type: %s]", dataRef, block.MediaType)
			}
		}
	}

	// 写入 tool_result-{id}.txt
	toolResultFile := filepath.Join(sessionDir, fmt.Sprintf("tool_result-%s.txt", result.ToolCallID))
	if err := os.WriteFile(toolResultFile, []byte(textContent), 0644); err != nil {
		return "", fmt.Errorf("write tool result: %w", err)
	}

	return toolResultFile, nil
}

// writeDataFile 写入数据文件（按内容哈希去重）
func (w *LocalWorkspace) writeDataFile(source *DataSource) (string, error) {
	dataDir := filepath.Join(w.Workdir, "data")

	// 计算内容哈希（简化实现，使用时间戳作为唯一标识）
	// 实际应用中应使用 SHA-256
	var data string
	var ext string

	switch source.Type {
	case DataSourceBase64:
		data = source.Data
		switch source.MediaType {
		case "image/png":
			ext = ".png"
		case "image/jpeg":
			ext = ".jpg"
		case "image/gif":
			ext = ".gif"
		case "audio/mp3":
			ext = ".mp3"
		case "audio/wav":
			ext = ".wav"
		default:
			ext = ".bin"
		}
	case DataSourceURL:
		// URL 引用，直接返回 URL
		return source.URL, nil
	default:
		return "", fmt.Errorf("unsupported data source type: %s", source.Type)
	}

	// 生成文件名（简化：用时间戳）
	fileName := fmt.Sprintf("%s%s", nowISO(), ext)
	filePath := filepath.Join(dataDir, fileName)

	if err := os.WriteFile(filePath, []byte(data), 0644); err != nil {
		return "", fmt.Errorf("write data file: %w", err)
	}

	return filePath, nil
}

// =============================================
// NoOpOffloader 空 Offloader（不持久化，丢弃内容）
// =============================================

// NoOpOffloader 空操作 Offloader
type NoOpOffloader struct{}

// NewNoOpOffloader 创建空操作 Offloader
func NewNoOpOffloader() *NoOpOffloader {
	return &NoOpOffloader{}
}

func (o *NoOpOffloader) OffloadContext(ctx context.Context, sessionID string, msgs []Msg) (string, error) {
	return "", nil // 不持久化，直接丢弃
}

func (o *NoOpOffloader) OffloadToolResult(ctx context.Context, sessionID string, result ContentBlock) (string, error) {
	return "", nil // 不持久化，直接丢弃
}

func (o *NoOpOffloader) SaveSummary(ctx context.Context, sessionID string, summary *Summary) error {
	return nil // 不持久化
}

func (o *NoOpOffloader) Initialize(ctx context.Context) error {
	return nil
}

func (o *NoOpOffloader) Cleanup(ctx context.Context) error {
	return nil
}

// =============================================
// 辅助：从 LocalWorkspace 读取 offloaded 内容
// =============================================

// LoadOffloadedContext 从 LocalWorkspace 加载已 offload 的上下文
func (w *LocalWorkspace) LoadOffloadedContext(sessionID string) ([]Msg, error) {
	sessionDir := filepath.Join(w.Workdir, "sessions", sessionID)
	contextFile := filepath.Join(sessionDir, "context.jsonl")

	// 检查文件是否存在
	if _, err := os.Stat(contextFile); os.IsNotExist(err) {
		return nil, nil
	}

	// 读取文件
	content, err := os.ReadFile(contextFile)
	if err != nil {
		return nil, fmt.Errorf("read context file: %w", err)
	}

	// 按行解析 JSON
	lines := strings.Split(string(content), "\n")
	msgs := make([]Msg, 0)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var msg Msg
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // 跳过无法解析的行
		}
		msgs = append(msgs, msg)
	}

	return msgs, nil
}

// LoadOffloadedToolResult 从 LocalWorkspace 加载已 offload 的工具结果
func (w *LocalWorkspace) LoadOffloadedToolResult(sessionID, toolCallID string) (string, error) {
	sessionDir := filepath.Join(w.Workdir, "sessions", sessionID)
	toolResultFile := filepath.Join(sessionDir, fmt.Sprintf("tool_result-%s.txt", toolCallID))

	// 检查文件是否存在
	if _, err := os.Stat(toolResultFile); os.IsNotExist(err) {
		return "", fmt.Errorf("tool result file not found")
	}

	content, err := os.ReadFile(toolResultFile)
	if err != nil {
		return "", fmt.Errorf("read tool result: %w", err)
	}

	return string(content), nil
}

// LoadSummary 从 LocalWorkspace 加载已持久化的摘要
func (w *LocalWorkspace) LoadSummary(sessionID string) (*Summary, error) {
	sessionDir := filepath.Join(w.Workdir, "sessions", sessionID)
	summaryFile := filepath.Join(sessionDir, "summary.json")

	if _, err := os.Stat(summaryFile); os.IsNotExist(err) {
		return nil, nil
	}

	data, err := os.ReadFile(summaryFile)
	if err != nil {
		return nil, fmt.Errorf("read summary file: %w", err)
	}

	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("parse summary: %w", err)
	}
	return &summary, nil
}
