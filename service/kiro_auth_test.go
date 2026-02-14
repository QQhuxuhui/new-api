package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func TestResolveClaudeAPIKey_APIKeyMode(t *testing.T) {
	resetKiroAuthCacheForTest()
	settings := dto.ChannelOtherSettings{ClaudeAuthMode: dto.ClaudeAuthModeAPIKey}

	resolved, err := ResolveClaudeAPIKey("sk-test-plain", settings)
	if err != nil {
		t.Fatalf("ResolveClaudeAPIKey returned error: %v", err)
	}
	if resolved != "sk-test-plain" {
		t.Fatalf("unexpected key: got %q", resolved)
	}
}

func TestResolveClaudeAPIKey_InvalidKiroCredential(t *testing.T) {
	resetKiroAuthCacheForTest()
	settings := dto.ChannelOtherSettings{ClaudeAuthMode: dto.ClaudeAuthModeKiroJSON}

	_, err := ResolveClaudeAPIKey(`{"foo":"bar"}`, settings)
	if err == nil {
		t.Fatalf("expected error for invalid credential")
	}
}

func TestResolveClaudeAPIKey_SocialCredentialAndCache(t *testing.T) {
	resetKiroAuthCacheForTest()

	var hitCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/refreshToken" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		atomic.AddInt32(&hitCount, 1)

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["refreshToken"] != "aor-social-1" {
			t.Fatalf("unexpected refresh token: %s", payload["refreshToken"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"social-access-token","refreshToken":"aor-social-2","expiresIn":1200}`))
	}))
	defer server.Close()

	restoreEndpoint := setKiroDesktopAuthEndpointForTest(server.URL)
	defer restoreEndpoint()
	restoreClient := setKiroAuthHTTPClientForTest(server.Client())
	defer restoreClient()

	settings := dto.ChannelOtherSettings{ClaudeAuthMode: dto.ClaudeAuthModeKiroJSON}
	raw := `{"refreshToken":"aor-social-1","provider":"Google"}`

	resolved1, err := ResolveClaudeAPIKey(raw, settings)
	if err != nil {
		t.Fatalf("ResolveClaudeAPIKey first call failed: %v", err)
	}
	if resolved1 != "social-access-token" {
		t.Fatalf("unexpected first resolved key: %s", resolved1)
	}

	resolved2, err := ResolveClaudeAPIKey(raw, settings)
	if err != nil {
		t.Fatalf("ResolveClaudeAPIKey second call failed: %v", err)
	}
	if resolved2 != "social-access-token" {
		t.Fatalf("unexpected second resolved key: %s", resolved2)
	}

	if atomic.LoadInt32(&hitCount) != 1 {
		t.Fatalf("expected one refresh call, got %d", hitCount)
	}
}

func TestResolveClaudeAPIKey_IdCCredential(t *testing.T) {
	resetKiroAuthCacheForTest()

	var hitCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		atomic.AddInt32(&hitCount, 1)

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["clientId"] != "cid-1" || payload["clientSecret"] != "csec-1" {
			t.Fatalf("unexpected client credential payload: %+v", payload)
		}
		if payload["refreshToken"] != "aor-idc-1" {
			t.Fatalf("unexpected refresh token payload: %+v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"idc-access-token","refreshToken":"aor-idc-2","expiresIn":900}`))
	}))
	defer server.Close()

	restoreBuilder := setKiroOIDCTokenURLBuilderForTest(func(region string) string {
		if region != "us-east-1" {
			t.Fatalf("unexpected region: %s", region)
		}
		return server.URL + "/token"
	})
	defer restoreBuilder()
	restoreClient := setKiroAuthHTTPClientForTest(server.Client())
	defer restoreClient()

	settings := dto.ChannelOtherSettings{ClaudeAuthMode: dto.ClaudeAuthModeKiroJSON}
	raw := `{"refreshToken":"aor-idc-1","provider":"BuilderId","clientId":"cid-1","clientSecret":"csec-1","region":"us-east-1"}`

	resolved, err := ResolveClaudeAPIKey(raw, settings)
	if err != nil {
		t.Fatalf("ResolveClaudeAPIKey failed: %v", err)
	}
	if resolved != "idc-access-token" {
		t.Fatalf("unexpected resolved key: %s", resolved)
	}

	if atomic.LoadInt32(&hitCount) != 1 {
		t.Fatalf("expected one oidc refresh call, got %d", hitCount)
	}
}
