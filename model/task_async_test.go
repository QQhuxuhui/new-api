package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestInitTaskPersistsSelectedKeyForEveryTaskChannel(t *testing.T) {
	task := InitTask(constant.TaskPlatform("video"), &TaskInitContext{
		ChannelType:   constant.ChannelTypeSora,
		ChannelApiKey: "selected-key",
		PublicTaskID:  "task_public",
	})

	if task.TaskID != "task_public" {
		t.Fatalf("expected public task ID to be retained, got %q", task.TaskID)
	}
	if task.PrivateData.Key != "selected-key" {
		t.Fatalf("expected selected key to be persisted, got %q", task.PrivateData.Key)
	}
}
