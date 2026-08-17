package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type memStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemStore() *memStore { return &memStore{data: map[string][]byte{}} }

func (m *memStore) PutObject(_ context.Context, key string, data []byte, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = data
	return nil
}
func (m *memStore) GetObject(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.data[key]
	if !ok {
		return nil, fmt.Errorf("no such key %s", key)
	}
	return d, nil
}
func (m *memStore) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://fake/presigned-get/" + key, nil
}
func (m *memStore) PresignPut(_ context.Context, key string, _ string, _ time.Duration) (string, error) {
	return "https://fake/presigned-put/" + key, nil
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUpscaleImageHappyPath(t *testing.T) {
	store := newMemStore()
	var gotInput map[string]any
	polls := 0
	rp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/run":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			gotInput, _ = req["input"].(map[string]any)
			// 模拟 worker：把结果 PNG 写进 out key
			_ = store.PutObject(r.Context(), gotInput["out_key"].(string), pngBytes(t, 128, 128), "image/png")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "job-1", "status": "IN_QUEUE"})
		case r.URL.Path == "/status/job-1":
			polls++
			status := "IN_PROGRESS"
			if polls >= 2 {
				status = "COMPLETED"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "job-1", "status": status})
		default:
			http.NotFound(w, r)
		}
	}))
	defer rp.Close()

	u := &ImageUpscaler{
		cfg:   &ImageUpscaleConfig{Endpoint: rp.URL, APIKey: "k", Timeout: 10 * time.Second},
		store: store,
		http:  rp.Client(),
		keyFn: func() string { return "upscale/test/req1" },
		pollInterval: 10 * time.Millisecond,
	}
	out, err := u.UpscaleImage(context.Background(), pngBytes(t, 32, 32), 128, 128)
	if err != nil {
		t.Fatalf("UpscaleImage: %v", err)
	}
	cfgImg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil || cfgImg.Width != 128 || cfgImg.Height != 128 {
		t.Fatalf("输出应为 128x128 PNG, got %dx%d err=%v", cfgImg.Width, cfgImg.Height, err)
	}
	if gotInput["src_url"] == "" || gotInput["put_url"] == "" ||
		gotInput["target_w"].(float64) != 128 || gotInput["target_h"].(float64) != 128 {
		t.Fatalf("worker input 不完整: %+v", gotInput)
	}
	if _, err := store.GetObject(context.Background(), "upscale/test/req1/src.png"); err != nil {
		t.Fatal("源图应已上传到存储")
	}
}

func TestUpscaleImageDimensionMismatch(t *testing.T) {
	store := newMemStore()
	rp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/run" {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			in := req["input"].(map[string]any)
			_ = store.PutObject(r.Context(), in["out_key"].(string), pngBytes(t, 64, 64), "image/png") // 尺寸不对
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "j", "status": "COMPLETED"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "j", "status": "COMPLETED"})
	}))
	defer rp.Close()
	u := &ImageUpscaler{
		cfg: &ImageUpscaleConfig{Endpoint: rp.URL, APIKey: "k", Timeout: 5 * time.Second},
		store: store, http: rp.Client(),
		keyFn: func() string { return "upscale/test/req2" }, pollInterval: 10 * time.Millisecond,
	}
	if _, err := u.UpscaleImage(context.Background(), pngBytes(t, 32, 32), 128, 128); err == nil {
		t.Fatal("输出尺寸不符必须报错（由调用方降级）")
	}
}

func TestUpscaleImageRunpodFailed(t *testing.T) {
	rp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "j", "status": "FAILED", "error": "boom"})
	}))
	defer rp.Close()
	u := &ImageUpscaler{
		cfg: &ImageUpscaleConfig{Endpoint: rp.URL, APIKey: "k", Timeout: 5 * time.Second},
		store: newMemStore(), http: rp.Client(),
		keyFn: func() string { return "upscale/test/req3" }, pollInterval: 10 * time.Millisecond,
	}
	if _, err := u.UpscaleImage(context.Background(), pngBytes(t, 32, 32), 128, 128); err == nil {
		t.Fatal("FAILED 状态必须报错")
	}
}
