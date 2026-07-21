package hailuo

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestPollingFileLookupReusesSelectedKeyAndProxy(t *testing.T) {
	const selectedKey = "selected-task-key"
	requests := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("Authorization"); got != "Bearer "+selectedKey {
			t.Errorf("authorization = %q, want selected task key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case QueryTaskEndpoint:
			_, _ = fmt.Fprint(w, `{"base_resp":{"status_code":0}}`)
		case "/v1/files/retrieve":
			_, _ = fmt.Fprint(w, `{"file":{"download_url":"https://cdn.example/video.mp4"},"base_resp":{"status_code":0}}`)
		default:
			t.Errorf("unexpected proxy request path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(proxy.Close)

	adaptor := &TaskAdaptor{}
	adaptor.Init(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:         "aggregate-channel-key",
			ChannelBaseUrl: "http://unreachable.invalid",
		},
	})
	resp, err := adaptor.FetchTask("http://unreachable.invalid", selectedKey, map[string]any{
		"task_id": "provider-task",
	}, proxy.URL)
	if err != nil {
		t.Fatalf("poll through proxy: %v", err)
	}
	_ = resp.Body.Close()

	result, err := adaptor.ParseTaskResult([]byte(`{
		"task_id":"provider-task",
		"status":"Success",
		"file_id":"file-1",
		"base_resp":{"status_code":0}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Url != "https://cdn.example/video.mp4" {
		t.Fatalf("result URL = %q, want proxied file URL", result.Url)
	}
	if requests != 2 {
		t.Fatalf("proxy request count = %d, want query + file lookup", requests)
	}
}

func TestCompletedTaskFileLookupFailureRemainsRetryable(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == QueryTaskEndpoint {
			_, _ = fmt.Fprint(w, `{"base_resp":{"status_code":0}}`)
			return
		}
		http.Error(w, "temporary file service failure", http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)

	adaptor := &TaskAdaptor{}
	resp, err := adaptor.FetchTask("http://unreachable.invalid", "selected-key", map[string]any{
		"task_id": "provider-task",
	}, proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	_, err = adaptor.ParseTaskResult([]byte(`{
		"task_id":"provider-task",
		"status":"Success",
		"file_id":"file-1",
		"base_resp":{"status_code":0}
	}`))
	if err == nil {
		t.Fatal("expected file lookup failure to keep the task retryable")
	}
}
