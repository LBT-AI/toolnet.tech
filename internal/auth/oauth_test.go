package auth

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestCredentialStore_EncryptDecryptRoundtrip(t *testing.T) {
	cs, err := NewCredentialStore()
	if err != nil {
		t.Fatalf("NewCredentialStore: %v", err)
	}
	plain := "super-secret-access-token-12345"
	enc, err := cs.encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == plain {
		t.Fatal("ciphertext equals plaintext")
	}
	dec, err := cs.decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != plain {
		t.Fatalf("roundtrip mismatch: got %q want %q", dec, plain)
	}
}

func TestExchangeCode_RequiresTokenEndpoint(t *testing.T) {
	// No endpoint configured -> should fail fast with a clear error, not
	// attempt a network call.
	t.Setenv("TOOLNET_OAUTH_TOKEN_ENDPOINT", "")
	t.Setenv("TOOLNET_OAUTH_TOKEN_ENDPOINT_OPENAI", "")
	_, err := ExchangeCode(context.Background(), "openai", "abc", "http://localhost:1455/auth/callback")
	if err == nil {
		t.Fatal("expected error when no token endpoint is configured")
	}
}

func TestIsDeviceFlowProvider(t *testing.T) {
	// Built-in + known providers are device-flow, regardless of env.
	for _, p := range []string{"openai", "gemini", "antigravity"} {
		if !IsDeviceFlowProvider(p) {
			t.Errorf("%q should be a device-flow provider", p)
		}
	}
	// An unknown provider is not, unless an endpoint is configured.
	if IsDeviceFlowProvider("someunknown") {
		t.Error("unknown provider should not be a device-flow provider by default")
	}
	t.Setenv("TOOLNET_OAUTH_DEVICE_ENDPOINT_SOMEUNKNOWN", "https://example.com/device")
	if !IsDeviceFlowProvider("someunknown") {
		t.Error("unknown provider with configured endpoint should be device-flow")
	}
}

func TestStartDeviceAuthorization_RequiresEndpoint(t *testing.T) {
	t.Setenv("TOOLNET_OAUTH_DEVICE_ENDPOINT_OPENAI", "")
	_, err := StartDeviceAuthorization(context.Background(), "openai")
	if err == nil {
		t.Fatal("expected error when no device endpoint is configured")
	}
}

func TestCredentialStore_MultipleAccounts(t *testing.T) {
	dir := t.TempDir()
	cs := &CredentialStore{path: dir + "/creds.json", key: make([]byte, 32)}

	if err := cs.Save(Credentials{Provider: "openai", AccessToken: "tok-a"}); err != nil {
		t.Fatalf("save a: %v", err)
	}
	if err := cs.Save(Credentials{Provider: "openai", AccessToken: "tok-b"}); err != nil {
		t.Fatalf("save b: %v", err)
	}
	// Different provider should be independent.
	if err := cs.Save(Credentials{Provider: "gemini", AccessToken: "tok-c"}); err != nil {
		t.Fatalf("save c: %v", err)
	}

	list, err := cs.ListCredentials("openai")
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 openai accounts, got %d", len(list))
	}
	if list[0].AccessToken != "tok-a" || list[1].AccessToken != "tok-b" {
		t.Fatalf("account order/decrypt wrong: %+v", list)
	}
	// Load returns the most recent.
	latest, err := cs.Load("openai")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if latest.AccessToken != "tok-b" {
		t.Fatalf("Load should return latest (tok-b), got %q", latest.AccessToken)
	}
}

func TestCredentialStore_BackwardCompatSingleFormat(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/creds.json"
	// Build a legacy-format file (provider -> single JSON credential) using a
	// properly encrypted token, to verify the loader migrates the structure.
	cs := &CredentialStore{path: path, key: make([]byte, 32)}
	enc, err := cs.encrypt("tok-old")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(Credentials{Provider: "openai", AccessToken: enc})
	legacy := map[string]string{"openai": string(b)}
	data, _ := json.MarshalIndent(legacy, "", "  ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := cs.ListCredentials("openai")
	if err != nil {
		t.Fatalf("ListCredentials on legacy file: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 migrated account, got %d", len(list))
	}
	if list[0].AccessToken != "tok-old" {
		t.Fatalf("migrated token wrong: %q", list[0].AccessToken)
	}
}
