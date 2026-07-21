package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBuildTaskFromSubmitKeepsPublicAndUpstreamIDs(t *testing.T) {
	info := &relaycommon.RelayInfo{
		UserId:            7,
		TokenId:           3,
		TokenSettledQuota: 7,
		UsingGroup:        "default",
		OriginModelName:   "sora-2",
		BillingSource:     service.BillingSourceDailyPool,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeSora,
			ChannelId:         11,
			ApiKey:            "selected-key",
			UpstreamModelName: "sora-2",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			Action:       "create",
			PublicTaskID: "task_public",
		},
	}

	task := buildTaskFromSubmit(info, &relay.TaskSubmitResult{
		Platform:       constant.TaskPlatform("sora"),
		UpstreamTaskID: "provider-123",
		Quota:          10,
	})
	if task.TaskID != "task_public" {
		t.Fatalf("expected public task ID, got %q", task.TaskID)
	}
	if task.PrivateData.UpstreamTaskID != "provider-123" {
		t.Fatalf("expected upstream ID in private data, got %q", task.PrivateData.UpstreamTaskID)
	}
	if task.PrivateData.Key != "selected-key" {
		t.Fatalf("expected selected key in private data, got %q", task.PrivateData.Key)
	}
	if task.PrivateData.DailyPoolDate != model.GetTodayDate() {
		t.Fatalf("expected daily-pool billing date snapshot, got %q", task.PrivateData.DailyPoolDate)
	}
	if task.PrivateData.TokenChargedQuota == nil || *task.PrivateData.TokenChargedQuota != 7 || !task.PrivateData.TokenQuotaEnabled {
		t.Fatalf("unexpected token billing snapshot: %+v", task.PrivateData)
	}
}

func TestBuildTaskFromSubmitDoesNotEnableUnlimitedTokenBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		UserId:         7,
		TokenId:        3,
		TokenUnlimited: true,
		ChannelMeta:    &relaycommon.ChannelMeta{},
		TaskRelayInfo:  &relaycommon.TaskRelayInfo{PublicTaskID: "task_unlimited_token"},
	}
	task := buildTaskFromSubmit(info, &relay.TaskSubmitResult{Quota: 10})
	if task.PrivateData.TokenQuotaEnabled {
		t.Fatal("unlimited token must not be enabled for deferred task token billing")
	}
}

func TestBuildTaskFromSubmitTracksActuallyDebitedUnlimitedToken(t *testing.T) {
	info := &relaycommon.RelayInfo{
		UserId:            7,
		TokenId:           3,
		TokenUnlimited:    true,
		TokenSettledQuota: 10,
		ChannelMeta:       &relaycommon.ChannelMeta{},
		TaskRelayInfo:     &relaycommon.TaskRelayInfo{PublicTaskID: "task_debited_unlimited_token"},
	}
	task := buildTaskFromSubmit(info, &relay.TaskSubmitResult{Quota: 10})
	if !task.PrivateData.TokenQuotaEnabled || task.PrivateData.TokenChargedQuota == nil || *task.PrivateData.TokenChargedQuota != 10 {
		t.Fatalf("actual unlimited-token debit was not snapshotted: %+v", task.PrivateData)
	}
}

func TestFinalizeTaskSubmitReturnsInsertError(t *testing.T) {
	dsn := fmt.Sprintf("file:finalize_task_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Task{}, &model.TaskBillingCompensation{}, &model.User{}, &model.Log{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Create().Before("gorm:create").Register("test:fail-task-insert", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Task" {
			tx.AddError(errors.New("forced task insert failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldLogEnabled := common.LogConsumeEnabled
	oldBatchUpdate := common.BatchUpdateEnabled
	oldRedisEnabled := common.RedisEnabled
	oldMemoryCache := common.MemoryCacheEnabled
	model.DB = db
	model.LOG_DB = db
	common.LogConsumeEnabled = false
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.LogConsumeEnabled = oldLogEnabled
		common.BatchUpdateEnabled = oldBatchUpdate
		common.RedisEnabled = oldRedisEnabled
		common.MemoryCacheEnabled = oldMemoryCache
	})

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	user := model.User{Username: "finalize-user", Quota: 90, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	info := &relaycommon.RelayInfo{
		UserId:                user.Id,
		UserQuota:             1_000_000_000,
		IsPlayground:          true,
		BillingSource:         service.BillingSourceUserBalance,
		FinalPreConsumedQuota: 10,
		ChannelMeta:           &relaycommon.ChannelMeta{},
		TaskRelayInfo:         &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		OriginModelName:       "sora-2",
	}

	err = finalizeTaskSubmit(c, info, &relay.TaskSubmitResult{Platform: constant.TaskPlatform("sora"), Quota: 10})
	if err == nil || err.Error() != "forced task insert failure" {
		t.Fatalf("expected insert error, got %v", err)
	}
	var gotUser model.User
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if gotUser.Quota != 100 {
		t.Fatalf("failed task insert left pre-consumed quota charged: got %d, want 100", gotUser.Quota)
	}
	var compensation model.TaskBillingCompensation
	if err := db.Where("task_id = ?", "task_public").First(&compensation).Error; err != nil {
		t.Fatal(err)
	}
	if compensation.Status != model.TaskBillingCompensationSettled {
		t.Fatalf("compensation status = %s, want settled", compensation.Status)
	}
}

func TestFinalizeTaskSubmitDoesNotDirectRefundAmbiguousCompensationInsert(t *testing.T) {
	dsn := fmt.Sprintf("file:ambiguous_task_compensation_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Task{}, &model.TaskBillingCompensation{}, &model.User{}, &model.Log{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Create().Before("gorm:create").Register("test:fail-task-and-compensation-insert", func(tx *gorm.DB) {
		if tx.Statement.Schema == nil {
			return
		}
		if tx.Statement.Schema.Name == "Task" || tx.Statement.Schema.Name == "TaskBillingCompensation" {
			tx.AddError(errors.New("forced ambiguous insert failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldLogEnabled := common.LogConsumeEnabled
	oldBatchUpdate := common.BatchUpdateEnabled
	oldRedisEnabled := common.RedisEnabled
	oldMemoryCache := common.MemoryCacheEnabled
	model.DB = db
	model.LOG_DB = db
	common.LogConsumeEnabled = false
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.LogConsumeEnabled = oldLogEnabled
		common.BatchUpdateEnabled = oldBatchUpdate
		common.RedisEnabled = oldRedisEnabled
		common.MemoryCacheEnabled = oldMemoryCache
	})

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	user := model.User{Username: "ambiguous-compensation-user", Quota: 90, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	info := &relaycommon.RelayInfo{
		UserId:                user.Id,
		UserQuota:             1_000_000_000,
		IsPlayground:          true,
		BillingSource:         service.BillingSourceUserBalance,
		FinalPreConsumedQuota: 10,
		ChannelMeta:           &relaycommon.ChannelMeta{},
		TaskRelayInfo:         &relaycommon.TaskRelayInfo{PublicTaskID: "task_ambiguous_compensation"},
		OriginModelName:       "sora-2",
	}

	err = finalizeTaskSubmit(c, info, &relay.TaskSubmitResult{Platform: constant.TaskPlatform("sora"), Quota: 10})
	if err == nil || !strings.Contains(err.Error(), "compensation outbox state unknown") {
		t.Fatalf("expected ambiguous compensation error, got %v", err)
	}
	var gotUser model.User
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if gotUser.Quota != 90 {
		t.Fatalf("ambiguous outbox insert triggered unsafe direct refund: got %d, want 90", gotUser.Quota)
	}
}

func TestFinalizeTaskSubmitRecoversCommittedTaskAfterLostInsertAck(t *testing.T) {
	dsn := fmt.Sprintf("file:committed_task_lost_ack_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Task{}, &model.TaskBillingCompensation{}, &model.User{}, &model.Log{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Create().After("gorm:create").Register("test:lose-task-insert-ack", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Task" {
			tx.AddError(errors.New("lost task insert acknowledgement"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldLogEnabled := common.LogConsumeEnabled
	oldRedisEnabled := common.RedisEnabled
	model.DB = db
	model.LOG_DB = db
	common.LogConsumeEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.LogConsumeEnabled = oldLogEnabled
		common.RedisEnabled = oldRedisEnabled
	})

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	user := model.User{Username: "lost-ack-user", Quota: 90, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	info := &relaycommon.RelayInfo{
		UserId:                user.Id,
		UserQuota:             1_000_000_000,
		IsPlayground:          true,
		BillingSource:         service.BillingSourceUserBalance,
		FinalPreConsumedQuota: 10,
		ChannelMeta:           &relaycommon.ChannelMeta{},
		TaskRelayInfo:         &relaycommon.TaskRelayInfo{PublicTaskID: "task_committed_lost_ack"},
		OriginModelName:       "sora-2",
	}

	if err := finalizeTaskSubmit(c, info, &relay.TaskSubmitResult{Platform: constant.TaskPlatform("sora"), Quota: 10}); err != nil {
		t.Fatalf("committed task was treated as failed: %v", err)
	}
	var taskCount int64
	var compensationCount int64
	var gotUser model.User
	if err := db.Model(&model.Task{}).Where("task_id = ?", "task_committed_lost_ack").Count(&taskCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.TaskBillingCompensation{}).Count(&compensationCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 || compensationCount != 0 || gotUser.Quota != 90 {
		t.Fatalf("lost ACK caused duplicate recovery: tasks=%d compensations=%d wallet=%d", taskCount, compensationCount, gotUser.Quota)
	}
}

func TestFinalizeTaskSubmitRecoversCommittedFailureTaskAfterLostInsertAck(t *testing.T) {
	dsn := fmt.Sprintf("file:committed_failure_task_lost_ack_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Task{}, &model.TaskBillingCompensation{}, &model.User{}, &model.Log{}); err != nil {
		t.Fatal(err)
	}
	taskCreateAttempt := 0
	if err := db.Callback().Create().Before("gorm:create").Register("test:fail-first-task-insert", func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Name != "Task" {
			return
		}
		taskCreateAttempt++
		if taskCreateAttempt == 1 {
			tx.AddError(errors.New("forced first task insert failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Create().After("gorm:create").Register("test:lose-recovery-task-insert-ack", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Task" && taskCreateAttempt == 2 {
			tx.AddError(errors.New("lost recovery task insert acknowledgement"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldLogEnabled := common.LogConsumeEnabled
	oldBatchUpdate := common.BatchUpdateEnabled
	oldRedisEnabled := common.RedisEnabled
	oldMemoryCache := common.MemoryCacheEnabled
	model.DB = db
	model.LOG_DB = db
	common.LogConsumeEnabled = false
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.LogConsumeEnabled = oldLogEnabled
		common.BatchUpdateEnabled = oldBatchUpdate
		common.RedisEnabled = oldRedisEnabled
		common.MemoryCacheEnabled = oldMemoryCache
	})

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	user := model.User{Username: "lost-recovery-ack-user", Quota: 90, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	info := &relaycommon.RelayInfo{
		UserId:                user.Id,
		UserQuota:             1_000_000_000,
		IsPlayground:          true,
		BillingSource:         service.BillingSourceUserBalance,
		FinalPreConsumedQuota: 10,
		ChannelMeta:           &relaycommon.ChannelMeta{},
		TaskRelayInfo:         &relaycommon.TaskRelayInfo{PublicTaskID: "task_committed_failure_lost_ack"},
		OriginModelName:       "sora-2",
	}

	err = finalizeTaskSubmit(c, info, &relay.TaskSubmitResult{Platform: constant.TaskPlatform("sora"), Quota: 10})
	if err == nil || err.Error() != "forced first task insert failure" {
		t.Fatalf("expected original insert failure, got %v", err)
	}
	var persisted model.Task
	var compensationCount int64
	var gotUser model.User
	if err := db.Where("task_id = ?", "task_committed_failure_lost_ack").First(&persisted).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.TaskBillingCompensation{}).Count(&compensationCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status != model.TaskStatusFailure || persisted.Quota != 0 || compensationCount != 0 || gotUser.Quota != 100 {
		t.Fatalf("lost recovery ACK was not idempotent: status=%s quota=%d compensations=%d wallet=%d",
			persisted.Status, persisted.Quota, compensationCount, gotUser.Quota)
	}
}

func TestBuildTaskFromSubmitUsesActuallySettledPartialQuota(t *testing.T) {
	info := &relaycommon.RelayInfo{
		UserId:              7,
		BillingSource:       service.BillingSourcePlan,
		UserPlanId:          9,
		FundingSettledQuota: 60,
		ChannelMeta:         &relaycommon.ChannelMeta{},
		TaskRelayInfo:       &relaycommon.TaskRelayInfo{PublicTaskID: "task_partial"},
	}
	task := buildTaskFromSubmit(info, &relay.TaskSubmitResult{Quota: 100})
	if task.Quota != 60 {
		t.Fatalf("task refund marker = %d, want actually charged quota 60", task.Quota)
	}
}

func TestBuildTaskFromSubmitUsesExactExceptionalMixedSettlement(t *testing.T) {
	info := &relaycommon.RelayInfo{
		UserId:                      7,
		BillingSource:               service.BillingSourcePlanAndUserBalance,
		UserPlanId:                  9,
		PlanPreConsumeQuota:         100,
		UserBalancePreConsumedQuota: 50,
		FundingSettledQuota:         150,
		FundingSettledPlanQuota:     100,
		FundingSettledWalletQuota:   50,
		ChannelMeta:                 &relaycommon.ChannelMeta{},
		TaskRelayInfo:               &relaycommon.TaskRelayInfo{PublicTaskID: "task_partial_mixed"},
	}
	task := buildTaskFromSubmit(info, &relay.TaskSubmitResult{Quota: 130})
	if task.Quota != 150 {
		t.Fatalf("task refund marker = %d, want actually charged quota 150", task.Quota)
	}
	if task.PrivateData.PlanChargedQuota != 100 || task.PrivateData.WalletChargedQuota != 50 {
		t.Fatalf("task billing split = plan %d + wallet %d, want plan 100 + wallet 50",
			task.PrivateData.PlanChargedQuota, task.PrivateData.WalletChargedQuota)
	}
}
