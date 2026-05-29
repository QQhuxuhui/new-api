package model

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupErrorCaptureTestDB(t *testing.T) {
	t.Helper()
	dsn := fmt.Sprintf("file:error_capture_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	DB = db
	LOG_DB = db
	if err := db.AutoMigrate(&ErrorCaptureLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestTruncateBody(t *testing.T) {
	short := []byte("hello")
	if got := truncateBody(short); got != "hello" {
		t.Fatalf("short body changed: %q", got)
	}
	big := make([]byte, ErrorCaptureBodyMaxBytes+1000)
	for i := range big {
		big[i] = 'a'
	}
	got := truncateBody(big)
	if len(got) > ErrorCaptureBodyMaxBytes+len(errorCaptureTruncateMark)+4 {
		t.Fatalf("truncated body too large: %d", len(got))
	}
	if !strings.HasSuffix(got, errorCaptureTruncateMark) {
		t.Fatalf("missing truncate mark: ...%q", got[len(got)-20:])
	}
}

func TestTruncateBodyMultibyte(t *testing.T) {
	// '世' is 3 bytes; ErrorCaptureBodyMaxBytes (65536) is not divisible by 3,
	// so cutting at the byte boundary splits a character and forces walk-back.
	r := []byte("世")
	n := (ErrorCaptureBodyMaxBytes/len(r) + 50) // enough to exceed the cap
	big := make([]byte, 0, n*len(r))
	for i := 0; i < n; i++ {
		big = append(big, r...)
	}
	got := truncateBody(big)
	if !strings.HasSuffix(got, errorCaptureTruncateMark) {
		t.Fatalf("missing truncate mark")
	}
	body := strings.TrimSuffix(got, errorCaptureTruncateMark)
	if !utf8.ValidString(body) {
		t.Fatalf("truncated body is not valid UTF-8 (walk-back failed)")
	}
	if len(body) > ErrorCaptureBodyMaxBytes {
		t.Fatalf("body exceeds max: %d", len(body))
	}
}

func TestRecordAndTrimErrorCapture(t *testing.T) {
	setupErrorCaptureTestDB(t)

	targets := []ErrorCaptureTarget{{RuleId: "r1", Keyword: "k", MaxRecords: 3}}
	for i := 0; i < 5; i++ {
		RecordErrorCaptureLogs(targets, ErrorCapturePayload{
			CreatedAt:   int64(i),
			Content:     "boom",
			RequestBody: []byte(fmt.Sprintf("body-%d", i)),
		})
	}
	// 另一个规则不受影响
	RecordErrorCaptureLogs([]ErrorCaptureTarget{{RuleId: "r2", Keyword: "k", MaxRecords: 100}},
		ErrorCapturePayload{RequestBody: []byte("other")})

	// r1 应裁剪到最新 3 条
	logs, total, err := GetErrorCaptureLogs("r1", 1, 50)
	if err != nil {
		t.Fatalf("list r1: %v", err)
	}
	if total != 3 || len(logs) != 3 {
		t.Fatalf("expected 3 kept for r1, got total=%d len=%d", total, len(logs))
	}
	// 列表不返回 request_body
	if logs[0].RequestBody != "" {
		t.Fatalf("list should omit request_body, got %q", logs[0].RequestBody)
	}
	// 最新优先（id desc）：最后写入的 body-4 应在最前
	detail, err := GetErrorCaptureLogDetail(logs[0].Id)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.RequestBody != "body-4" {
		t.Fatalf("expected newest body-4 first, got %q", detail.RequestBody)
	}

	// r2 不受 r1 裁剪影响，保留 1 条
	logs2, total2, err := GetErrorCaptureLogs("r2", 1, 50)
	if err != nil {
		t.Fatalf("list r2: %v", err)
	}
	if total2 != 1 || len(logs2) != 1 {
		t.Fatalf("r2 should keep 1, got total=%d len=%d", total2, len(logs2))
	}

	// 删除 r1 全部
	n, err := DeleteErrorCaptureLogsByRule("r1")
	if err != nil || n != 3 {
		t.Fatalf("delete r1 expected 3, got n=%d err=%v", n, err)
	}
	if _, total3, _ := GetErrorCaptureLogs("r1", 1, 50); total3 != 0 {
		t.Fatalf("r1 should be empty after delete, got total=%d", total3)
	}
}
