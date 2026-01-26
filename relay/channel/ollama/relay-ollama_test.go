package ollama

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func TestFetchOllamaModels_RespectsFallbackTimeoutEnvWhenRelayTimeoutZero(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 0
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
	})

	if err := os.Setenv("OLLAMA_FETCH_TIMEOUT", "50ms"); err != nil {
		t.Fatalf("set env: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("OLLAMA_FETCH_TIMEOUT")
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3"}]}`))
	}))
	t.Cleanup(server.Close)

	_, err := FetchOllamaModels(server.URL, "")
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}

func TestFetchOllamaModels_UsesProxyFromEnv(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/tags") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3"}]}`))
	}))
	t.Cleanup(upstream.Close)

	if err := os.Setenv("OLLAMA_FETCH_PROXY", "ftp://127.0.0.1:1234"); err != nil {
		t.Fatalf("set env: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("OLLAMA_FETCH_PROXY")
	})

	_, err := FetchOllamaModels(upstream.URL, "")
	if err == nil || !strings.Contains(err.Error(), "unsupported proxy scheme") {
		t.Fatalf("expected unsupported proxy scheme error, got: %v", err)
	}
}
