package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/dto"
)

const (
	defaultKiroDesktopAuthEndpoint = "https://prod.us-east-1.auth.desktop.kiro.dev"
	defaultKiroRegion              = "us-east-1"
	kiroRefreshSafeWindow          = 30 * time.Second
)

type kiroCredential struct {
	RefreshToken string `json:"refreshToken"`
	Provider     string `json:"provider,omitempty"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Region       string `json:"region,omitempty"`
}

type kiroTokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int64  `json:"expiresIn"`
}

type kiroTokenCacheEntry struct {
	mu           sync.Mutex
	refreshToken string
	accessToken  string
	expireAt     time.Time
}

var (
	kiroDesktopAuthEndpoint = defaultKiroDesktopAuthEndpoint
	kiroOIDCTokenURLBuilder = func(region string) string {
		region = strings.TrimSpace(region)
		if region == "" {
			region = defaultKiroRegion
		}
		return fmt.Sprintf("https://oidc.%s.amazonaws.com/token", region)
	}
	kiroAuthHTTPClient = &http.Client{Timeout: 15 * time.Second}
	kiroTokenCache     sync.Map
)

func ResolveClaudeAPIKey(rawKey string, settings dto.ChannelOtherSettings) (string, error) {
	mode := settings.ClaudeAuthMode
	if mode == "" {
		mode = dto.ClaudeAuthModeAPIKey
	}
	if mode != dto.ClaudeAuthModeKiroJSON {
		return rawKey, nil
	}

	credential, err := parseKiroCredential(rawKey)
	if err != nil {
		return "", err
	}

	cacheKey := hashKiroCredentialKey(rawKey)
	cacheValue, _ := kiroTokenCache.LoadOrStore(cacheKey, &kiroTokenCacheEntry{refreshToken: credential.RefreshToken})
	entry := cacheValue.(*kiroTokenCacheEntry)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.refreshToken == "" {
		entry.refreshToken = credential.RefreshToken
	}

	now := time.Now()
	if entry.accessToken != "" && !entry.expireAt.IsZero() && now.Before(entry.expireAt.Add(-kiroRefreshSafeWindow)) {
		return entry.accessToken, nil
	}

	tokenResponse, err := refreshKiroAccessToken(credential, entry.refreshToken)
	if err != nil {
		return "", err
	}
	if tokenResponse.AccessToken == "" {
		return "", fmt.Errorf("kiro credential refresh returned empty access token")
	}

	expiresIn := tokenResponse.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 900
	}

	entry.accessToken = tokenResponse.AccessToken
	if tokenResponse.RefreshToken != "" {
		entry.refreshToken = tokenResponse.RefreshToken
	}
	entry.expireAt = now.Add(time.Duration(expiresIn) * time.Second)

	return entry.accessToken, nil
}

func parseKiroCredential(raw string) (kiroCredential, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return kiroCredential{}, fmt.Errorf("kiro credential is empty")
	}

	credential := kiroCredential{}
	if err := json.Unmarshal([]byte(trimmed), &credential); err != nil {
		return kiroCredential{}, fmt.Errorf("invalid kiro credential json: %w", err)
	}

	credential.RefreshToken = strings.TrimSpace(credential.RefreshToken)
	credential.Provider = strings.TrimSpace(credential.Provider)
	credential.ClientID = strings.TrimSpace(credential.ClientID)
	credential.ClientSecret = strings.TrimSpace(credential.ClientSecret)
	credential.Region = strings.TrimSpace(credential.Region)

	if credential.RefreshToken == "" {
		return kiroCredential{}, fmt.Errorf("kiro credential missing refreshToken")
	}
	if !strings.HasPrefix(credential.RefreshToken, "aor") {
		return kiroCredential{}, fmt.Errorf("kiro credential refreshToken format invalid")
	}

	if isKiroIdCCredential(credential) {
		if credential.ClientID == "" || credential.ClientSecret == "" {
			return kiroCredential{}, fmt.Errorf("kiro idc credential requires clientId and clientSecret")
		}
		if credential.Region == "" {
			credential.Region = defaultKiroRegion
		}
	}

	return credential, nil
}

func isKiroIdCCredential(credential kiroCredential) bool {
	provider := strings.ToLower(strings.TrimSpace(credential.Provider))
	if provider == "builderid" || provider == "enterprise" {
		return true
	}
	return credential.ClientID != "" || credential.ClientSecret != ""
}

func refreshKiroAccessToken(credential kiroCredential, refreshToken string) (kiroTokenResponse, error) {
	if strings.TrimSpace(refreshToken) == "" {
		refreshToken = credential.RefreshToken
	}

	if isKiroIdCCredential(credential) {
		region := credential.Region
		if region == "" {
			region = defaultKiroRegion
		}
		return doKiroRefreshRequest(kiroOIDCTokenURLBuilder(region), map[string]string{
			"clientId":     credential.ClientID,
			"clientSecret": credential.ClientSecret,
			"grantType":    "refresh_token",
			"refreshToken": refreshToken,
		})
	}

	return doKiroRefreshRequest(strings.TrimRight(kiroDesktopAuthEndpoint, "/")+"/refreshToken", map[string]string{
		"refreshToken": refreshToken,
	})
}

func doKiroRefreshRequest(url string, payload map[string]string) (kiroTokenResponse, error) {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return kiroTokenResponse{}, fmt.Errorf("marshal kiro refresh payload failed: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return kiroTokenResponse{}, fmt.Errorf("build kiro refresh request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := kiroAuthHTTPClient.Do(req)
	if err != nil {
		return kiroTokenResponse{}, fmt.Errorf("kiro refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return kiroTokenResponse{}, fmt.Errorf("read kiro refresh response failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized {
			return kiroTokenResponse{}, fmt.Errorf("refreshToken 已过期或无效")
		}
		body := string(respBody)
		if len(body) > 200 {
			body = body[:200]
		}
		return kiroTokenResponse{}, fmt.Errorf("kiro refresh failed with status %d: %s", resp.StatusCode, body)
	}

	var tokenResponse kiroTokenResponse
	if err = json.Unmarshal(respBody, &tokenResponse); err != nil {
		return kiroTokenResponse{}, fmt.Errorf("parse kiro refresh response failed: %w", err)
	}

	return tokenResponse, nil
}

func hashKiroCredentialKey(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:])
}

func setKiroDesktopAuthEndpointForTest(endpoint string) func() {
	old := kiroDesktopAuthEndpoint
	kiroDesktopAuthEndpoint = strings.TrimSpace(endpoint)
	return func() {
		kiroDesktopAuthEndpoint = old
	}
}

func setKiroOIDCTokenURLBuilderForTest(builder func(region string) string) func() {
	old := kiroOIDCTokenURLBuilder
	kiroOIDCTokenURLBuilder = builder
	return func() {
		kiroOIDCTokenURLBuilder = old
	}
}

func setKiroAuthHTTPClientForTest(client *http.Client) func() {
	old := kiroAuthHTTPClient
	kiroAuthHTTPClient = client
	return func() {
		kiroAuthHTTPClient = old
	}
}

func resetKiroAuthCacheForTest() {
	kiroTokenCache = sync.Map{}
}
