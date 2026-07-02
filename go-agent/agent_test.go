package agent

import (
	"context"
	"testing"
)

func TestNewAgent(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "minimal config",
			config:  *DefaultConfig("test", nil, "You are a test assistant"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ag := NewAgent(tt.config)
			if ag == nil {
				t.Error("NewAgent() returned nil")
			}
			if ag.GetName() != tt.config.Name {
				t.Errorf("GetName() = %v, want %v", ag.GetName(), tt.config.Name)
			}
		})
	}
}

func TestAgent_Reply(t *testing.T) {
	ag := NewAgent(*DefaultConfig("test", nil, "You are a test assistant"))

	ctx := context.Background()
	msg := UserMsg("test-user", "Hello")

	reply, err := ag.Reply(ctx, msg)
	if err != nil {
		t.Logf("Reply() returned error: %v (may be expected without real model)", err)
	}

	if reply != nil {
		if reply.Role != RoleAssistant {
			t.Errorf("reply.Role = %v, want %v", reply.Role, RoleAssistant)
		}
	}
}

func TestAgent_Observe(t *testing.T) {
	ag := NewAgent(*DefaultConfig("test", nil, "You are a test assistant"))

	msg := UserMsg("test-user", "Hello")
	ag.Observe(msg)
}

func TestAgent_GetName(t *testing.T) {
	tests := []struct {
		name     string
		agent    *Agent
		expected string
	}{
		{
			name:     "named agent",
			agent:    NewAgent(*DefaultConfig("my-agent", nil, "You are a test assistant")),
			expected: "my-agent",
		},
		{
			name:     "empty name",
			agent:    NewAgent(*DefaultConfig("", nil, "You are a test assistant")),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.agent.GetName(); got != tt.expected {
				t.Errorf("GetName() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAgent_GetSessionID(t *testing.T) {
	ag := NewAgent(*DefaultConfig("test", nil, "You are a test assistant"))

	sessionID := ag.GetSession().GetID()
	if sessionID == "" {
		t.Error("GetSession().GetID() returned empty string")
	}
}

func TestUserMsg(t *testing.T) {
	tests := []struct {
		name     string
		userName string
		text     string
	}{
		{
			name:     "simple message",
			userName: "user1",
			text:     "Hello world",
		},
		{
			name:     "empty text",
			userName: "user2",
			text:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := UserMsg(tt.userName, tt.text)
			if msg.Name != tt.userName {
				t.Errorf("msg.Name = %v, want %v", msg.Name, tt.userName)
			}
			if msg.Role != RoleUser {
				t.Errorf("msg.Role = %v, want %v", msg.Role, RoleUser)
			}
			if msg.GetTextContent() != tt.text {
				t.Errorf("msg.GetTextContent() = %v, want %v", msg.GetTextContent(), tt.text)
			}
		})
	}
}

func TestSystemMsg(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{
			name: "system prompt",
			text: "You are a helpful assistant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewSystemMsg(tt.text)
			if msg.Role != RoleSystem {
				t.Errorf("msg.Role = %v, want %v", msg.Role, RoleSystem)
			}
			if msg.GetTextContent() != tt.text {
				t.Errorf("msg.GetTextContent() = %v, want %v", msg.GetTextContent(), tt.text)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("test-agent", nil, "You are a test assistant")

	if cfg.Name != "test-agent" {
		t.Errorf("cfg.Name = %v, want %v", cfg.Name, "test-agent")
	}
	if cfg.SystemPrompt != "You are a test assistant" {
		t.Errorf("cfg.SystemPrompt = %v, want %v", cfg.SystemPrompt, "You are a test assistant")
	}
	if cfg.ReActConfig == nil {
		t.Error("cfg.ReActConfig is nil")
	}
	if cfg.ContextConfig == nil {
		t.Error("cfg.ContextConfig is nil")
	}
}

func TestConfig_BuilderMethods(t *testing.T) {
	cfg := DefaultConfig("test", nil, "You are a test assistant")

	result := cfg.
		WithMaxIters(5).
		WithMiddlewares().
		WithMemory(nil).
		WithSkills(nil)

	if result.ReActConfig.MaxIters != 5 {
		t.Errorf("ReActConfig.MaxIters = %v, want %v", result.ReActConfig.MaxIters, 5)
	}
}

func TestGenerateID(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
	}{
		{
			name:   "msg prefix",
			prefix: "msg",
		},
		{
			name:   "evt prefix",
			prefix: "evt",
		},
		{
			name:   "reply prefix",
			prefix: "reply",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := generateID(tt.prefix)
			if id == "" {
				t.Error("generateID() returned empty string")
			}
			if len(id) < len(tt.prefix)+1 {
				t.Errorf("generateID() returned too short ID: %v", id)
			}
		})
	}
}

func TestGenerateID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := generateID("test")
		if ids[id] {
			t.Errorf("generateID() produced duplicate ID: %v", id)
		}
		ids[id] = true
	}
}

func TestSession_AddMessage(t *testing.T) {
	session := NewSession(nil)

	msg := UserMsg("user", "Hello")
	session.AddMessage(msg)

	if len(session.GetHistory()) != 1 {
		t.Errorf("len(session.GetHistory()) = %v, want %v", len(session.GetHistory()), 1)
	}
}

func TestSession_Count(t *testing.T) {
	session := NewSession(nil)

	if len(session.GetHistory()) != 0 {
		t.Errorf("len(session.GetHistory()) = %v, want %v", len(session.GetHistory()), 0)
	}

	session.AddMessage(UserMsg("user", "Hello"))
	if len(session.GetHistory()) != 1 {
		t.Errorf("len(session.GetHistory()) = %v, want %v", len(session.GetHistory()), 1)
	}

	session.AddMessage(UserMsg("user", "Hi"))
	if len(session.GetHistory()) != 2 {
		t.Errorf("len(session.GetHistory()) = %v, want %v", len(session.GetHistory()), 2)
	}
}

func TestSession_Clear(t *testing.T) {
	session := NewSession(nil)

	session.AddMessage(UserMsg("user", "Hello"))
	if len(session.GetHistory()) != 1 {
		t.Errorf("len(session.GetHistory()) = %v, want %v", len(session.GetHistory()), 1)
	}

	session.Clear()
	if len(session.GetHistory()) != 0 {
		t.Errorf("len(session.GetHistory()) = %v, want %v", len(session.GetHistory()), 0)
	}
}
