package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

// 这些测试覆盖 add-poster-popup-system change 在 option.go 中加入的:
//   - 7 个新 key 在 OptionMap init 中的注册
//   - updateOption switch 中的处理
//   - 关键:OSSAccessKeySecret 占位 *** 检测不覆盖原值

func setupOptionMapForTest() {
	// 直接 ensure map 非 nil,避免 InitOptionMap 触发 loadOptionsFromDatabase 需要 DB
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()
}

func TestUpdateOption_OSSPlaintextKeysRoundTrip(t *testing.T) {
	setupOptionMapForTest()
	defer func() {
		// 还原全局变量,避免污染其他测试
		common.OSSAccessKeyId = ""
		common.OSSEndpoint = ""
		common.OSSBucket = ""
		common.PosterImageUrl = ""
		common.PosterClickUrl = ""
		common.EnablePoster = false
	}()

	cases := []struct {
		key, value, want string
		check            func() string
	}{
		{"OSSAccessKeyId", "LTAI5tx", "LTAI5tx", func() string { return common.OSSAccessKeyId }},
		{"OSSEndpoint", "oss-cn-shanghai.aliyuncs.com", "oss-cn-shanghai.aliyuncs.com", func() string { return common.OSSEndpoint }},
		{"OSSBucket", "my-bucket", "my-bucket", func() string { return common.OSSBucket }},
		{"PosterImageUrl", "https://x.com/p.jpg", "https://x.com/p.jpg", func() string { return common.PosterImageUrl }},
		{"PosterClickUrl", "https://mp.weixin.qq.com/abc", "https://mp.weixin.qq.com/abc", func() string { return common.PosterClickUrl }},
	}
	for _, c := range cases {
		if err := updateOptionMap(c.key, c.value); err != nil {
			t.Fatalf("update %s: %v", c.key, err)
		}
		if got := c.check(); got != c.want {
			t.Errorf("%s: want %q, got %q", c.key, c.want, got)
		}
	}
}

func TestUpdateOption_EnablePosterBool(t *testing.T) {
	setupOptionMapForTest()
	defer func() { common.EnablePoster = false }()

	if err := updateOptionMap("EnablePoster", "true"); err != nil {
		t.Fatalf("set true: %v", err)
	}
	if !common.EnablePoster {
		t.Fatal("want true, got false")
	}
	if err := updateOptionMap("EnablePoster", "false"); err != nil {
		t.Fatalf("set false: %v", err)
	}
	if common.EnablePoster {
		t.Fatal("want false, got true")
	}
}

// 关键:OSSAccessKeySecret 收到字面量 *** 时不覆盖原值。
func TestUpdateOption_OSSAccessKeySecret_PlaceholderNoOp(t *testing.T) {
	setupOptionMapForTest()
	defer func() { common.OSSAccessKeySecret = "" }()

	// 先写入真实 secret
	original := "real_secret_xxxxxxxxxxxxxxxx"
	if err := updateOptionMap("OSSAccessKeySecret", original); err != nil {
		t.Fatalf("set original: %v", err)
	}
	if common.OSSAccessKeySecret != original {
		t.Fatalf("setup: want %q, got %q", original, common.OSSAccessKeySecret)
	}

	// 写入 *** 占位 → 不应覆盖
	if err := updateOptionMap("OSSAccessKeySecret", "***"); err != nil {
		t.Fatalf("set placeholder: %v", err)
	}
	if common.OSSAccessKeySecret != original {
		t.Fatalf("placeholder MUST NOT overwrite; got %q (want %q)", common.OSSAccessKeySecret, original)
	}
}

// 真实 secret 中含 *** 子串(极小概率)应被正常写入,不能被误判。
func TestUpdateOption_OSSAccessKeySecret_AllowsSubstringStars(t *testing.T) {
	setupOptionMapForTest()
	defer func() { common.OSSAccessKeySecret = "" }()

	val := "real***mid***end" // 含 *** 子串但不是字面量
	if err := updateOptionMap("OSSAccessKeySecret", val); err != nil {
		t.Fatalf("err: %v", err)
	}
	if common.OSSAccessKeySecret != val {
		t.Fatalf("want %q, got %q", val, common.OSSAccessKeySecret)
	}
}

// 关键:OSSAccessKeySecret 收到空字符串时也不覆盖原值
// (用户清空密码输入框可能只是想"不修改",不是真的想清空 Secret)。
func TestUpdateOption_OSSAccessKeySecret_EmptyAlsoNoOp(t *testing.T) {
	setupOptionMapForTest()
	defer func() { common.OSSAccessKeySecret = "" }()

	original := "real_secret_dont_lose_me"
	if err := updateOptionMap("OSSAccessKeySecret", original); err != nil {
		t.Fatalf("set original: %v", err)
	}

	// 写入空字符串 → 不应覆盖
	if err := updateOptionMap("OSSAccessKeySecret", ""); err != nil {
		t.Fatalf("set empty: %v", err)
	}
	if common.OSSAccessKeySecret != original {
		t.Fatalf("empty MUST NOT overwrite; got %q (want %q)", common.OSSAccessKeySecret, original)
	}
}

// 真实 secret 写入后,再用一个不同的真实 secret 覆盖 OK。
func TestUpdateOption_OSSAccessKeySecret_RealOverwritesReal(t *testing.T) {
	setupOptionMapForTest()
	defer func() { common.OSSAccessKeySecret = "" }()

	if err := updateOptionMap("OSSAccessKeySecret", "secret_v1"); err != nil {
		t.Fatalf("v1: %v", err)
	}
	if err := updateOptionMap("OSSAccessKeySecret", "secret_v2"); err != nil {
		t.Fatalf("v2: %v", err)
	}
	if common.OSSAccessKeySecret != "secret_v2" {
		t.Fatalf("want secret_v2, got %q", common.OSSAccessKeySecret)
	}
}
