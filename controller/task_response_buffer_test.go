package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTaskResponseBufferDefersAndCommitsResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	buffer := newTaskResponseBuffer(c.Writer)
	c.Writer = buffer

	c.JSON(http.StatusOK, gin.H{"id": "task_public"})
	if recorder.Body.Len() != 0 {
		t.Fatalf("response escaped before commit: %q", recorder.Body.String())
	}
	if err := buffer.Commit(); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "task_public") {
		t.Fatalf("unexpected committed response: code=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestTaskResponseBufferResetDiscardsPreviousAttempt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	buffer := newTaskResponseBuffer(c.Writer)
	c.Writer = buffer

	c.JSON(http.StatusBadGateway, gin.H{"error": "first"})
	buffer.Reset()
	c.JSON(http.StatusOK, gin.H{"id": "second"})
	if err := buffer.Commit(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(recorder.Body.String(), "first") || !strings.Contains(recorder.Body.String(), "second") {
		t.Fatalf("reset did not isolate attempts: %q", recorder.Body.String())
	}
}
