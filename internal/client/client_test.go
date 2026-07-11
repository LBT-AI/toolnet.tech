package client

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/LBT-AI/toolnet.tech/internal/config"
)

// fakeClient records calls and returns a preset result/error per call index.
// On success it returns its Tag so tests can tell which account served.
type fakeClient struct {
	mu    sync.Mutex
	calls int
	failN int // number of leading calls that should fail
	tag   string
}

func (f *fakeClient) Call(ctx context.Context, _, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := f.calls
	f.calls++
	if idx < f.failN {
		return "", fmt.Errorf("account %d failed", idx)
	}
	return f.tag, nil
}

func newConfig(provider string) config.AgentConfig {
	return config.AgentConfig{Provider: provider, Model: "m", Endpoint: "https://x", APIKey: "k"}
}

func TestRotatingClient_SingleAccount(t *testing.T) {
	c, err := NewClient(newConfig("openai"), 0)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, ok := c.(*RotatingClient); ok {
		t.Fatal("expected a single adapter, not a RotatingClient, for one key")
	}
}

func newRC(sticky bool, weights []int, clients ...LLMClient) *RotatingClient {
	h := make([]bool, len(clients))
	w := make([]int, len(clients))
	for i := range h {
		h[i] = true
		if i < len(weights) {
			w[i] = weights[i]
		} else {
			w[i] = 1
		}
	}
	return &RotatingClient{clients: clients, healthy: h, weights: w, sticky: sticky, pinned: -1}
}

func TestRotatingClient_RoundRobin(t *testing.T) {
	rc := newRC(false, nil,
		&fakeClient{tag: "a0"},
		&fakeClient{tag: "a1"},
		&fakeClient{tag: "a2"},
	)
	for i := 0; i < 3; i++ {
		out, err := rc.Call(context.Background(), "s", "u")
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if want := fmt.Sprintf("a%d", i); out != want {
			t.Fatalf("call %d: got %q want %q (expected round-robin order)", i, out, want)
		}
	}
}

func TestRotatingClient_Weighted(t *testing.T) {
	// Account 0 has weight 2, account 1 weight 1 -> ~2:1 distribution.
	rc := newRC(false, []int{2, 1},
		&fakeClient{tag: "a0"},
		&fakeClient{tag: "a1"},
	)
	counts := map[string]int{}
	const n = 300
	for i := 0; i < n; i++ {
		out, err := rc.Call(context.Background(), "s", "u")
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		counts[out]++
	}
	if counts["a0"] <= counts["a1"] {
		t.Fatalf("expected weighted account (a0) to be used more: %v", counts)
	}
	// Allow generous slack: a0 should get the majority of traffic.
	if counts["a0"] < n/2 {
		t.Fatalf("weighted account under-selected: %v", counts)
	}
}

func TestRotatingClient_Failover(t *testing.T) {
	a0 := &fakeClient{failN: 100, tag: "a0"} // always fails
	a1 := &fakeClient{tag: "a1"}
	rc := newRC(false, nil, a0, a1)
	out, err := rc.Call(context.Background(), "s", "u")
	if err != nil {
		t.Fatalf("expected failover to succeed: %v", err)
	}
	if out != "a1" {
		t.Fatalf("got %q, want a1", out)
	}
	if a0.calls != 1 || a1.calls < 1 {
		t.Fatalf("expected account0 tried then account1, got calls %d/%d", a0.calls, a1.calls)
	}
}

func TestRotatingClient_SkipUnhealthy(t *testing.T) {
	a0 := &fakeClient{failN: 100, tag: "a0"} // permanently unhealthy
	a1 := &fakeClient{tag: "a1"}
	rc := newRC(false, nil, a0, a1)
	if _, err := rc.Call(context.Background(), "s", "u"); err != nil {
		t.Fatalf("call1: %v", err)
	}
	if _, err := rc.Call(context.Background(), "s", "u"); err != nil {
		t.Fatalf("call2: %v", err)
	}
	if a0.calls != 1 {
		t.Fatalf("unhealthy account should be skipped after first failure, got calls=%d", a0.calls)
	}
}

func TestRotatingClient_Sticky(t *testing.T) {
	a0 := &fakeClient{tag: "a0"}
	a1 := &fakeClient{tag: "a1"}
	rc := newRC(true, nil, a0, a1) // sticky
	first, err := rc.Call(context.Background(), "s", "u")
	if err != nil {
		t.Fatalf("call1: %v", err)
	}
	for i := 0; i < 5; i++ {
		out, err := rc.Call(context.Background(), "s", "u")
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if out != first {
			t.Fatalf("sticky: expected same account %q every call, got %q", first, out)
		}
	}
}

func TestRotatingClient_StickyFailsOver(t *testing.T) {
	a0 := &fakeClient{failN: 100, tag: "a0"} // dies, forcing unpin + failover
	a1 := &fakeClient{tag: "a1"}
	rc := newRC(true, nil, a0, a1)
	out, err := rc.Call(context.Background(), "s", "u")
	if err != nil {
		t.Fatalf("expected failover: %v", err)
	}
	if out != "a1" {
		t.Fatalf("sticky should fail over to a1, got %q", out)
	}
}

func TestRotatingClient_AllFail(t *testing.T) {
	rc := newRC(false, nil,
		&fakeClient{failN: 100, tag: "a0"},
		&fakeClient{failN: 100, tag: "a1"},
	)
	_, err := rc.Call(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected error when all accounts fail")
	}
}

func TestNewClientForRole_MultiProvider(t *testing.T) {
	role := config.RoleConfig{
		Pool: []config.AgentConfig{
			{Provider: "openai", Model: "m", Endpoint: "https://x", APIKey: "k1"},
			{Provider: "anthropic", Model: "m", Endpoint: "https://y", APIKey: "k2"},
			{Provider: "groq", Model: "m", Endpoint: "https://z", APIKey: "k3", Disabled: true},
		},
	}
	c, err := NewClientForRole(role, 0)
	if err != nil {
		t.Fatalf("NewClientForRole: %v", err)
	}
	rc, ok := c.(*RotatingClient)
	if !ok {
		t.Fatalf("expected *RotatingClient, got %T", c)
	}
	if len(rc.clients) != 2 {
		t.Fatalf("disabled account should be skipped, want 2 clients got %d", len(rc.clients))
	}
}
