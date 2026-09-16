package common

import (
	"bytes"
	"io"
)

// RewrittenImageResponseBody carries the original JSON for usage accounting
// and a separately assembled response for client delivery. Keeping the two
// views together avoids forcing image handlers to read the rewritten body a
// second time just to parse usage.
type RewrittenImageResponseBody struct {
	reader     *bytes.Reader
	original   []byte
	finalBytes []byte
}

func NewRewrittenImageResponseBody(original, finalBytes []byte) *RewrittenImageResponseBody {
	return &RewrittenImageResponseBody{
		reader:     bytes.NewReader(finalBytes),
		original:   original,
		finalBytes: finalBytes,
	}
}

func (b *RewrittenImageResponseBody) Read(p []byte) (int, error) {
	if b == nil || b.reader == nil {
		return 0, io.EOF
	}
	return b.reader.Read(p)
}

func (b *RewrittenImageResponseBody) Close() error { return nil }

func (b *RewrittenImageResponseBody) OriginalBody() []byte {
	if b == nil {
		return nil
	}
	return b.original
}

func (b *RewrittenImageResponseBody) FinalLength() int64 {
	if b == nil {
		return 0
	}
	return int64(len(b.finalBytes))
}
