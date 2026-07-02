package agent

import (
	"testing"
)

func TestNewAgentPool(t *testing.T) {
	pool := NewAgentPool(10)

	if pool == nil {
		t.Error("NewAgentPool() returned nil")
	}
	if pool.Count() != 0 {
		t.Errorf("pool.Count() = %v, want %v", pool.Count(), 0)
	}
}

func TestAgentPool_RegisterAgent(t *testing.T) {
	pool := NewAgentPool(10)
	agent := NewAgent(*DefaultConfig("test-agent", nil, "You are a test assistant"))

	pool.RegisterAgent("test-agent", agent)

	if pool.Count() != 1 {
		t.Errorf("pool.Count() = %v, want %v", pool.Count(), 1)
	}

	retrieved := pool.Get("test-agent")
	if retrieved == nil {
		t.Error("pool.Get() returned nil")
	}
	if retrieved.GetName() != "test-agent" {
		t.Errorf("retrieved.GetName() = %v, want %v", retrieved.GetName(), "test-agent")
	}
}

func TestAgentPool_Get(t *testing.T) {
	pool := NewAgentPool(10)

	if pool.Get("nonexistent") != nil {
		t.Error("pool.Get(nonexistent) should return nil")
	}

	agent := NewAgent(*DefaultConfig("test-agent", nil, "You are a test assistant"))
	pool.RegisterAgent("test-agent", agent)

	if pool.Get("test-agent") == nil {
		t.Error("pool.Get(test-agent) should not return nil")
	}
}

func TestAgentPool_GetOrCreate(t *testing.T) {
	pool := NewAgentPool(10)

	agent, err := pool.GetOrCreate("nonexistent")
	if err == nil {
		t.Error("pool.GetOrCreate(nonexistent) should return error")
	}
	if agent != nil {
		t.Error("pool.GetOrCreate(nonexistent) should return nil agent")
	}

	pool.RegisterCreator("creatable", func() (*Agent, error) {
		return NewAgent(*DefaultConfig("creatable", nil, "You are a test assistant")), nil
	})

	agent, err = pool.GetOrCreate("creatable")
	if err != nil {
		t.Errorf("pool.GetOrCreate(creatable) error = %v", err)
	}
	if agent == nil {
		t.Error("pool.GetOrCreate(creatable) should return agent")
	}
	if agent.GetName() != "creatable" {
		t.Errorf("agent.GetName() = %v, want %v", agent.GetName(), "creatable")
	}
}

func TestAgentPool_List(t *testing.T) {
	pool := NewAgentPool(10)

	agents := pool.List()
	if len(agents) != 0 {
		t.Errorf("pool.List() len = %v, want %v", len(agents), 0)
	}

	pool.RegisterAgent("agent1", NewAgent(*DefaultConfig("agent1", nil, "You are agent1")))
	pool.RegisterAgent("agent2", NewAgent(*DefaultConfig("agent2", nil, "You are agent2")))

	agents = pool.List()
	if len(agents) != 2 {
		t.Errorf("pool.List() len = %v, want %v", len(agents), 2)
	}
}

func TestAgentPool_Remove(t *testing.T) {
	pool := NewAgentPool(10)

	pool.RegisterAgent("test-agent", NewAgent(*DefaultConfig("test-agent", nil, "You are a test assistant")))
	if pool.Count() != 1 {
		t.Errorf("pool.Count() = %v, want %v", pool.Count(), 1)
	}

	pool.Remove("test-agent")
	if pool.Count() != 0 {
		t.Errorf("pool.Count() = %v, want %v", pool.Count(), 0)
	}
	if pool.Get("test-agent") != nil {
		t.Error("pool.Get(test-agent) should return nil after removal")
	}
}

func TestAgentPool_Clear(t *testing.T) {
	pool := NewAgentPool(10)

	pool.RegisterAgent("agent1", NewAgent(*DefaultConfig("agent1", nil, "You are agent1")))
	pool.RegisterAgent("agent2", NewAgent(*DefaultConfig("agent2", nil, "You are agent2")))
	if pool.Count() != 2 {
		t.Errorf("pool.Count() = %v, want %v", pool.Count(), 2)
	}

	pool.Clear()
	if pool.Count() != 0 {
		t.Errorf("pool.Count() = %v, want %v", pool.Count(), 0)
	}
}

func TestAgentPool_Count(t *testing.T) {
	pool := NewAgentPool(10)

	if pool.Count() != 0 {
		t.Errorf("pool.Count() = %v, want %v", pool.Count(), 0)
	}

	pool.RegisterAgent("agent1", NewAgent(*DefaultConfig("agent1", nil, "You are agent1")))
	if pool.Count() != 1 {
		t.Errorf("pool.Count() = %v, want %v", pool.Count(), 1)
	}

	pool.RegisterAgent("agent2", NewAgent(*DefaultConfig("agent2", nil, "You are agent2")))
	if pool.Count() != 2 {
		t.Errorf("pool.Count() = %v, want %v", pool.Count(), 2)
	}
}

func TestAgentPool_Stats(t *testing.T) {
	pool := NewAgentPool(10)

	stats := pool.Stats()
	if stats.TotalAgents != 0 {
		t.Errorf("stats.TotalAgents = %v, want %v", stats.TotalAgents, 0)
	}
	if stats.MaxAgents != 10 {
		t.Errorf("stats.MaxAgents = %v, want %v", stats.MaxAgents, 10)
	}
	if stats.Registered != 0 {
		t.Errorf("stats.Registered = %v, want %v", stats.Registered, 0)
	}

	pool.RegisterAgent("agent1", NewAgent(*DefaultConfig("agent1", nil, "You are agent1")))
	pool.RegisterCreator("creator1", func() (*Agent, error) { return nil, nil })

	stats = pool.Stats()
	if stats.TotalAgents != 1 {
		t.Errorf("stats.TotalAgents = %v, want %v", stats.TotalAgents, 1)
	}
	if stats.Registered != 1 {
		t.Errorf("stats.Registered = %v, want %v", stats.Registered, 1)
	}
}

func TestAgentPool_Broadcast(t *testing.T) {
	pool := NewAgentPool(10)

	agent1 := NewAgent(*DefaultConfig("agent1", nil, "You are agent1"))
	agent2 := NewAgent(*DefaultConfig("agent2", nil, "You are agent2"))

	pool.RegisterAgent("agent1", agent1)
	pool.RegisterAgent("agent2", agent2)

	msg := UserMsg("test-user", "Hello")

	pool.Broadcast(msg)

	if len(agent1.GetSession().GetHistory()) != 1 {
		t.Errorf("agent1 session length = %v, want %v", len(agent1.GetSession().GetHistory()), 1)
	}
	if len(agent2.GetSession().GetHistory()) != 1 {
		t.Errorf("agent2 session length = %v, want %v", len(agent2.GetSession().GetHistory()), 1)
	}
}

func TestAgentPool_HealthCheck(t *testing.T) {
	pool := NewAgentPool(10)

	pool.RegisterAgent("agent1", NewAgent(*DefaultConfig("agent1", nil, "You are agent1")))
	pool.RegisterAgent("agent2", NewAgent(*DefaultConfig("agent2", nil, "You are agent2")))

	health := pool.HealthCheck()
	if len(health) != 2 {
		t.Errorf("len(health) = %v, want %v", len(health), 2)
	}
	if !health["agent1"] {
		t.Error("health[agent1] should be true")
	}
	if !health["agent2"] {
		t.Error("health[agent2] should be true")
	}
}
