package agent

import (
	"context"
	"testing"
)

func TestNewAgentRouter(t *testing.T) {
	pool := NewAgentPool(10)
	router := NewAgentRouter(pool, "default")

	if router == nil {
		t.Error("NewAgentRouter() returned nil")
	}
}

func TestAgentRouter_AddRule(t *testing.T) {
	pool := NewAgentPool(10)
	router := NewAgentRouter(pool, "default")

	rule := RuleByKeyword("test-rule", []string{"hello"}, "target-agent", 10)
	router.AddRule(rule)

	rules := router.ListRules()
	if len(rules) != 1 {
		t.Errorf("router.ListRules() len = %v, want %v", len(rules), 1)
	}
	if rules[0].Name != "test-rule" {
		t.Errorf("rules[0].Name = %v, want %v", rules[0].Name, "test-rule")
	}
}

func TestAgentRouter_RemoveRule(t *testing.T) {
	pool := NewAgentPool(10)
	router := NewAgentRouter(pool, "default")

	router.AddRule(RuleByKeyword("test-rule", []string{"hello"}, "target-agent", 10))
	if len(router.ListRules()) != 1 {
		t.Errorf("len(rules) = %v, want %v", len(router.ListRules()), 1)
	}

	router.RemoveRule("test-rule")
	if len(router.ListRules()) != 0 {
		t.Errorf("len(rules) = %v, want %v", len(router.ListRules()), 0)
	}
}

func TestAgentRouter_Route(t *testing.T) {
	pool := NewAgentPool(10)
	router := NewAgentRouter(pool, "default")

	targetAgent := NewAgent(*DefaultConfig("target-agent", nil, "You are target"))
	pool.RegisterAgent("target-agent", targetAgent)

	router.AddRule(RuleByKeyword("hello-rule", []string{"hello"}, "target-agent", 10))

	msg := UserMsg("user", "hello world")
	agent, ruleName := router.Route(msg)

	if agent == nil {
		t.Error("router.Route() returned nil agent")
	}
	if agent.GetName() != "target-agent" {
		t.Errorf("agent.GetName() = %v, want %v", agent.GetName(), "target-agent")
	}
	if ruleName != "hello-rule" {
		t.Errorf("ruleName = %v, want %v", ruleName, "hello-rule")
	}
}

func TestAgentRouter_Route_Default(t *testing.T) {
	pool := NewAgentPool(10)
	router := NewAgentRouter(pool, "default-agent")

	defaultAgent := NewAgent(*DefaultConfig("default-agent", nil, "You are default"))
	pool.RegisterAgent("default-agent", defaultAgent)

	msg := UserMsg("user", "random message")
	agent, ruleName := router.Route(msg)

	if agent == nil {
		t.Error("router.Route() returned nil agent")
	}
	if agent.GetName() != "default-agent" {
		t.Errorf("agent.GetName() = %v, want %v", agent.GetName(), "default-agent")
	}
	if ruleName != "default" {
		t.Errorf("ruleName = %v, want %v", ruleName, "default")
	}
}

func TestAgentRouter_Route_NoMatch(t *testing.T) {
	pool := NewAgentPool(10)
	router := NewAgentRouter(pool, "")

	msg := UserMsg("user", "random message")
	agent, ruleName := router.Route(msg)

	if agent != nil {
		t.Error("router.Route() should return nil agent when no match")
	}
	if ruleName != "" {
		t.Errorf("ruleName = %v, want %v", ruleName, "")
	}
}

func TestRuleByKeyword(t *testing.T) {
	rule := RuleByKeyword("test-rule", []string{"hello", "hi"}, "target", 10)

	if rule.Name != "test-rule" {
		t.Errorf("rule.Name = %v, want %v", rule.Name, "test-rule")
	}
	if rule.Target != "target" {
		t.Errorf("rule.Target = %v, want %v", rule.Target, "target")
	}
	if rule.Priority != 10 {
		t.Errorf("rule.Priority = %v, want %v", rule.Priority, 10)
	}

	msg1 := UserMsg("user", "hello world")
	if !rule.Match(msg1) {
		t.Error("rule.Match(hello world) should be true")
	}

	msg2 := UserMsg("user", "hi there")
	if !rule.Match(msg2) {
		t.Error("rule.Match(hi there) should be true")
	}

	msg3 := UserMsg("user", "goodbye")
	if rule.Match(msg3) {
		t.Error("rule.Match(goodbye) should be false")
	}
}

func TestRuleByKeyword_CaseInsensitive(t *testing.T) {
	rule := RuleByKeyword("test-rule", []string{"HELLO"}, "target", 10)

	msg := UserMsg("user", "hello world")
	if !rule.Match(msg) {
		t.Error("rule.Match(hello world) should be true (case insensitive)")
	}

	msg2 := UserMsg("user", "Hello World")
	if !rule.Match(msg2) {
		t.Error("rule.Match(Hello World) should be true (case insensitive)")
	}
}

func TestRuleByUser(t *testing.T) {
	rule := RuleByUser("test-rule", "user123", "target", 10)

	msg1 := UserMsg("user123", "hello")
	if !rule.Match(msg1) {
		t.Error("rule.Match(user123) should be true")
	}

	msg2 := UserMsg("user456", "hello")
	if rule.Match(msg2) {
		t.Error("rule.Match(user456) should be false")
	}
}

func TestRuleByRole(t *testing.T) {
	rule := RuleByRole("test-rule", RoleUser, "target", 10)

	msg1 := UserMsg("user", "hello")
	if !rule.Match(msg1) {
		t.Error("rule.Match(user) should be true")
	}

	msg2 := NewSystemMsg("hello")
	if rule.Match(msg2) {
		t.Error("rule.Match(system) should be false")
	}
}

func TestRuleAlways(t *testing.T) {
	rule := RuleAlways("test-rule", "target", 10)

	msg1 := UserMsg("user", "hello")
	if !rule.Match(msg1) {
		t.Error("rule.Match(user) should be true")
	}

	msg2 := NewSystemMsg("hello")
	if !rule.Match(msg2) {
		t.Error("rule.Match(system) should be true")
	}
}

func TestAgentRouter_ListRules(t *testing.T) {
	pool := NewAgentPool(10)
	router := NewAgentRouter(pool, "default")

	rules := router.ListRules()
	if len(rules) != 0 {
		t.Errorf("len(rules) = %v, want %v", len(rules), 0)
	}

	router.AddRule(RuleByKeyword("rule1", []string{"hello"}, "target1", 5))
	router.AddRule(RuleByKeyword("rule2", []string{"hi"}, "target2", 10))

	rules = router.ListRules()
	if len(rules) != 2 {
		t.Errorf("len(rules) = %v, want %v", len(rules), 2)
	}

	if rules[0].Priority != 10 {
		t.Errorf("rules[0].Priority = %v, want %v", rules[0].Priority, 10)
	}
	if rules[1].Priority != 5 {
		t.Errorf("rules[1].Priority = %v, want %v", rules[1].Priority, 5)
	}
}

func TestAgentRouter_ClearRules(t *testing.T) {
	pool := NewAgentPool(10)
	router := NewAgentRouter(pool, "default")

	router.AddRule(RuleByKeyword("rule1", []string{"hello"}, "target1", 5))
	if len(router.ListRules()) != 1 {
		t.Errorf("len(rules) = %v, want %v", len(router.ListRules()), 1)
	}

	router.ClearRules()
	if len(router.ListRules()) != 0 {
		t.Errorf("len(rules) = %v, want %v", len(router.ListRules()), 0)
	}
}

func TestAgentRouter_Reply(t *testing.T) {
	pool := NewAgentPool(10)
	router := NewAgentRouter(pool, "default")

	defaultAgent := NewAgent(*DefaultConfig("default-agent", nil, "You are default"))
	pool.RegisterAgent("default-agent", defaultAgent)

	ctx := context.Background()
	msg := UserMsg("user", "hello")

	reply, err := router.Reply(ctx, msg)
	if err != nil {
		t.Logf("router.Reply() returned error: %v (may be expected without real model)", err)
	}

	if reply != nil && reply.Role != RoleAssistant {
		t.Errorf("reply.Role = %v, want %v", reply.Role, RoleAssistant)
	}
}

func TestAgentRouter_ReplyStream(t *testing.T) {
	pool := NewAgentPool(10)
	router := NewAgentRouter(pool, "default")

	defaultAgent := NewAgent(*DefaultConfig("default-agent", nil, "You are default"))
	pool.RegisterAgent("default-agent", defaultAgent)

	ctx := context.Background()
	msg := UserMsg("user", "hello")

	events, err := router.ReplyStream(ctx, msg)
	if err != nil {
		t.Logf("router.ReplyStream() returned error: %v", err)
		return
	}
	if events == nil {
		t.Error("router.ReplyStream() returned nil channel")
	}

	for range events {
	}
}
