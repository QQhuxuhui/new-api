package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// 这些测试覆盖 add-poster-popup-system change 在 GetOptions 中
// 加入的 OSSAccessKeySecret 豁免脱敏逻辑。

func setupOptionsControllerTest(t *testing.T) *gin.Engine {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/option/", GetOptions)
	return r
}

// 找到 options 列表中指定 key,返回 value(找不到返回空字符串)。
func findOptionValue(options []*model.Option, key string) string {
	for _, o := range options {
		if o.Key == key {
			return o.Value
		}
	}
	return ""
}

// 检测 options 列表中是否含某 key。
func containsOptionKey(options []*model.Option, key string) bool {
	for _, o := range options {
		if o.Key == key {
			return true
		}
	}
	return false
}

func TestGetOptions_OSSAccessKeySecret_NonEmptyMaskedToStars(t *testing.T) {
	r := setupOptionsControllerTest(t)
	common.OptionMapRWMutex.Lock()
	common.OptionMap["OSSAccessKeySecret"] = "real_secret_xxxxxxxxxxxxxxxx"
	common.OptionMapRWMutex.Unlock()
	defer func() {
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, "OSSAccessKeySecret")
		common.OptionMapRWMutex.Unlock()
	}()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/option/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []*model.Option `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !containsOptionKey(resp.Data, "OSSAccessKeySecret") {
		t.Fatal("OSSAccessKeySecret SHOULD be present in response (with masked value), got missing")
	}
	if v := findOptionValue(resp.Data, "OSSAccessKeySecret"); v != "***" {
		t.Fatalf("OSSAccessKeySecret value: want ***, got %q (real secret leaked!)", v)
	}
}

func TestGetOptions_OSSAccessKeySecret_EmptyReturnsEmpty(t *testing.T) {
	r := setupOptionsControllerTest(t)
	common.OptionMapRWMutex.Lock()
	common.OptionMap["OSSAccessKeySecret"] = ""
	common.OptionMapRWMutex.Unlock()
	defer func() {
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, "OSSAccessKeySecret")
		common.OptionMapRWMutex.Unlock()
	}()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/option/", nil)
	r.ServeHTTP(w, req)

	var resp struct {
		Data []*model.Option `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// 空时 Key 仍存在,Value 为空(让前端区分"未配置")
	if !containsOptionKey(resp.Data, "OSSAccessKeySecret") {
		t.Fatal("empty secret should still be present")
	}
	if v := findOptionValue(resp.Data, "OSSAccessKeySecret"); v != "" {
		t.Fatalf("empty secret should return empty value, got %q", v)
	}
}

// 其他以 Secret/Token/Key 结尾的字段继续被现有过滤剔除(零回归)。
func TestGetOptions_OtherSecretSuffixesStillFiltered(t *testing.T) {
	r := setupOptionsControllerTest(t)
	common.OptionMapRWMutex.Lock()
	common.OptionMap["GitHubClientSecret"] = "should_not_leak"
	common.OptionMap["TurnstileSecretKey"] = "should_not_leak"
	common.OptionMapRWMutex.Unlock()
	defer func() {
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, "GitHubClientSecret")
		delete(common.OptionMap, "TurnstileSecretKey")
		common.OptionMapRWMutex.Unlock()
	}()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/option/", nil)
	r.ServeHTTP(w, req)

	var resp struct {
		Data []*model.Option `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if containsOptionKey(resp.Data, "GitHubClientSecret") {
		t.Error("GitHubClientSecret MUST be filtered out (existing behavior)")
	}
	if containsOptionKey(resp.Data, "TurnstileSecretKey") {
		t.Error("TurnstileSecretKey MUST be filtered out (existing behavior)")
	}
}
