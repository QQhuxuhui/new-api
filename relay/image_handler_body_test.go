package relay

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

type fixedLengthReader struct {
	remaining int64
	reads     int
}

func (r *fixedLengthReader) Read(p []byte) (int, error) {
	r.reads++
	if r.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	for i := range p {
		p[i] = 'x'
	}
	r.remaining -= int64(len(p))
	return len(p), nil
}

// 声明长度超限：一个字节都不读，直接标记透传（不再是错误——已付费的生成不能变 500）。
func TestReadImageResponseBodyPassesThroughOversizeContentLengthBeforeRead(t *testing.T) {
	r := &fixedLengthReader{remaining: 1}
	resp := &http.Response{Body: io.NopCloser(r), ContentLength: maxImageResponseBytes + 1}
	got, tooLarge, err := readImageResponseBody(resp)
	if err != nil || !tooLarge || got != nil {
		t.Fatalf("oversized content length must passthrough: tooLarge=%v err=%v len=%d", tooLarge, err, len(got))
	}
	if r.reads != 0 {
		t.Fatalf("oversized content length should not read the body, reads=%d", r.reads)
	}
}

// 未知长度超限：读到上限即停，已读前缀拼回流，后续可完整读出全部字节。
func TestReadImageResponseBodyIsBounded(t *testing.T) {
	total := maxImageResponseBytes + 1
	resp := &http.Response{Body: io.NopCloser(&fixedLengthReader{remaining: total}), ContentLength: -1}
	got, tooLarge, err := readImageResponseBody(resp)
	if err != nil || !tooLarge || got != nil {
		t.Fatalf("oversized image response must passthrough: tooLarge=%v err=%v len=%d", tooLarge, err, len(got))
	}
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil || n != total {
		t.Fatalf("restored body must replay all bytes: n=%d want %d err=%v", n, total, err)
	}
}

func TestReadImageResponseBodyReadsWithinLimit(t *testing.T) {
	want := bytes.Repeat([]byte("x"), 32)
	resp := &http.Response{Body: io.NopCloser(bytes.NewReader(want)), ContentLength: int64(len(want))}
	got, tooLarge, err := readImageResponseBody(resp)
	if err != nil || tooLarge {
		t.Fatalf("bounded response: tooLarge=%v err=%v", tooLarge, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("bounded response mismatch: got %d bytes", len(got))
	}
}
