package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupManageUserBanTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedisEnabled
	})

	dsn := fmt.Sprintf("file:manage_user_ban_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	model.DB = db
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserPlan{},
		&model.UserDailyPool{},
		&model.UserAssetSnapshot{},
		&model.AdminPlanLog{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func callManageUser(t *testing.T, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", 99)
	context.Set("username", "admin")
	context.Set("role", common.RoleAdminUser)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/manage", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	ManageUser(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	return recorder
}

func TestManageUser_DisableAndEnableRunPlanPauseLifecycle(t *testing.T) {
	db := setupManageUserBanTest(t)
	user := &model.User{Username: "managed-pause", Password: "12345678", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	originalExpiry := time.Now().Add(time.Hour).UnixMilli()
	plan := &model.UserPlan{
		UserId: user.Id, Quota: 100, Status: model.UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1, StartedAt: time.Now().Add(-time.Hour).UnixMilli(), ExpiresAt: originalExpiry,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}

	disable := callManageUser(t, user.Id, fmt.Sprintf(`{"id":%d,"action":"disable"}`, user.Id))
	if !strings.Contains(disable.Body.String(), `"success":true`) {
		t.Fatalf("disable failed: %s", disable.Body.String())
	}
	var disabledUser model.User
	var pausedPlan model.UserPlan
	if err := db.First(&disabledUser, user.Id).Error; err != nil {
		t.Fatalf("reload disabled user: %v", err)
	}
	if err := db.First(&pausedPlan, plan.Id).Error; err != nil {
		t.Fatalf("reload paused plan: %v", err)
	}
	if disabledUser.Status != common.UserStatusDisabled || pausedPlan.PausedAt == 0 || pausedPlan.Pinned != 1 || pausedPlan.IsCurrent != 1 {
		t.Fatalf("disabled status=%d paused=%d current=%d pinned=%d", disabledUser.Status, pausedPlan.PausedAt, pausedPlan.IsCurrent, pausedPlan.Pinned)
	}

	enable := callManageUser(t, user.Id, fmt.Sprintf(`{"id":%d,"action":"enable"}`, user.Id))
	if !strings.Contains(enable.Body.String(), `"success":true`) {
		t.Fatalf("enable failed: %s", enable.Body.String())
	}
	var enabledUser model.User
	var resumedPlan model.UserPlan
	if err := db.First(&enabledUser, user.Id).Error; err != nil {
		t.Fatalf("reload enabled user: %v", err)
	}
	if err := db.First(&resumedPlan, plan.Id).Error; err != nil {
		t.Fatalf("reload resumed plan: %v", err)
	}
	if enabledUser.Status != common.UserStatusEnabled || resumedPlan.PausedAt != 0 || resumedPlan.ExpiresAt < originalExpiry || resumedPlan.Pinned != 1 {
		t.Fatalf("enabled status=%d paused=%d expires=%d pinned=%d", enabledUser.Status, resumedPlan.PausedAt, resumedPlan.ExpiresAt, resumedPlan.Pinned)
	}
}

func TestManageUser_PermanentBanIsReachableAndIdempotent(t *testing.T) {
	db := setupManageUserBanTest(t)
	user := &model.User{Username: "managed-ban", Password: "12345678", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	plan := &model.UserPlan{
		UserId: user.Id, Quota: 100, Status: model.UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}

	banBody := fmt.Sprintf(`{"id":%d,"action":"ban","reason":"abuse"}`, user.Id)
	ban := callManageUser(t, user.Id, banBody)
	if !strings.Contains(ban.Body.String(), `"success":true`) {
		t.Fatalf("ban failed: %s", ban.Body.String())
	}
	var bannedUser model.User
	var forfeited model.UserPlan
	if err := db.First(&bannedUser, user.Id).Error; err != nil {
		t.Fatalf("reload banned user: %v", err)
	}
	if err := db.First(&forfeited, plan.Id).Error; err != nil {
		t.Fatalf("reload forfeited plan: %v", err)
	}
	if bannedUser.Status != common.UserStatusBanned || forfeited.Status != model.UserPlanStatusForfeited || forfeited.IsCurrent != 0 || forfeited.Pinned != 0 {
		t.Fatalf("ban status=%d plan=(status=%d,current=%d,pinned=%d)", bannedUser.Status, forfeited.Status, forfeited.IsCurrent, forfeited.Pinned)
	}

	second := callManageUser(t, user.Id, banBody)
	if !strings.Contains(second.Body.String(), `"success":true`) {
		t.Fatalf("second ban failed: %s", second.Body.String())
	}
	var snapshots int64
	if err := db.Model(&model.UserAssetSnapshot{}).Where("user_id = ?", user.Id).Count(&snapshots).Error; err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if snapshots != 1 {
		t.Fatalf("snapshot count=%d, want 1", snapshots)
	}

	enable := callManageUser(t, user.Id, fmt.Sprintf(`{"id":%d,"action":"enable"}`, user.Id))
	if !strings.Contains(enable.Body.String(), `"success":false`) {
		t.Fatalf("permanent ban enabled without restore: %s", enable.Body.String())
	}
	if err := db.First(&bannedUser, user.Id).Error; err != nil {
		t.Fatalf("reload still-banned user: %v", err)
	}
	if bannedUser.Status != common.UserStatusBanned {
		t.Fatalf("status after rejected enable=%d", bannedUser.Status)
	}

	var snapshot model.UserAssetSnapshot
	if err := db.Where("user_id = ?", user.Id).First(&snapshot).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if err := service.RestoreFromSnapshot(snapshot.Id, &service.RestoreOptions{
		RestoreCurrentPlan: true,
	}, 99, "admin", "127.0.0.1"); err != nil {
		t.Fatalf("restore snapshot: %v", err)
	}
	enable = callManageUser(t, user.Id, fmt.Sprintf(`{"id":%d,"action":"enable"}`, user.Id))
	if !strings.Contains(enable.Body.String(), `"success":true`) {
		t.Fatalf("enable after restore failed: %s", enable.Body.String())
	}
	if err := db.First(&bannedUser, user.Id).Error; err != nil {
		t.Fatalf("reload restored user: %v", err)
	}
	if bannedUser.Status != common.UserStatusEnabled {
		t.Fatalf("status after restored enable=%d", bannedUser.Status)
	}
}

func TestManageUser_RetryRepairsStatusLifecycleGap(t *testing.T) {
	db := setupManageUserBanTest(t)
	now := time.Now()

	disabled := &model.User{
		Username: "managed-gap-disable", Password: "12345678", Role: common.RoleCommonUser,
		Status: common.UserStatusDisabled, AffCode: "managed-gap-disable",
	}
	if err := db.Create(disabled).Error; err != nil {
		t.Fatalf("create disabled user: %v", err)
	}
	disabledPlan := &model.UserPlan{
		UserId: disabled.Id, Quota: 100, Status: model.UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1, StartedAt: now.Add(-time.Hour).UnixMilli(), ExpiresAt: now.Add(time.Hour).UnixMilli(),
	}
	if err := db.Create(disabledPlan).Error; err != nil {
		t.Fatalf("create unpaused disabled plan: %v", err)
	}
	disable := callManageUser(t, disabled.Id, fmt.Sprintf(`{"id":%d,"action":"disable"}`, disabled.Id))
	if !strings.Contains(disable.Body.String(), `"success":true`) {
		t.Fatalf("disable repair failed: %s", disable.Body.String())
	}
	if err := db.First(disabledPlan, disabledPlan.Id).Error; err != nil {
		t.Fatalf("reload disabled plan: %v", err)
	}
	if disabledPlan.PausedAt == 0 || disabledPlan.Pinned != 1 {
		t.Fatalf("disable retry paused=%d pinned=%d", disabledPlan.PausedAt, disabledPlan.Pinned)
	}

	enabled := &model.User{
		Username: "managed-gap-enable", Password: "12345678", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, AffCode: "managed-gap-enable",
	}
	if err := db.Create(enabled).Error; err != nil {
		t.Fatalf("create enabled user: %v", err)
	}
	enabledPlan := &model.UserPlan{
		UserId: enabled.Id, Quota: 100, Status: model.UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1, StartedAt: now.Add(-time.Hour).UnixMilli(), ExpiresAt: now.Add(time.Hour).UnixMilli(),
		PausedAt: now.Add(-30 * time.Minute).UnixMilli(),
	}
	if err := db.Create(enabledPlan).Error; err != nil {
		t.Fatalf("create paused enabled plan: %v", err)
	}
	enable := callManageUser(t, enabled.Id, fmt.Sprintf(`{"id":%d,"action":"enable"}`, enabled.Id))
	if !strings.Contains(enable.Body.String(), `"success":true`) {
		t.Fatalf("enable repair failed: %s", enable.Body.String())
	}
	if err := db.First(enabledPlan, enabledPlan.Id).Error; err != nil {
		t.Fatalf("reload enabled plan: %v", err)
	}
	if enabledPlan.PausedAt != 0 || enabledPlan.Pinned != 1 {
		t.Fatalf("enable retry paused=%d pinned=%d", enabledPlan.PausedAt, enabledPlan.Pinned)
	}

	banned := &model.User{
		Username: "managed-gap-ban", Password: "12345678", Role: common.RoleCommonUser,
		Status: common.UserStatusBanned, AffCode: "managed-gap-ban",
	}
	if err := db.Create(banned).Error; err != nil {
		t.Fatalf("create banned user: %v", err)
	}
	bannedPlan := &model.UserPlan{
		UserId: banned.Id, Quota: 100, Status: model.UserPlanStatusActive, IsCurrent: 1, Pinned: 1,
	}
	if err := db.Create(bannedPlan).Error; err != nil {
		t.Fatalf("create active banned plan: %v", err)
	}
	ban := callManageUser(t, banned.Id, fmt.Sprintf(`{"id":%d,"action":"ban"}`, banned.Id))
	if !strings.Contains(ban.Body.String(), `"success":true`) {
		t.Fatalf("ban repair failed: %s", ban.Body.String())
	}
	if err := db.First(bannedPlan, bannedPlan.Id).Error; err != nil {
		t.Fatalf("reload banned plan: %v", err)
	}
	if bannedPlan.Status != model.UserPlanStatusForfeited || bannedPlan.Pinned != 0 {
		t.Fatalf("ban retry status=%d pinned=%d", bannedPlan.Status, bannedPlan.Pinned)
	}
	var snapshotCount int64
	if err := db.Model(&model.UserAssetSnapshot{}).Where("user_id = ?", banned.Id).Count(&snapshotCount).Error; err != nil {
		t.Fatalf("count repair snapshots: %v", err)
	}
	if snapshotCount != 1 {
		t.Fatalf("repair snapshot count=%d", snapshotCount)
	}
}
