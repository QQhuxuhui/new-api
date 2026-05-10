package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

// setupOptionMapWithDB 真实 DB(in-memory sqlite)+ OptionMap 都准备好,
// 用于覆盖 UpdateOption 端到端落库行为。
func setupOptionMapWithDB(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:option_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	DB = db
	if err := db.AutoMigrate(&Option{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	setupOptionMapForTest()
}

// cleanupOption 删除某个 key 的 option,避免污染其他测试。
func cleanupOption(t *testing.T, key string) {
	t.Helper()
	if DB != nil {
		DB.Where("`key` = ?", key).Delete(&Option{})
	}
	common.OptionMapRWMutex.Lock()
	if common.OptionMap != nil {
		delete(common.OptionMap, key)
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

// 端到端:UpdateOption("OSSAccessKeySecret", "***") 必须既不落库也不污染 OptionMap。
// 这是 v272 报告的"落库后重启加载丢真实 secret"bug 的回归测试。
func TestUpdateOption_OSSAccessKeySecret_PlaceholderNotPersisted(t *testing.T) {
	setupOptionMapWithDB(t)
	defer cleanupOption(t, "OSSAccessKeySecret")
	defer func() { common.OSSAccessKeySecret = "" }()

	// 1. 先模拟真实 secret 已落库 + 已加载到运行时
	if err := UpdateOption("OSSAccessKeySecret", "real_secret_v1"); err != nil {
		t.Fatalf("seed real secret: %v", err)
	}
	if common.OSSAccessKeySecret != "real_secret_v1" {
		t.Fatalf("setup: %q", common.OSSAccessKeySecret)
	}

	// 2. 调 UpdateOption("OSSAccessKeySecret", "***") — 必须 no-op
	if err := UpdateOption("OSSAccessKeySecret", "***"); err != nil {
		t.Fatalf("placeholder update should silently succeed (no-op): %v", err)
	}

	// 3. 检查 DB 行没被改成 ***
	var stored Option
	if err := DB.Where("`key` = ?", "OSSAccessKeySecret").First(&stored).Error; err != nil {
		t.Fatalf("read DB: %v", err)
	}
	if stored.Value != "real_secret_v1" {
		t.Fatalf("DB poisoned: stored value = %q (want real_secret_v1)", stored.Value)
	}

	// 4. 检查运行时变量也没被污染
	if common.OSSAccessKeySecret != "real_secret_v1" {
		t.Fatalf("runtime var poisoned: %q (want real_secret_v1)", common.OSSAccessKeySecret)
	}
}

// 同上,但用空字符串触发(管理员清空密码框场景)。
func TestUpdateOption_OSSAccessKeySecret_EmptyNotPersisted(t *testing.T) {
	setupOptionMapWithDB(t)
	defer cleanupOption(t, "OSSAccessKeySecret")
	defer func() { common.OSSAccessKeySecret = "" }()

	if err := UpdateOption("OSSAccessKeySecret", "real_secret_v2"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := UpdateOption("OSSAccessKeySecret", ""); err != nil {
		t.Fatalf("empty update should silently succeed: %v", err)
	}

	var stored Option
	if err := DB.Where("`key` = ?", "OSSAccessKeySecret").First(&stored).Error; err != nil {
		t.Fatalf("read DB: %v", err)
	}
	if stored.Value != "real_secret_v2" {
		t.Fatalf("DB poisoned with empty: %q", stored.Value)
	}
	if common.OSSAccessKeySecret != "real_secret_v2" {
		t.Fatalf("runtime var poisoned: %q", common.OSSAccessKeySecret)
	}
}

// 财务参数范围校验
func TestUpdateOption_InviterRewardDefaultPercent_RejectsOutOfRange(t *testing.T) {
	setupOptionMapWithDB(t)
	defer cleanupOption(t, "InviterRewardDefaultPercent")
	defer func() { common.InviterRewardDefaultPercent = 10 }()

	cases := []string{"-1", "101", "abc", ""}
	for _, v := range cases {
		if err := UpdateOption("InviterRewardDefaultPercent", v); err == nil {
			t.Errorf("value=%q should be rejected, got nil error", v)
		}
	}

	// 边界 0 / 100 / 50.5 / 0.5 应该接受
	for _, v := range []string{"0", "100", "50.5", "0.5"} {
		if err := UpdateOption("InviterRewardDefaultPercent", v); err != nil {
			t.Errorf("value=%q should be accepted, got: %v", v, err)
		}
	}
}

func TestUpdateOption_InviterRewardCooldownDays_RejectsOutOfRange(t *testing.T) {
	setupOptionMapWithDB(t)
	defer cleanupOption(t, "InviterRewardCooldownDays")
	defer func() { common.InviterRewardCooldownDays = 7 }()

	for _, v := range []string{"0", "-1", "366", "abc", ""} {
		if err := UpdateOption("InviterRewardCooldownDays", v); err == nil {
			t.Errorf("value=%q should be rejected", v)
		}
	}
	for _, v := range []string{"1", "7", "365"} {
		if err := UpdateOption("InviterRewardCooldownDays", v); err != nil {
			t.Errorf("value=%q should be accepted: %v", v, err)
		}
	}
}

func TestUpdateOption_InviterRewardCutoffMs_RejectsNegative(t *testing.T) {
	setupOptionMapWithDB(t)
	defer cleanupOption(t, "InviterRewardCutoffMs")
	defer func() { common.InviterRewardCutoffMs = 0 }()

	for _, v := range []string{"-1", "abc", ""} {
		if err := UpdateOption("InviterRewardCutoffMs", v); err == nil {
			t.Errorf("value=%q should be rejected", v)
		}
	}
	if err := UpdateOption("InviterRewardCutoffMs", "0"); err != nil {
		t.Errorf("0 (disabled) should be accepted: %v", err)
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
