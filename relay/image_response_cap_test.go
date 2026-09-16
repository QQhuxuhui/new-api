package relay

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

// 超过改写上限的上游响应必须原样透传：前缀已被读走也要拼回去，字节不丢不重。
func TestReadImageResponseBodyOversizePassthrough(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), int(maxImageResponseBytes)+1024)
	resp := &http.Response{ContentLength: -1, Body: io.NopCloser(bytes.NewReader(payload))}
	body, tooLarge, err := readImageResponseBody(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tooLarge || body != nil {
		t.Fatalf("expected tooLarge passthrough, got tooLarge=%v len=%d", tooLarge, len(body))
	}
	rest, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read restored body: %v", err)
	}
	if !bytes.Equal(rest, payload) {
		t.Fatalf("restored body mismatch: got %d bytes want %d", len(rest), len(payload))
	}
	// 声明长度超限时不读任何字节，直接透传。
	resp2 := &http.Response{ContentLength: maxImageResponseBytes + 1, Body: io.NopCloser(bytes.NewReader(payload))}
	if _, tooLarge, err := readImageResponseBody(resp2); err != nil || !tooLarge {
		t.Fatalf("declared oversize should passthrough, got tooLarge=%v err=%v", tooLarge, err)
	}
	// 正常体积照常读完。
	small := []byte(`{"data":[]}`)
	resp3 := &http.Response{ContentLength: int64(len(small)), Body: io.NopCloser(bytes.NewReader(small))}
	body, tooLarge, err = readImageResponseBody(resp3)
	if err != nil || tooLarge || !bytes.Equal(body, small) {
		t.Fatalf("small body should be read fully, got tooLarge=%v err=%v body=%q", tooLarge, err, body)
	}
}
