package service

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
)

func allowLocalImageFetchForTest(t *testing.T) {
	t.Helper()
	previous := *system_setting.GetFetchSetting()
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	t.Cleanup(func() { *system_setting.GetFetchSetting() = previous })
}

func TestGetImageFromURLRetryableHTTPStatuses(t *testing.T) {
	allowLocalImageFetchForTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		statusCode := http.StatusRequestTimeout
		switch r.URL.Path {
		case "/425":
			statusCode = http.StatusTooEarly
		case "/429":
			statusCode = http.StatusTooManyRequests
		}
		w.WriteHeader(statusCode)
	}))
	defer server.Close()

	for _, path := range []string{"/408", "/425", "/429"} {
		_, _, err := GetImageFromUrl(server.URL + path)
		if err == nil {
			t.Fatalf("expected HTTP error for %s", path)
		}
		if types.IsClientInputError(err) {
			t.Fatalf("temporary HTTP status %s must remain retryable, got %v", path, err)
		}
	}
}

func TestGetImageFromURLAcceptsExactSizeLimit(t *testing.T) {
	allowLocalImageFetchForTest(t)
	previousLimit := constant.MaxFileDownloadMB
	constant.MaxFileDownloadMB = 1
	t.Cleanup(func() { constant.MaxFileDownloadMB = previousLimit })

	limit := constant.MaxFileDownloadMB * 1024 * 1024
	body := bytes.Repeat([]byte{0x01}, limit)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		_, _ = w.Write(body)
	}))
	defer server.Close()

	_, data, err := GetImageFromUrl(server.URL)
	if err != nil {
		t.Fatalf("an image exactly at the configured limit must be accepted: %v", err)
	}
	if data == "" {
		t.Fatal("expected encoded image data")
	}
}
