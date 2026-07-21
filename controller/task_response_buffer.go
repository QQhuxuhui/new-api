package controller

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
)

type taskResponseBuffer struct {
	target     gin.ResponseWriter
	baseHeader http.Header
	header     http.Header
	body       bytes.Buffer
	status     int
	size       int
}

func newTaskResponseBuffer(target gin.ResponseWriter) *taskResponseBuffer {
	baseHeader := target.Header().Clone()
	w := &taskResponseBuffer{target: target, baseHeader: baseHeader}
	w.Reset()
	return w
}

func (w *taskResponseBuffer) Header() http.Header { return w.header }

func (w *taskResponseBuffer) WriteHeader(code int) {
	if code > 0 && !w.Written() {
		w.status = code
	}
}

func (w *taskResponseBuffer) WriteHeaderNow() {
	if !w.Written() {
		w.size = 0
	}
}

func (w *taskResponseBuffer) Write(data []byte) (int, error) {
	w.WriteHeaderNow()
	n, err := w.body.Write(data)
	w.size += n
	return n, err
}

func (w *taskResponseBuffer) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func (w *taskResponseBuffer) Status() int   { return w.status }
func (w *taskResponseBuffer) Size() int     { return w.size }
func (w *taskResponseBuffer) Written() bool { return w.size >= 0 }

func (w *taskResponseBuffer) Reset() {
	w.header = w.baseHeader.Clone()
	w.body.Reset()
	w.status = http.StatusOK
	w.size = -1
}

func (w *taskResponseBuffer) Commit() error {
	if !w.Written() {
		return nil
	}
	targetHeader := w.target.Header()
	for key := range targetHeader {
		targetHeader.Del(key)
	}
	for key, values := range w.header {
		for _, value := range values {
			targetHeader.Add(key, value)
		}
	}
	w.target.WriteHeader(w.status)
	_, err := w.target.Write(w.body.Bytes())
	return err
}

func (w *taskResponseBuffer) Flush() { w.WriteHeaderNow() }

func (w *taskResponseBuffer) CloseNotify() <-chan bool { return w.target.CloseNotify() }

func (w *taskResponseBuffer) Pusher() http.Pusher { return w.target.Pusher() }

func (w *taskResponseBuffer) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := w.target.Hijack()
	if err != nil {
		return nil, nil, fmt.Errorf("hijack task response: %w", err)
	}
	return conn, rw, nil
}
