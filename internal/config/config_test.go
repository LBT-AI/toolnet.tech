package config

import "testing"

func TestDefaultConfigUsesChatCompletionsEndpoint(t *testing.T) {
	cfg := DefaultConfig()
	for name, role := range map[string]RoleConfig{"coo": cfg.COO, "pm": cfg.PM, "dev": cfg.DEV, "qa": cfg.QA} {
		if role.Endpoint != "https://api.openai.com/v1/chat/completions" {
			t.Errorf("%s endpoint = %q", name, role.Endpoint)
		}
	}
}

func TestResolveRoleEnvDropsUnresolvedSecrets(t *testing.T) {
	t.Setenv("TOOLNET_PRESENT", "secret")
	r := RoleConfig{AgentConfig: AgentConfig{APIKey: "${TOOLNET_MISSING}"}, Pool: []AgentConfig{{APIKey: "${TOOLNET_PRESENT}"}}}
	resolveRoleEnv(&r)
	if r.APIKey != "" {
		t.Fatalf("unresolved placeholder was treated as a key: %q", r.APIKey)
	}
	if r.Pool[0].APIKey != "secret" {
		t.Fatalf("resolved key = %q", r.Pool[0].APIKey)
	}
}
