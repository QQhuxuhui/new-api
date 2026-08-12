package hailuo

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// 回归：任务轮询必须带上渠道 header_override 解析出的请求头。
// 之前只有提交请求走了覆盖逻辑，轮询直接调 adaptor，依赖自定义头的上游
// 会「提交成功但查询失败」。
func TestFetchTaskAppliesExtraHeaders(t *testing.T) {
	var gotApp string
	var gotMulti []string
	var gotHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotApp = r.Header.Get("X-App")
		gotMulti = r.Header.Values("X-Multi")
		gotHost = r.Host
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"base_resp":{"status_code":0}}`)
	}))
	t.Cleanup(upstream.Close)

	adaptor := &TaskAdaptor{}
	adaptor.Init(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:         "k",
			ChannelBaseUrl: upstream.URL,
		},
	})

	resp, err := adaptor.FetchTask(upstream.URL, "k", map[string]any{
		"task_id": "t1",
	}, "", http.Header{
		"X-App":   []string{"cli"},
		"X-Multi": []string{"a", "b"},
		"Host":    []string{"spoofed.example"},
	})
	if err != nil {
		t.Fatalf("FetchTask: %v", err)
	}
	_ = resp.Body.Close()

	if gotApp != "cli" {
		t.Fatalf("X-App = %q, want cli", gotApp)
	}
	if len(gotMulti) != 2 || gotMulti[0] != "a" || gotMulti[1] != "b" {
		t.Fatalf("X-Multi = %v, want [a b]", gotMulti)
	}
	if gotHost != "spoofed.example" {
		t.Fatalf("Host = %q, want spoofed.example (只写 Header 不写 req.Host 不生效)", gotHost)
	}
}
