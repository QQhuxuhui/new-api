package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func containsModel(models []string, target string) bool {
	for _, modelName := range models {
		if modelName == target {
			return true
		}
	}
	return false
}

func TestDashboardModelCatalogIncludesTaskAdaptorModels(t *testing.T) {
	models := channelId2Models[constant.ChannelTypeAli]
	if !containsModel(models, "wan2.7-i2v") || !containsModel(models, "wan2.7-t2v") {
		t.Fatalf("Ali channel catalog is missing Wan2.7 task models: %v", models)
	}
}

func TestChannelModelCatalogIncludesTaskAdaptorModels(t *testing.T) {
	if _, ok := openAIModelsMap["wan2.7-i2v"]; !ok {
		t.Fatal("global channel model catalog is missing wan2.7-i2v")
	}
	if _, ok := openAIModelsMap["wan2.7-t2v"]; !ok {
		t.Fatal("global channel model catalog is missing wan2.7-t2v")
	}
}

func TestModelCatalogIncludesNamedTaskPlatformModels(t *testing.T) {
	models := channelId2Models[constant.ChannelTypeSunoAPI]
	if !containsModel(models, "suno_music") || !containsModel(models, "suno_lyrics") {
		t.Fatalf("Suno channel catalog is missing task models: %v", models)
	}
	if _, ok := openAIModelsMap["suno_music"]; !ok {
		t.Fatal("global channel model catalog is missing suno_music")
	}
}
