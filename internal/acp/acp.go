package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	glog "go-claw/pkg/log"
)

// SessionState 会话状态
type SessionState string

const (
	StateStarting SessionState = "starting"
	StateReady    SessionState = "ready"
	StateBusy     SessionState = "busy"
	StateClosed   SessionState = "closed"
)

// Message ACP 消息
type Message struct {
	Type      string                 `json:"type"`       // "request", "response", "event"
	ID        string                 `json:"id"`         // 消息 ID
	Method    string                 `json:"method"`     // 方法名
	Params    map[string]interface{} `json:"params"`     // 参数
	Result    interface{}            `json:"result"`     // 结果
	Error     string                 `json:"error"`      // 错误信息
	Timestamp time.Time              `json:"timestamp"`  // 时间戳
}

// Session 外部 Agent 会话
type Session struct {
	ID       string
	Name     string        // Agent 名称 (claude, codex, etc.)
	Command  string        // 启动命令
	State    SessionState
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.Reader
	mu       sync.RWMutex
	pending  map[string]chan *Message // 等待响应的消息
}

// Service ACP 服务
type Service struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewService 创建 ACP 服务
func NewService() *Service {
	return &Service{
		sessions: make(map[string]*Session),
	}
}

// StartSession 启动外部 Agent 会话
func (s *Service) StartSession(ctx context.Context, name, command string) (*Session, error) {
	logger := glog.Logger()
	logger.Info("[ACP] 启动外部 Agent", "name", name, "command", command)

	id := fmt.Sprintf("session_%s_%d", name, time.Now().UnixNano())

	session := &Session{
		ID:      id,
		Name:    name,
		Command: command,
		State:   StateStarting,
		pending: make(map[string]chan *Message),
	}

	// 启动进程
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 stdin 管道失败: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 stdout 管道失败: %v", err)
	}

	session.cmd = cmd
	session.stdin = stdin
	session.stdout = stdout

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动进程失败: %v", err)
	}

	session.State = StateReady

	// 启动消息读取协程
	go s.readMessages(session)

	s.mu.Lock()
	s.sessions[id] = session
	s.mu.Unlock()

	logger.Info("[ACP] 外部 Agent 已启动", "id", id, "name", name)
	return session, nil
}

// readMessages 读取消息协程
func (s *Service) readMessages(session *Session) {
	scanner := bufio.NewScanner(session.stdout)
	for scanner.Scan() {
		line := scanner.Text()
		var msg Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			glog.Logger().Warn("[ACP] 解析消息失败", "err", err, "line", line)
			continue
		}

		// 处理响应
		session.mu.RLock()
		if ch, exists := session.pending[msg.ID]; exists {
			ch <- &msg
		}
		session.mu.RUnlock()
	}
}

// SendMessage 发送消息
func (s *Service) SendMessage(ctx context.Context, sessionID string, msg *Message) (*Message, error) {
	s.mu.RLock()
	session, exists := s.sessions[sessionID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	if session.State != StateReady {
		return nil, fmt.Errorf("会话状态不正确: %s", session.State)
	}

	msg.ID = fmt.Sprintf("msg_%d", time.Now().UnixNano())
	msg.Timestamp = time.Now()

	// 创建响应通道
	respChan := make(chan *Message, 1)
	session.mu.Lock()
	session.pending[msg.ID] = respChan
	session.State = StateBusy
	session.mu.Unlock()

	defer func() {
		session.mu.Lock()
		delete(session.pending, msg.ID)
		session.State = StateReady
		session.mu.Unlock()
	}()

	// 发送消息
	data, _ := json.Marshal(msg)
	session.stdin.Write(data)
	session.stdin.Write([]byte("\n"))

	// 等待响应
	select {
	case resp := <-respChan:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(60 * time.Second):
		return nil, fmt.Errorf("等待响应超时")
	}
}

// CloseSession 关闭会话
func (s *Service) CloseSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil
	}

	if session.cmd != nil && session.cmd.Process != nil {
		session.cmd.Process.Signal(os.Interrupt)
		session.cmd.Wait()
	}

	session.State = StateClosed
	delete(s.sessions, sessionID)

	glog.Logger().Info("[ACP] 会话已关闭", "id", sessionID)
	return nil
}

// ListSessions 列出所有会话
func (s *Service) ListSessions() []SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var infos []SessionInfo
	for _, session := range s.sessions {
		infos = append(infos, SessionInfo{
			ID:    session.ID,
			Name:  session.Name,
			State: string(session.State),
		})
	}
	return infos
}

// SessionInfo 会话信息
type SessionInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}