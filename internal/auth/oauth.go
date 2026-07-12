// Package auth implements OAuth2 login flows for providers that support
// browser-based authentication (e.g. OpenAI Codex, Antigravity, etc.).
// It spins up a temporary local HTTP server on localhost:<port> to capture
// the authorization code, then exchanges it for an access token and stores
// it encrypted in ~/.toolnet/credentials.json.
package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Credentials stores tokens for a single provider.
type Credentials struct {
	Provider     string    `json:"provider"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

// CredentialStore holds all saved credentials encrypted on disk.
type CredentialStore struct {
	path string
	key  []byte
}

// NewCredentialStore opens or creates the store at ~/.toolnet/credentials.json.
// The encryption key is derived from TOOLNET_CREDENTIAL_KEY (the legacy
// LBT_CREDENTIAL_KEY is also accepted), or generated once into a mode-0600 file.
func NewCredentialStore() (*CredentialStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".toolnet")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create .toolnet dir: %w", err)
	}
	passphrase := os.Getenv("TOOLNET_CREDENTIAL_KEY")
	if passphrase == "" {
		passphrase = os.Getenv("LBT_CREDENTIAL_KEY")
	}
	if passphrase == "" {
		keyPath := filepath.Join(dir, "credential.key")
		key, readErr := os.ReadFile(keyPath)
		if os.IsNotExist(readErr) {
			key = make([]byte, 32)
			if _, err := io.ReadFull(rand.Reader, key); err != nil {
				return nil, fmt.Errorf("generate credential key: %w", err)
			}
			if err := os.WriteFile(keyPath, key, 0o600); err != nil {
				return nil, fmt.Errorf("write credential key: %w", err)
			}
		} else if readErr != nil {
			return nil, fmt.Errorf("read credential key: %w", readErr)
		}
		passphrase = base64.RawStdEncoding.EncodeToString(key)
	}
	h := sha256.Sum256([]byte(passphrase))
	return &CredentialStore{
		path: filepath.Join(dir, "credentials.json"),
		key:  h[:],
	}, nil
}

func (cs *CredentialStore) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(cs.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (cs *CredentialStore) decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(cs.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// Save appends credentials for a provider, encrypting the token. A provider
// may hold several accounts (e.g. logged in with multiple OAuth identities);
// each call to Save adds another, supporting round-robin rotation across
// them.
func (cs *CredentialStore) Save(creds Credentials) error {
	all, err := cs.readAll()
	if err != nil {
		return err
	}
	enc, err := cs.encryptCreds(creds)
	if err != nil {
		return err
	}
	all[creds.Provider] = append(all[creds.Provider], enc)
	return cs.writeAll(all)
}

// encryptCreds encrypts the token (and refresh token, if present) and returns
// the JSON encoding of the credential ready to be persisted.
func (cs *CredentialStore) encryptCreds(creds Credentials) (string, error) {
	c := creds
	encToken, err := cs.encrypt(c.AccessToken)
	if err != nil {
		return "", fmt.Errorf("encrypt token: %w", err)
	}
	c.AccessToken = encToken
	if c.RefreshToken != "" {
		encRefresh, err := cs.encrypt(c.RefreshToken)
		if err != nil {
			return "", fmt.Errorf("encrypt refresh token: %w", err)
		}
		c.RefreshToken = encRefresh
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// readAll loads the credential file, supporting both the current
// provider -> list-of-credentials format and the legacy single-credential
// per-provider format (auto-migrated in memory).
func (cs *CredentialStore) readAll() (map[string][]string, error) {
	data, err := os.ReadFile(cs.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]string{}, nil
		}
		return nil, fmt.Errorf("read credentials file: %w", err)
	}
	listFmt := map[string][]string{}
	if err := json.Unmarshal(data, &listFmt); err == nil {
		return listFmt, nil
	}
	singleFmt := map[string]string{}
	if err := json.Unmarshal(data, &singleFmt); err != nil {
		return nil, fmt.Errorf("parse credentials file: %w", err)
	}
	out := map[string][]string{}
	for p, raw := range singleFmt {
		out[p] = []string{raw}
	}
	return out, nil
}

func (cs *CredentialStore) writeAll(all map[string][]string) error {
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cs.path, data, 0o600)
}

// ListCredentials returns all decrypted accounts stored for a provider, in
// the order they were saved.
func (cs *CredentialStore) ListCredentials(provider string) ([]Credentials, error) {
	all, err := cs.readAll()
	if err != nil {
		return nil, err
	}
	raws, ok := all[provider]
	if !ok || len(raws) == 0 {
		return nil, fmt.Errorf("no credentials found for provider %q", provider)
	}
	out := make([]Credentials, 0, len(raws))
	for _, raw := range raws {
		var creds Credentials
		if err := json.Unmarshal([]byte(raw), &creds); err != nil {
			return nil, err
		}
		decToken, err := cs.decrypt(creds.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("decrypt token: %w", err)
		}
		creds.AccessToken = decToken
		if creds.RefreshToken != "" {
			decRefresh, err := cs.decrypt(creds.RefreshToken)
			if err != nil {
				return nil, fmt.Errorf("decrypt refresh token: %w", err)
			}
			creds.RefreshToken = decRefresh
		}
		out = append(out, creds)
	}
	return out, nil
}

// Load retrieves the most recently saved (decrypted) credential for a
// provider. Retained for backward compatibility; prefer ListCredentials
// when multiple accounts are expected.
func (cs *CredentialStore) Load(provider string) (*Credentials, error) {
	list, err := cs.ListCredentials(provider)
	if err != nil {
		return nil, err
	}
	return &list[len(list)-1], nil
}

// HasCredentials reports whether a usable credential for the provider is
// already stored (used by config validation to allow api_key-free setups
// that rely on `toolnet login`).
func HasCredentials(provider string) bool {
	cs, err := NewCredentialStore()
	if err != nil {
		return false
	}
	if _, err := cs.ListCredentials(provider); err != nil {
		return false
	}
	return true
}

// OAuthServer handles the local redirect callback for browser-based auth.
type OAuthServer struct {
	port     int
	codeChan chan string
	server   *http.Server
}

// NewOAuthServer creates a temporary HTTP server on localhost:port.
func NewOAuthServer(port int) *OAuthServer {
	return &OAuthServer{
		port:     port,
		codeChan: make(chan string, 1),
	}
}

// Start launches the server and returns the callback URL to use in the auth request.
func (o *OAuthServer) Start() string {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			code = r.URL.Query().Get("token") // some providers return token directly
		}
		o.codeChan <- code
		fmt.Fprintf(w, "<html><body><h2>Authentication successful. You can close this window.</h2></body></html>")
	})
	o.server = &http.Server{Addr: fmt.Sprintf("localhost:%d", o.port), Handler: mux}
	go func() {
		if err := o.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "oauth server error: %v\n", err)
		}
	}()
	return fmt.Sprintf("http://localhost:%d/auth/callback", o.port)
}

// Wait blocks until the authorization code is received, then shuts down the server.
func (o *OAuthServer) Wait(ctx context.Context) (string, error) {
	select {
	case code := <-o.codeChan:
		_ = o.server.Shutdown(ctx)
		return code, nil
	case <-ctx.Done():
		_ = o.server.Shutdown(ctx)
		return "", ctx.Err()
	}
}

// builtinEndpoints ships sensible defaults for providers whose OAuth
// endpoints are public and stable. Operators can still override any of them
// with the TOOLNET_OAUTH_* environment variables below.
var builtinEndpoints = map[string]struct {
	device string // OAuth 2.0 Device Authorization endpoint (optional)
	token  string // token endpoint for the authorization-code or device flow
}{
	"openai": {
		device: "https://auth.openai.com/codex/device",
		token:  "https://auth.openai.com/codex/device/token",
	},
}

// oauthClientID returns the OAuth client id from TOOLNET_OAUTH_CLIENT_ID.
func oauthClientID() string {
	return os.Getenv("TOOLNET_OAUTH_CLIENT_ID")
}

// deviceEndpointFor resolves the OAuth2 device-authorization endpoint for a
// provider (used by the device flow). Order:
//  1. TOOLNET_OAUTH_DEVICE_ENDPOINT_<PROVIDER>
//  2. TOOLNET_OAUTH_DEVICE_ENDPOINT (global)
//  3. built-in default for the provider
func deviceEndpointFor(provider string) (string, error) {
	if v := os.Getenv("TOOLNET_OAUTH_DEVICE_ENDPOINT_" + strings.ToUpper(provider)); v != "" {
		return v, nil
	}
	if v := os.Getenv("TOOLNET_OAUTH_DEVICE_ENDPOINT"); v != "" {
		return v, nil
	}
	if e, ok := builtinEndpoints[strings.ToLower(provider)]; ok && e.device != "" {
		return e.device, nil
	}
	return "", fmt.Errorf("no device endpoint configured for provider %q (set TOOLNET_OAUTH_DEVICE_ENDPOINT_%s)",
		provider, strings.ToUpper(provider))
}

// tokenEndpointFor resolves the OAuth2 token endpoint for a provider. Order:
//  1. TOOLNET_OAUTH_TOKEN_ENDPOINT_<PROVIDER>
//  2. TOOLNET_OAUTH_TOKEN_ENDPOINT (global)
//  3. built-in default for the provider
func tokenEndpointFor(provider string) (string, error) {
	if v := os.Getenv("TOOLNET_OAUTH_TOKEN_ENDPOINT_" + strings.ToUpper(provider)); v != "" {
		return v, nil
	}
	if v := os.Getenv("TOOLNET_OAUTH_TOKEN_ENDPOINT"); v != "" {
		return v, nil
	}
	if e, ok := builtinEndpoints[strings.ToLower(provider)]; ok && e.token != "" {
		return e.token, nil
	}
	return "", fmt.Errorf("no token endpoint configured for provider %q (set TOOLNET_OAUTH_TOKEN_ENDPOINT_%s)",
		provider, strings.ToUpper(provider))
}

// knownDeviceFlowProviders lists providers that authenticate via the OAuth2
// Device Authorization flow. Built-in endpoints (e.g. openai) live in
// builtinEndpoints; others rely on operator-supplied env vars and will
// surface a clear "set TOOLNET_OAUTH_DEVICE_ENDPOINT_<PROVIDER>" message.
var knownDeviceFlowProviders = map[string]bool{
	"openai":      true,
	"gemini":      true,
	"antigravity": true,
}

// IsDeviceFlowProvider reports whether the provider authenticates via the
// OAuth2 Device Authorization flow (e.g. OpenAI Codex, Gemini, Antigravity)
// rather than the browser redirect (authorization-code) flow.
func IsDeviceFlowProvider(provider string) bool {
	if _, err := deviceEndpointFor(provider); err == nil {
		return true
	}
	return knownDeviceFlowProviders[strings.ToLower(provider)]
}

// DeviceFlow represents an in-progress OAuth2 device authorization, as
// returned by StartDeviceAuthorization.
type DeviceFlow struct {
	Provider        string
	DeviceCode      string
	UserCode        string
	VerificationURI string
	Interval        int
}

// StartDeviceAuthorization kicks off the device flow: it asks the provider
// for a user/device code and the URL the user must visit to approve. The
// caller should present VerificationURI and UserCode to the user, then call
// Poll to wait for the token.
func StartDeviceAuthorization(ctx context.Context, provider string) (*DeviceFlow, error) {
	endpoint, err := deviceEndpointFor(provider)
	if err != nil {
		return nil, err
	}
	clientID := oauthClientID()
	form := url.Values{}
	if clientID != "" {
		form.Set("client_id", clientID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build device request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device authorization: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read device response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("device endpoint returned status %d: %s", resp.StatusCode, string(body))
	}
	var d struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		Interval                int    `json:"interval"`
		Error                   string `json:"error"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("parse device response: %w", err)
	}
	if d.Error != "" {
		return nil, fmt.Errorf("device endpoint error: %s", d.Error)
	}
	if d.DeviceCode == "" {
		return nil, fmt.Errorf("device endpoint returned no device_code")
	}
	uri := d.VerificationURIComplete
	if uri == "" {
		uri = d.VerificationURI
	}
	return &DeviceFlow{
		Provider:        provider,
		DeviceCode:      d.DeviceCode,
		UserCode:        d.UserCode,
		VerificationURI: uri,
		Interval:        d.Interval,
	}, nil
}

// Poll blocks, polling the token endpoint, until the user completes the
// device flow, the context is cancelled, or a hard error occurs. It honours
// the provider's polling interval and the "slow_down" backoff signal.
func (f *DeviceFlow) Poll(ctx context.Context) (*Credentials, error) {
	tokenEndpoint, err := tokenEndpointFor(f.Provider)
	if err != nil {
		return nil, err
	}
	clientID := oauthClientID()
	interval := f.Interval
	if interval <= 0 {
		interval = 5
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", f.DeviceCode)
	if clientID != "" {
		form.Set("client_id", clientID)
	}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, fmt.Errorf("build token poll: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("token poll: %w", err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read token poll response: %w", err)
		}
		var tok struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int    `json:"expires_in"`
			Error        string `json:"error"`
		}
		if err := json.Unmarshal(body, &tok); err != nil {
			return nil, fmt.Errorf("parse token poll response: %w (body: %s)", err, string(body))
		}
		switch tok.Error {
		case "":
			if tok.AccessToken == "" {
				return nil, fmt.Errorf("token endpoint returned no access_token")
			}
			creds := &Credentials{Provider: f.Provider, AccessToken: tok.AccessToken}
			if tok.RefreshToken != "" {
				creds.RefreshToken = tok.RefreshToken
			}
			if tok.ExpiresIn > 0 {
				creds.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
			}
			return creds, nil
		case "authorization_pending":
			// expected: user has not finished approving yet.
		case "slow_down":
			interval += 5
		default:
			return nil, fmt.Errorf("token endpoint error: %s", tok.Error)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}
	}
}

// ExchangeCode performs the OAuth2 authorization-code grant: it POSTs the
// received code to the provider's token endpoint and returns the resulting
// credentials (access token, optional refresh token, expiry). clientID and
// clientSecret are read from TOOLNET_OAUTH_CLIENT_ID /
// TOOLNET_OAUTH_CLIENT_SECRET; secret may be empty for public clients.
func ExchangeCode(ctx context.Context, provider, code, redirectURI string) (*Credentials, error) {
	tokenEndpoint, err := tokenEndpointFor(provider)
	if err != nil {
		return nil, err
	}
	clientID := os.Getenv("TOOLNET_OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("TOOLNET_OAUTH_CLIENT_SECRET")

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	if clientID != "" {
		form.Set("client_id", clientID)
	}
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	if tok.Error != "" {
		return nil, fmt.Errorf("token endpoint error: %s %s", tok.Error, tok.ErrorDesc)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint returned no access_token")
	}
	creds := &Credentials{
		Provider:    provider,
		AccessToken: tok.AccessToken,
	}
	if tok.RefreshToken != "" {
		creds.RefreshToken = tok.RefreshToken
	}
	if tok.ExpiresIn > 0 {
		creds.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	}
	return creds, nil
}
