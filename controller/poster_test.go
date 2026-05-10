package controller

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

func setupPosterTest(t *testing.T) *gin.Engine {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/option/poster/upload", UploadPoster)
	r.GET("/api/poster", GetPoster)
	return r
}

// 构造一个 multipart 请求,写入指定文件名 / Content-Type / body 字节。
func buildMultipartUpload(fieldName, filename, contentType string, body []byte) (*http.Request, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="` + fieldName + `"; filename="` + filename + `"`}
	if contentType != "" {
		h["Content-Type"] = []string{contentType}
	}
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(body); err != nil {
		return nil, err
	}
	w.Close()

	req, err := http.NewRequest("POST", "/api/option/poster/upload", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req, nil
}

func TestUploadPoster_RejectsMissingFile(t *testing.T) {
	r := setupPosterTest(t)
	w := httptest.NewRecorder()

	// 空 multipart body 不带 file 字段
	body := bytes.NewBufferString("")
	req, _ := http.NewRequest("POST", "/api/option/poster/upload", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=----none")
	r.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		var resp struct{ Success bool }
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Success {
			t.Fatal("missing file should not return success")
		}
	}
}

func TestUploadPoster_RejectsUnsupportedMime(t *testing.T) {
	r := setupPosterTest(t)
	req, err := buildMultipartUpload("file", "x.txt", "text/plain", []byte("hello"))
	if err != nil {
		t.Fatalf("build req: %v", err)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Success {
		t.Fatalf("unsupported mime should be rejected, got %s", w.Body.String())
	}
	if !strings.Contains(strings.ToLower(resp.Message), "type") &&
		!strings.Contains(resp.Message, "类型") {
		t.Errorf("error message should mention type/类型, got %q", resp.Message)
	}
}

func TestUploadPoster_RejectsOversize(t *testing.T) {
	r := setupPosterTest(t)
	// 6 MB 的 fake jpeg(>5MB 上限)
	big := bytes.Repeat([]byte("x"), 6*1024*1024)
	req, err := buildMultipartUpload("file", "big.jpg", "image/jpeg", big)
	if err != nil {
		t.Fatalf("build req: %v", err)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Success {
		t.Fatalf("oversize should be rejected, got %s", w.Body.String())
	}
	low := strings.ToLower(resp.Message)
	if !strings.Contains(low, "size") && !strings.Contains(resp.Message, "大小") &&
		!strings.Contains(resp.Message, "5") {
		t.Errorf("error message should mention size limit, got %q", resp.Message)
	}
}

func TestUploadPoster_RejectsWhenOSSNotConfigured(t *testing.T) {
	r := setupPosterTest(t)
	// OSS 配置全清空(其他测试可能污染)
	common.OSSAccessKeyId = ""
	common.OSSAccessKeySecret = ""
	common.OSSEndpoint = ""
	common.OSSBucket = ""

	// 一个有效的小图片(用 PNG 1px 字节流可以,不需要真实有效的 PNG)
	tiny := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a} // PNG header bytes
	req, err := buildMultipartUpload("file", "x.png", "image/png", tiny)
	if err != nil {
		t.Fatalf("build req: %v", err)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Success {
		t.Fatalf("OSS not configured should be rejected, got %s", w.Body.String())
	}
	if !strings.Contains(resp.Message, "OSS") {
		t.Errorf("error should mention OSS, got %q", resp.Message)
	}
}

func TestGetPoster_DefaultDisabled(t *testing.T) {
	r := setupPosterTest(t)
	common.EnablePoster = false
	common.PosterImageUrl = ""
	common.PosterClickUrl = ""

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/poster", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}
	var resp struct {
		Data struct {
			Enabled  bool   `json:"enabled"`
			ImageUrl string `json:"image_url"`
			ClickUrl string `json:"click_url"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Enabled {
		t.Errorf("default enabled should be false, got true")
	}
	if resp.Data.ImageUrl != "" {
		t.Errorf("default image_url should be empty, got %q", resp.Data.ImageUrl)
	}
}

func TestGetPoster_WithConfig(t *testing.T) {
	r := setupPosterTest(t)
	common.EnablePoster = true
	common.PosterImageUrl = "https://oss.example.com/posters/p.jpg"
	common.PosterClickUrl = "https://mp.weixin.qq.com/abc"
	defer func() {
		common.EnablePoster = false
		common.PosterImageUrl = ""
		common.PosterClickUrl = ""
	}()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/poster", nil)
	r.ServeHTTP(w, req)

	var resp struct {
		Data struct {
			Enabled  bool   `json:"enabled"`
			ImageUrl string `json:"image_url"`
			ClickUrl string `json:"click_url"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Data.Enabled {
		t.Error("should be enabled")
	}
	if resp.Data.ImageUrl != "https://oss.example.com/posters/p.jpg" {
		t.Errorf("image_url: %q", resp.Data.ImageUrl)
	}
	if resp.Data.ClickUrl != "https://mp.weixin.qq.com/abc" {
		t.Errorf("click_url: %q", resp.Data.ClickUrl)
	}
}
