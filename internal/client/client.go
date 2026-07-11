// Package client implements the "Provider Adapter" pattern described in
// DESIGN.md section 5: a single LLMClient interface with one concrete
// implementation per API format (OpenAI-compatible, Anthropic, Google, Ollama, etc.).
// Adding a new provider (OpenRouter, Groq, Ollama, HuggingFace, ...) only
// requires a new struct implementing LLMClient --- workflow.go never changes.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/LBT-AI/toolnet.tech/internal/auth"
	"github.com/LBT-AI/toolnet.tech/internal/config"
)

// LLMClient is the common contract every provider adapter must satisfy.
type LLMClient interface {
	// Call sends a system + user prompt and returns the model's raw text reply.
	Call(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// defaultMaxTokens is used when an AgentConfig does not specify MaxTokens.
const defaultMaxTokens = 4096

// resolveMaxTokens returns the configured max tokens, or the provider default
// when unset.
func resolveMaxTokens(cfg config.AgentConfig) int {
	if cfg.MaxTokens > 0 {
		return cfg.MaxTokens
	}
	return defaultMaxTokens
}

// resolveAPIKeys returns the list of accounts (API keys / tokens) available
// for a role. It combines, in order:
//  1. agent.APIKey (single-key form, for backward compatibility)
//  2. agent.APIKeys (explicit multi-account list)
//  3. credentials stored via `toolnet login --provider <p>` (may be several)
//
// The result drives round-robin rotation in the client.
func resolveAPIKeys(agent config.AgentConfig) ([]string, error) {
	var keys []string
	if agent.APIKey != "" {
		keys = append(keys, agent.APIKey)
	}
	keys = append(keys, agent.APIKeys...)

	if len(keys) == 0 {
		store, err := auth.NewCredentialStore()
		if err != nil {
			return nil, fmt.Errorf("role %q: no api_key/api_keys and credential store unavailable: %w", agent.Provider, err)
		}
		creds, err := store.ListCredentials(agent.Provider)
		if err != nil {
			return nil, fmt.Errorf("role %q: no api_key/api_keys set and no stored credentials (run `toolnet login --provider %s`): %w",
				agent.Provider, agent.Provider, err)
		}
		for _, c := range creds {
			keys = append(keys, c.AccessToken)
		}
	}
	return keys, nil
}

// NewClient is a factory that returns the correct adapter for the given
// agent configuration. When more than one account is available (via
// api_keys or multiple stored logins), it returns a RotatingClient that
// spreads calls round-robin across them with automatic failover. Unknown
// providers return an error so misconfiguration is caught at startup.
func NewClient(agent config.AgentConfig, timeout time.Duration) (LLMClient, error) {
	return NewClientForRole(config.RoleConfig{AgentConfig: agent}, timeout)
}

// NewClientForRole builds the client for a workflow role. It flattens the
// role's base account and its pool (which may span multiple providers) into
// a single rotation pool: one underlying adapter per (account, key). When
// only one usable account exists, it returns that adapter directly;
// otherwise it returns a RotatingClient that load-balances and fails over
// across them, skipping accounts that are disabled or become unhealthy.
func NewClientForRole(role config.RoleConfig, timeout time.Duration) (LLMClient, error) {
	var clients []LLMClient
	var weights []int
	for _, agent := range role.Agents() {
		if agent.Disabled {
			continue
		}
		w := agent.Weight
		if w < 1 {
			w = 1
		}
		keys, err := resolveAPIKeys(agent)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			a := agent
			a.APIKey = k
			c, err := buildAdapter(a, timeout)
			if err != nil {
				return nil, err
			}
			clients = append(clients, c)
			weights = append(weights, w)
		}
	}
	if len(clients) == 0 {
		return nil, fmt.Errorf("role has no usable accounts (all disabled or missing keys)")
	}
	if len(clients) == 1 {
		return clients[0], nil
	}
	healthy := make([]bool, len(clients))
	for i := range healthy {
		healthy[i] = true
	}
	return &RotatingClient{
		clients: clients,
		healthy: healthy,
		weights: weights,
		sticky:  role.Sticky,
		pinned:  -1,
	}, nil
}

// buildAdapter constructs a single-provider adapter (no rotation) for an
// agent that already has exactly one API key resolved.
func buildAdapter(agent config.AgentConfig, timeout time.Duration) (LLMClient, error) {
	httpClient := &http.Client{Timeout: timeout}
	switch agent.Provider {
	case "openai", "deepseek", "openrouter", "groq":
		// These all speak the OpenAI chat-completions wire format.
		return &OpenAIClient{cfg: agent, http: httpClient}, nil
	case "anthropic":
		return &AnthropicClient{cfg: agent, http: httpClient}, nil
	case "google":
		return &GoogleClient{cfg: agent, http: httpClient}, nil
	case "ollama":
		return &OllamaClient{cfg: agent, http: httpClient}, nil
	case "huggingface":
		return &HuggingFaceClient{cfg: agent, http: httpClient}, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q (add an adapter in internal/client)", agent.Provider)
	}
}

// RotatingClient wraps several underlying LLM clients (one per account,
// possibly across multiple providers) and spreads calls across them. It
// combines 9router's behaviours:
//   - weighted round-robin: an account's Weight controls how often it is
//     selected (higher priority = more traffic);
//   - health skip: an account that errors is marked unhealthy and skipped;
//   - failover: on error it transparently tries the next healthy account;
//   - sticky: when enabled, the first healthy account chosen stays pinned
//     for the rest of the run (stickyRoundRobinLimit-style) until it fails.
type RotatingClient struct {
	mu      sync.Mutex
	clients []LLMClient
	healthy []bool
	weights []int
	sticky  bool
	pinned  int
	counter uint64
	order   []int
	dirty   bool
}

// selectIndex picks the next account under lock, honouring weights, health
// and stickiness. It returns an error only when no healthy account exists.
func (r *RotatingClient) selectIndex() (int, error) {
	if r.sticky && r.pinned >= 0 && r.healthy[r.pinned] {
		return r.pinned, nil
	}
	order := r.buildOrder()
	if len(order) == 0 {
		return 0, fmt.Errorf("no healthy accounts")
	}
	idx := order[int(r.counter)%len(order)]
	r.counter++
	if r.sticky {
		r.pinned = idx
	}
	return idx, nil
}

// buildOrder returns the cached weighted ordering of healthy account
// indices, rebuilding it when health changed. Caller must hold r.mu.
func (r *RotatingClient) buildOrder() []int {
	if !r.dirty && r.order != nil {
		return r.order
	}
	order := make([]int, 0, len(r.clients))
	for i, h := range r.healthy {
		if !h {
			continue
		}
		w := r.weights[i]
		if w < 1 {
			w = 1
		}
		for j := 0; j < w; j++ {
			order = append(order, i)
		}
	}
	r.order = order
	r.dirty = false
	return order
}

func (r *RotatingClient) markUnhealthy(idx int) {
	r.mu.Lock()
	r.healthy[idx] = false
	r.pinned = -1
	r.dirty = true
	r.mu.Unlock()
}

func (r *RotatingClient) resetHealth() {
	r.mu.Lock()
	for i := range r.healthy {
		r.healthy[i] = true
	}
	r.pinned = -1
	r.dirty = true
	r.mu.Unlock()
}

// Call invokes the next healthy account (weighted, sticky-aware), failing
// over to the remaining accounts on error. It returns the first successful
// response, or the last error seen after trying every account. Health is
// reset between passes so temporarily-failed accounts get one more chance,
// but the number of passes is bounded (2) so permanently-failing pools
// return instead of looping forever.
func (r *RotatingClient) Call(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	var lastErr error
	n := len(r.clients)
	for round := 0; round < 2; round++ {
		for i := 0; i < n; i++ {
			r.mu.Lock()
			idx, err := r.selectIndex()
			r.mu.Unlock()
			if err != nil {
				break // no healthy account left in this pass
			}
			out, callErr := r.clients[idx].Call(ctx, systemPrompt, userPrompt)
			if callErr == nil {
				return out, nil
			}
			lastErr = callErr
			r.markUnhealthy(idx)
		}
		r.resetHealth()
	}
	return "", fmt.Errorf("all %d accounts failed: %w", n, lastErr)
}

// doRequest is a small shared helper for issuing an HTTP POST and reading
// the raw body, with basic status-code checking. Each adapter is
// responsible for building its own payload/headers and parsing its own
// response shape, since those differ per provider.
func doRequest(ctx context.Context, httpClient *http.Client, method, url string, headers map[string]string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http call to %s: %w", url, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("api error: status %d, body: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

// ------------------------------------------------------------------
// OpenAI-compatible adapter (OpenAI, DeepSeek, OpenRouter, Groq, ...)
// ------------------------------------------------------------------

type OpenAIClient struct {
	cfg  config.AgentConfig
	http *http.Client
}

type openAIRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *OpenAIClient) Call(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	payload := openAIRequest{
		Model: c.cfg.Model,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens: resolveMaxTokens(c.cfg),
	}
	headers := map[string]string{
		"Authorization": "Bearer " + c.cfg.APIKey,
	}
	raw, err := doRequest(ctx, c.http, http.MethodPost, c.cfg.Endpoint, headers, payload)
	if err != nil {
		return "", err
	}
	var parsed openAIResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("parse openai-compatible response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("provider %s returned error: %s", c.cfg.Provider, parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("provider %s returned no choices", c.cfg.Provider)
	}
	return parsed.Choices[0].Message.Content, nil
}

// ------------------------------------------------------------------
// Anthropic adapter (/v1/messages)
// ------------------------------------------------------------------

type AnthropicClient struct {
	cfg  config.AgentConfig
	http *http.Client
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *AnthropicClient) Call(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	payload := anthropicRequest{
		Model:     c.cfg.Model,
		MaxTokens: resolveMaxTokens(c.cfg),
		System:    systemPrompt,
		Messages: []anthropicMessage{
			{Role: "user", Content: userPrompt},
		},
	}
	headers := map[string]string{
		"x-api-key":         c.cfg.APIKey,
		"anthropic-version": "2023-06-01",
	}
	raw, err := doRequest(ctx, c.http, http.MethodPost, c.cfg.Endpoint, headers, payload)
	if err != nil {
		return "", err
	}
	var parsed anthropicResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("parse anthropic response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("provider anthropic returned error: %s", parsed.Error.Message)
	}
	for _, block := range parsed.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", fmt.Errorf("provider anthropic returned no text content")
}

// ------------------------------------------------------------------
// Google Gemini adapter (generateContent)
// ------------------------------------------------------------------

type GoogleClient struct {
	cfg  config.AgentConfig
	http *http.Client
}

type googleRequest struct {
	SystemInstruction *googleContent  `json:"systemInstruction,omitempty"`
	Contents          []googleContent `json:"contents"`
}

type googleContent struct {
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text string `json:"text"`
}

type googleResponse struct {
	Candidates []struct {
		Content googleContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *GoogleClient) Call(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	url := fmt.Sprintf("%s%s:generateContent?key=%s", c.cfg.Endpoint, c.cfg.Model, c.cfg.APIKey)
	payload := googleRequest{
		SystemInstruction: &googleContent{Parts: []googlePart{{Text: systemPrompt}}},
		Contents: []googleContent{
			{Parts: []googlePart{{Text: userPrompt}}},
		},
	}
	raw, err := doRequest(ctx, c.http, http.MethodPost, url, nil, payload)
	if err != nil {
		return "", err
	}
	var parsed googleResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("parse google response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("provider google returned error: %s", parsed.Error.Message)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("provider google returned no candidates")
	}
	return parsed.Candidates[0].Content.Parts[0].Text, nil
}

// ------------------------------------------------------------------
// Ollama adapter (local models)
// ------------------------------------------------------------------

type OllamaClient struct {
	cfg  config.AgentConfig
	http *http.Client
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaResponse struct {
	Message openAIMessage `json:"message"`
	Done    bool          `json:"done"`
	Error   string        `json:"error,omitempty"`
}

func (c *OllamaClient) Call(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	payload := ollamaRequest{
		Model: c.cfg.Model,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false,
	}
	headers := map[string]string{}
	raw, err := doRequest(ctx, c.http, http.MethodPost, c.cfg.Endpoint, headers, payload)
	if err != nil {
		return "", err
	}
	var parsed ollamaResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("parse ollama response: %w", err)
	}
	if parsed.Error != "" {
		return "", fmt.Errorf("provider ollama returned error: %s", parsed.Error)
	}
	return parsed.Message.Content, nil
}

// ------------------------------------------------------------------
// HuggingFace Inference adapter
// ------------------------------------------------------------------

type HuggingFaceClient struct {
	cfg  config.AgentConfig
	http *http.Client
}

type hfRequest struct {
	Inputs  string                 `json:"inputs"`
	Options map[string]interface{} `json:"options,omitempty"`
}

type hfResponse struct {
	GeneratedText string `json:"generated_text,omitempty"`
	Error         string `json:"error,omitempty"`
}

func (c *HuggingFaceClient) Call(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	fullPrompt := systemPrompt + "\n\n" + userPrompt
	payload := hfRequest{
		Inputs: fullPrompt,
		Options: map[string]interface{}{
			"wait_for_model": true,
		},
	}
	headers := map[string]string{
		"Authorization": "Bearer " + c.cfg.APIKey,
	}
	// HuggingFace endpoint usually ends with the model name in the URL
	url := c.cfg.Endpoint
	raw, err := doRequest(ctx, c.http, http.MethodPost, url, headers, payload)
	if err != nil {
		return "", err
	}
	// HF can return an array or object depending on the model
	var parsedArr []hfResponse
	if err := json.Unmarshal(raw, &parsedArr); err == nil && len(parsedArr) > 0 {
		return parsedArr[0].GeneratedText, nil
	}
	var parsedObj hfResponse
	if err := json.Unmarshal(raw, &parsedObj); err != nil {
		return "", fmt.Errorf("parse huggingface response: %w", err)
	}
	if parsedObj.Error != "" {
		return "", fmt.Errorf("provider huggingface returned error: %s", parsedObj.Error)
	}
	return parsedObj.GeneratedText, nil
}
