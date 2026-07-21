package model

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func TestUpdateOptionMapFromDatabaseMergesCurrentRatioDefaults(t *testing.T) {
	originalModelRatio := ratio_setting.ModelRatio2JSONString()
	originalCacheRatio := ratio_setting.CacheRatio2JSONString()
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		if err := ratio_setting.UpdateModelRatioByJSONString(originalModelRatio); err != nil {
			t.Fatalf("restore model ratios: %v", err)
		}
		if err := ratio_setting.UpdateCacheRatioByJSONString(originalCacheRatio); err != nil {
			t.Fatalf("restore cache ratios: %v", err)
		}
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	if err := updateOptionMapFromDatabase("ModelRatio", `{"gpt-5.6-sol":9.75,"operator-model":3}`); err != nil {
		t.Fatalf("load model ratios: %v", err)
	}
	if ratio, ok, _ := ratio_setting.GetModelRatio("gpt-5.6-sol"); !ok || ratio != 9.75 {
		t.Fatalf("persisted model override lost: ratio=%v ok=%v", ratio, ok)
	}
	if ratio, ok, _ := ratio_setting.GetModelRatio("gpt-5.6-terra"); !ok || ratio != 1.25 {
		t.Fatalf("missing current model default not merged: ratio=%v ok=%v", ratio, ok)
	}
	if ratio, ok, _ := ratio_setting.GetModelRatio("operator-model"); !ok || ratio != 3 {
		t.Fatalf("operator-only model lost: ratio=%v ok=%v", ratio, ok)
	}
	if _, ok, _ := ratio_setting.GetModelRatio("gpt-4"); ok {
		t.Fatal("historical model default was unexpectedly restored")
	}

	if err := updateOptionMapFromDatabase("CacheRatio", `{"gpt-5.6-sol":0.42,"operator-model":0.6}`); err != nil {
		t.Fatalf("load cache ratios: %v", err)
	}
	if ratio, ok := ratio_setting.GetCacheRatio("gpt-5.6-sol"); !ok || ratio != 0.42 {
		t.Fatalf("persisted cache override lost: ratio=%v ok=%v", ratio, ok)
	}
	if ratio, ok := ratio_setting.GetCacheRatio("gpt-5.6-terra"); !ok || ratio != 0.1 {
		t.Fatalf("missing current cache default not merged: ratio=%v ok=%v", ratio, ok)
	}
	if _, ok := ratio_setting.GetCacheRatio("gpt-4"); ok {
		t.Fatal("historical cache default was unexpectedly restored")
	}

	common.OptionMapRWMutex.RLock()
	modelOption := common.OptionMap["ModelRatio"]
	cacheOption := common.OptionMap["CacheRatio"]
	common.OptionMapRWMutex.RUnlock()
	assertJSONRatio(t, modelOption, "gpt-5.6-terra", 1.25)
	assertJSONRatio(t, cacheOption, "gpt-5.6-terra", 0.1)
	assertJSONRatioAbsent(t, modelOption, "gpt-4")
	assertJSONRatioAbsent(t, cacheOption, "gpt-4")
}

func TestUpdateOptionMapKeepsAdministratorReplacementSemantics(t *testing.T) {
	originalModelRatio := ratio_setting.ModelRatio2JSONString()
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		if err := ratio_setting.UpdateModelRatioByJSONString(originalModelRatio); err != nil {
			t.Fatalf("restore model ratios: %v", err)
		}
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	if err := updateOptionMap("ModelRatio", `{"operator-model":7}`); err != nil {
		t.Fatalf("update model ratios: %v", err)
	}
	if _, ok, _ := ratio_setting.GetModelRatio("gpt-5.6-terra"); ok {
		t.Fatal("administrator replacement unexpectedly merged defaults")
	}
}

func assertJSONRatio(t *testing.T, raw, key string, want float64) {
	t.Helper()
	var ratios map[string]float64
	if err := json.Unmarshal([]byte(raw), &ratios); err != nil {
		t.Fatalf("decode option JSON: %v", err)
	}
	if got, ok := ratios[key]; !ok || got != want {
		t.Fatalf("option %q: got %v (present=%v), want %v", key, got, ok, want)
	}
}

func assertJSONRatioAbsent(t *testing.T, raw, key string) {
	t.Helper()
	var ratios map[string]float64
	if err := json.Unmarshal([]byte(raw), &ratios); err != nil {
		t.Fatalf("decode option JSON: %v", err)
	}
	if _, ok := ratios[key]; ok {
		t.Fatalf("option %q was unexpectedly restored", key)
	}
}
