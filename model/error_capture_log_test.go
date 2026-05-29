package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

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
