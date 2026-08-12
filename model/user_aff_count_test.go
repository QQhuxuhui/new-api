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

func setupAffCountTestDB(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	commonKeyCol = "`key`"
	dsn := fmt.Sprintf("file:aff_count_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	DB = db
	LOG_DB = db
	if err := db.AutoMigrate(&User{}, &Option{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func mustCreateUser(t *testing.T, id int, username string, inviterId int, affCount int) {
	t.Helper()
	u := &User{
		Id:        id,
		Username:  username,
		Password:  "x",
		InviterId: inviterId,
		AffCount:  affCount,
		// aff_code 有唯一索引，留空会在第二条记录上撞约束
		AffCode: fmt.Sprintf("aff%d", id),
	}
	if err := DB.Create(u).Error; err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
}

func affCountOf(t *testing.T, id int) int {
	t.Helper()
	var u User
	if err := DB.Select("aff_count").Where("id = ?", id).First(&u).Error; err != nil {
		t.Fatalf("load user %d: %v", id, err)
	}
	return u.AffCount
}

// 回归：邀请人数的递增不能依赖「邀请人奖励额度 > 0」。
// 线上把 QuotaForInviter 设为 0 改走返佣结算后，aff_count 曾经永远停在旧值。
func TestInviteUserIncrementsAffCountWithoutInviterQuota(t *testing.T) {
	setupAffCountTestDB(t)
	saved := common.QuotaForInviter
	common.QuotaForInviter = 0
	defer func() { common.QuotaForInviter = saved }()

	mustCreateUser(t, 1, "inviter", 0, 0)

	for i := 0; i < 3; i++ {
		if err := inviteUser(1); err != nil {
			t.Fatalf("inviteUser: %v", err)
		}
	}

	if got := affCountOf(t, 1); got != 3 {
		t.Fatalf("aff_count = %d, want 3", got)
	}

	var u User
	if err := DB.Where("id = ?", 1).First(&u).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	// 奖励为 0 时不应动额度字段
	if u.AffQuota != 0 || u.AffHistoryQuota != 0 {
		t.Fatalf("aff_quota/aff_history should stay 0, got %d/%d", u.AffQuota, u.AffHistoryQuota)
	}
}

func TestInviteUserAddsQuotaWhenConfigured(t *testing.T) {
	setupAffCountTestDB(t)
	saved := common.QuotaForInviter
	common.QuotaForInviter = 5000
	defer func() { common.QuotaForInviter = saved }()

	mustCreateUser(t, 1, "inviter", 0, 0)
	if err := inviteUser(1); err != nil {
		t.Fatalf("inviteUser: %v", err)
	}

	var u User
	if err := DB.Where("id = ?", 1).First(&u).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if u.AffCount != 1 || u.AffQuota != 5000 || u.AffHistoryQuota != 5000 {
		t.Fatalf("got count=%d quota=%d history=%d, want 1/5000/5000", u.AffCount, u.AffQuota, u.AffHistoryQuota)
	}
}

// inviteUser 必须用原子表达式更新，不能整行写回快照，
// 否则邀请人并发进行中的额度扣减会被旧快照覆盖。
func TestInviteUserDoesNotClobberConcurrentQuotaChange(t *testing.T) {
	setupAffCountTestDB(t)
	saved := common.QuotaForInviter
	common.QuotaForInviter = 0
	defer func() { common.QuotaForInviter = saved }()

	mustCreateUser(t, 1, "inviter", 0, 0)
	if err := DB.Model(&User{}).Where("id = ?", 1).UpdateColumn("quota", 100000).Error; err != nil {
		t.Fatalf("seed quota: %v", err)
	}

	// 模拟：inviteUser 执行期间，邀请人的额度被其它请求扣减
	if err := DB.Model(&User{}).Where("id = ?", 1).UpdateColumn("quota", 42).Error; err != nil {
		t.Fatalf("concurrent quota change: %v", err)
	}
	if err := inviteUser(1); err != nil {
		t.Fatalf("inviteUser: %v", err)
	}

	var u User
	if err := DB.Where("id = ?", 1).First(&u).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if u.Quota != 42 {
		t.Fatalf("quota = %d, want 42 (被 inviteUser 的整行写回覆盖了)", u.Quota)
	}
	if u.AffCount != 1 {
		t.Fatalf("aff_count = %d, want 1", u.AffCount)
	}
}

func TestBackfillUserAffCount(t *testing.T) {
	setupAffCountTestDB(t)

	// 1 号邀请了 3 人（其中 1 人已软删），但计数停在 1
	mustCreateUser(t, 1, "inviter", 0, 1)
	mustCreateUser(t, 2, "invitee-a", 1, 0)
	mustCreateUser(t, 3, "invitee-b", 1, 0)
	mustCreateUser(t, 4, "invitee-deleted", 1, 0)
	// 5 号计数本来就是对的，不应被无谓更新
	mustCreateUser(t, 5, "other-inviter", 0, 1)
	mustCreateUser(t, 6, "invitee-c", 5, 0)
	// 7 号计数偏大，也要被纠正
	mustCreateUser(t, 7, "overcounted", 0, 9)

	if err := DB.Delete(&User{}, 4).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if err := backfillUserAffCount(); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	if got := affCountOf(t, 1); got != 2 {
		t.Fatalf("user1 aff_count = %d, want 2 (软删除的被邀请人不计入)", got)
	}
	if got := affCountOf(t, 5); got != 1 {
		t.Fatalf("user5 aff_count = %d, want 1", got)
	}
	if got := affCountOf(t, 7); got != 0 {
		t.Fatalf("user7 aff_count = %d, want 0", got)
	}
}

func TestBackfillUserAffCountIsIdempotent(t *testing.T) {
	setupAffCountTestDB(t)
	mustCreateUser(t, 1, "inviter", 0, 0)
	mustCreateUser(t, 2, "invitee", 1, 0)

	if err := backfillUserAffCount(); err != nil {
		t.Fatalf("first backfill: %v", err)
	}
	if got := affCountOf(t, 1); got != 1 {
		t.Fatalf("aff_count = %d, want 1", got)
	}

	// 第二次运行必须是空操作：即使此后计数被人为改错也不再纠正，
	// 说明 options 守卫生效，不会每次启动都全表扫描。
	if err := DB.Model(&User{}).Where("id = ?", 1).UpdateColumn("aff_count", 77).Error; err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if err := backfillUserAffCount(); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if got := affCountOf(t, 1); got != 77 {
		t.Fatalf("aff_count = %d, want 77 (第二次运行应被 options 守卫跳过)", got)
	}
}
