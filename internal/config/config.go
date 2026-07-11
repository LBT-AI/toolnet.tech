// Package config handles loading and validating the multi-agent
// workflow configuration (config.yaml). It maps each Role (COO/PM/DEV/QA) to its
// own Provider, Model, API key and Endpoint, and resolves ${ENV_VAR}
// placeholders in api_key fields so real secrets never need to be
// committed to the yaml file.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// AgentConfig holds connection details for a single account (one provider
// key, or one entry in a role's pool).
type AgentConfig struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
	APIKey   string `yaml:"api_key"`
	// APIKeys is an optional list of accounts (API keys or tokens) for the
	// same provider/model. When more than one is supplied, the client
	// rotates through them round-robin (with automatic failover on error),
	// spreading load and quota across accounts -- similar to a router that
	// balances many keys behind one endpoint.
	APIKeys  []string `yaml:"api_keys"`
	Endpoint string   `yaml:"endpoint"`
	// MaxTokens caps the length of a single model response. 0 means
	// "use the provider default" (currently 4096 for Anthropic).
	MaxTokens int `yaml:"max_tokens"`
	// Weight is this account's share of the rotation (mirrors 9router's
	// priority): higher weight means it is selected more often. 0 falls
	// back to 1.
	Weight int `yaml:"weight"`
	// Disabled skips this account entirely (mirrors 9router's
	// isActive=false): it is never added to the rotation pool.
	Disabled bool `yaml:"disabled"`
}

// RoleConfig is what a single workflow role (COO/PM/DEV/QA) can be: either a
// single provider account, or a pool of accounts spanning one or more
// providers. The client rotates across every usable account in the pool
// (round-robin + failover), exactly like 9router's provider connections.
type RoleConfig struct {
	AgentConfig `yaml:",inline"`
	// Pool lists additional provider accounts for this role. Each entry may
	// be a different provider, model, or set of keys.
	Pool []AgentConfig `yaml:"pool"`
	// Sticky pins each role to a single account for the duration of a run
	// (mirrors 9router's stickyRoundRobinLimit): the first healthy account
	// chosen stays selected until it fails, instead of rotating every call.
	Sticky bool `yaml:"sticky"`
}

// Agents returns the flattened list of accounts that make up this role: the
// inline base account (when it has a provider set) followed by the pool.
func (r RoleConfig) Agents() []AgentConfig {
	var out []AgentConfig
	if r.Provider != "" {
		out = append(out, r.AgentConfig)
	}
	out = append(out, r.Pool...)
	return out
}

// WorkflowConfig holds pipeline-level tunables.
type WorkflowConfig struct {
	MaxRetries     int  `yaml:"max_retries"`
	StrictMode     bool `yaml:"strict_mode"`
	GitAutoBranch  bool `yaml:"git_auto_branch"`
	TimeoutSeconds int  `yaml:"timeout_seconds"`
}

// Config is the root configuration object loaded from config.yaml.
type Config struct {
	COO      RoleConfig     `yaml:"coo"`
	PM       RoleConfig     `yaml:"pm"`
	DEV      RoleConfig     `yaml:"dev"`
	QA       RoleConfig     `yaml:"qa"`
	Workflow WorkflowConfig `yaml:"workflow"`
}

var envPlaceholder = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// resolveEnv replaces ${VAR_NAME} occurrences with the value of the
// corresponding OS environment variable. If the variable is not set,
// the placeholder is left untouched and the caller (Validate) will
// catch the missing key later.
func resolveEnv(value string) string {
	return envPlaceholder.ReplaceAllStringFunc(value, func(match string) string {
		name := envPlaceholder.FindStringSubmatch(match)[1]
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return match
	})
}

// resolveEnvList applies resolveEnv to every entry of a string slice,
// dropping empty results (e.g. unresolved placeholders).
func resolveEnvList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		resolved := resolveEnv(v)
		if resolved != "" && !envPlaceholder.MatchString(resolved) {
			out = append(out, resolved)
		}
	}
	return out
}

// LoadConfig reads the yaml file at `filename`. If filename is empty,
// it defaults to $HOME/.lbt/config.yaml. When the default config file does
// not exist yet, a starter config is written there so the CLI boots on a
// fresh machine (API keys can be added later or via `toolnet login`).
func LoadConfig(filename string) (*Config, error) {
	useDefault := false
	if filename == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		filename = filepath.Join(home, ".lbt", "config.yaml")
		useDefault = true
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		if useDefault && os.IsNotExist(err) {
			if mkErr := os.MkdirAll(filepath.Dir(filename), 0o755); mkErr != nil {
				return nil, fmt.Errorf("create config dir: %w", mkErr)
			}
			def := DefaultConfig()
			out, mErr := yaml.Marshal(def)
			if mErr != nil {
				return nil, fmt.Errorf("marshal default config: %w", mErr)
			}
			if wErr := os.WriteFile(filename, out, 0o600); wErr != nil {
				return nil, fmt.Errorf("write default config %q: %w", filename, wErr)
			}
			data = out
		} else {
			return nil, fmt.Errorf("read config file %q: %w", filename, err)
		}
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml %q: %w", filename, err)
	}
	// Resolve ${ENV_VAR} placeholders for the inline base account and every
	// account in each role's pool.
	resolveRoleEnv(&cfg.COO)
	resolveRoleEnv(&cfg.PM)
	resolveRoleEnv(&cfg.DEV)
	resolveRoleEnv(&cfg.QA)
	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DefaultConfig returns a starter configuration with all four roles mapped to
// OpenAI's API (gpt-4o) but no API keys set. Keys can be supplied via the
// api_key/api_keys fields, ${ENV_VAR} placeholders, or `toolnet login`.
func DefaultConfig() *Config {
	return &Config{
		COO:      RoleConfig{AgentConfig: AgentConfig{Provider: "openai", Model: "gpt-4o", Endpoint: "https://api.openai.com/v1"}},
		PM:       RoleConfig{AgentConfig: AgentConfig{Provider: "openai", Model: "gpt-4o", Endpoint: "https://api.openai.com/v1"}},
		DEV:      RoleConfig{AgentConfig: AgentConfig{Provider: "openai", Model: "gpt-4o", Endpoint: "https://api.openai.com/v1"}},
		QA:       RoleConfig{AgentConfig: AgentConfig{Provider: "openai", Model: "gpt-4o", Endpoint: "https://api.openai.com/v1"}},
		Workflow: WorkflowConfig{MaxRetries: 3, GitAutoBranch: true, TimeoutSeconds: 30},
	}
}

// resolveRoleEnv resolves ${ENV_VAR} placeholders in the base account and
// every account of a role's pool.
func resolveRoleEnv(r *RoleConfig) {
	r.APIKey = resolveEnv(r.APIKey)
	r.APIKeys = resolveEnvList(r.APIKeys)
	for i := range r.Pool {
		r.Pool[i].APIKey = resolveEnv(r.Pool[i].APIKey)
		r.Pool[i].APIKeys = resolveEnvList(r.Pool[i].APIKeys)
	}
}

func applyDefaults(cfg *Config) {
	if cfg.Workflow.MaxRetries <= 0 {
		cfg.Workflow.MaxRetries = 3
	}
	if cfg.Workflow.TimeoutSeconds <= 0 {
		cfg.Workflow.TimeoutSeconds = 30
	}
}

// Validate ensures every role has at least one usable account with the
// minimum required fields, so failures surface at startup rather than
// mid-pipeline.
func (c *Config) Validate() error {
	roles := map[string]RoleConfig{
		"coo": c.COO,
		"pm":  c.PM,
		"dev": c.DEV,
		"qa":  c.QA,
	}
	for name, role := range roles {
		agents := role.Agents()
		if len(agents) == 0 {
			return fmt.Errorf("role %q: no provider configured", name)
		}
		for ai, agent := range agents {
			if agent.Disabled {
				continue
			}
			if agent.Provider == "" {
				return fmt.Errorf("role %q account %d: missing provider", name, ai)
			}
			if agent.Model == "" {
				return fmt.Errorf("role %q account %d: missing model", name, ai)
			}
			if agent.Endpoint == "" {
				return fmt.Errorf("role %q account %d: missing endpoint", name, ai)
			}
			// api_key/api_keys are resolved at runtime from the inline
			// field, ${ENV_VAR} placeholders, or stored OAuth credentials
			// via `toolnet login`. We no longer hard-fail here so the CLI
			// can boot and show its UI on a fresh machine; a missing key
			// surfaces as a clear error only when a role actually calls
			// the provider.
		}
	}
	return nil
}
