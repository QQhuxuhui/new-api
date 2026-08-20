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

func TestReadImageResponseBodyRejectsContentLengthBeforeRead(t *testing.T) {
	r := &fixedLengthReader{remaining: 1}
	resp := &http.Response{Body: io.NopCloser(r), ContentLength: maxImageResponseBytes + 1}
	if _, err := readImageResponseBody(resp); err == nil {
		t.Fatal("oversized content length must be rejected")
	}
	if r.reads != 0 {
		t.Fatalf("oversized content length should not read the body, reads=%d", r.reads)
	}
}

func TestReadImageResponseBodyIsBounded(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(&fixedLengthReader{remaining: maxImageResponseBytes + 1}), ContentLength: -1}
	got, err := readImageResponseBody(resp)
	if err == nil {
		t.Fatal("oversized image response must be rejected")
	}
	if got != nil {
		t.Fatalf("oversized image response must not be returned, got %d bytes", len(got))
	}
}

func TestReadImageResponseBodyReadsWithinLimit(t *testing.T) {
	want := bytes.Repeat([]byte("x"), 32)
	resp := &http.Response{Body: io.NopCloser(bytes.NewReader(want)), ContentLength: int64(len(want))}
	got, err := readImageResponseBody(resp)
	if err != nil {
		t.Fatalf("bounded response: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("bounded response mismatch: got %d bytes", len(got))
	}
}
