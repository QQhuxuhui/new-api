package jimeng

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestSignRequestUsesEffectiveRequestHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://visual.volcengineapi.com/", strings.NewReader(`{"task_id":"task-1"}`))
	req.Host = "jimeng-gateway.example"

	if err := (&TaskAdaptor{}).signRequest(req, "access-key", "secret-key"); err != nil {
		t.Fatalf("sign request: %v", err)
	}
	if got := req.Header.Get("Host"); got != req.Host {
		t.Fatalf("signed Host = %q, want effective request Host %q", got, req.Host)
	}
}

func TestSignedHeaderNamesOnlyProtectsHMACRequests(t *testing.T) {
	bearer := &TaskAdaptor{}
	bearer.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "sk-relay-key"}})
	if got := bearer.SignedHeaderNames(); len(got) != 0 {
		t.Fatalf("Bearer relay is not HMAC-signed, got protected headers %v", got)
	}

	hmacAdaptor := &TaskAdaptor{}
	hmacAdaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "access-key|secret-key"}})
	if got := hmacAdaptor.SignedHeaderNames(); len(got) == 0 {
		t.Fatal("HMAC request must protect its signed headers")
	}
}

func TestBearerPollingAppliesExplicitAuthorizationOverrideLast(t *testing.T) {
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotAuthorization = req.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":10000,"data":{"status":"done"}}`)
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ApiKey:         "sk-relay-key",
		ChannelBaseUrl: server.URL,
	}})
	resp, err := adaptor.FetchTask(server.URL, "sk-relay-key", map[string]any{
		"task_id": "task-1",
	}, "", http.Header{"Authorization": []string{"Bearer overridden"}})
	if err != nil {
		t.Fatalf("fetch task: %v", err)
	}
	_ = resp.Body.Close()

	if gotAuthorization != "Bearer overridden" {
		t.Fatalf("Authorization = %q, want explicit override", gotAuthorization)
	}
}
