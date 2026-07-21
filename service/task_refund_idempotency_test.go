package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func TestRefundTaskQuotaClaimsQuotaBeforeRefund(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Task{}, &model.Log{})
	oldLogDB := model.LOG_DB
	oldBatchUpdate := common.BatchUpdateEnabled
	oldMemoryCache := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	model.LOG_DB = db
	common.BatchUpdateEnabled = false
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
		common.BatchUpdateEnabled = oldBatchUpdate
		common.MemoryCacheEnabled = oldMemoryCache
		common.RedisEnabled = oldRedisEnabled
	})

	user := model.User{Username: "refund-user", Quota: 100, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID: "task_refund",
		UserId: user.Id,
		Status: model.TaskStatusFailure,
		Quota:  10,
		PrivateData: model.TaskPrivateData{
			BillingSource: BillingSourceUserBalance,
		},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	var first, second model.Task
	if err := db.First(&first, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&second, task.ID).Error; err != nil {
		t.Fatal(err)
	}

	if !RefundTaskQuota(context.Background(), &first, "failed") {
		t.Fatal("first refund failed")
	}
	if !RefundTaskQuota(context.Background(), &second, "failed") {
		t.Fatal("idempotent second refund should be a no-op success")
	}

	var gotUser model.User
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if gotUser.Quota != 110 {
		t.Fatalf("expected one refund to produce quota 110, got %d", gotUser.Quota)
	}
	var gotTask model.Task
	if err := db.First(&gotTask, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if gotTask.Quota != 0 {
		t.Fatalf("expected refund marker cleared, got %d", gotTask.Quota)
	}
}

func TestRefundUnpersistedTaskQuotaCompensatesCharge(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Log{})
	oldLogDB := model.LOG_DB
	oldBatchUpdate := common.BatchUpdateEnabled
	oldRedisEnabled := common.RedisEnabled
	model.LOG_DB = db
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
		common.BatchUpdateEnabled = oldBatchUpdate
		common.RedisEnabled = oldRedisEnabled
	})

	user := model.User{Username: "compensate-user", Quota: 90, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID: "task_unpersisted",
		UserId: user.Id,
		Quota:  10,
		PrivateData: model.TaskPrivateData{
			BillingSource: BillingSourceUserBalance,
		},
	}
	if !RefundUnpersistedTaskQuota(context.Background(), &task, "insert failed") {
		t.Fatal("compensation failed")
	}
	var got model.User
	if err := db.First(&got, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if got.Quota != 100 {
		t.Fatalf("expected compensated quota 100, got %d", got.Quota)
	}
}

func TestRefundUnpersistedTaskQuotaRollsBackWhenTokenRefundFails(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Token{}, &model.Log{})
	oldLogDB := model.LOG_DB
	oldRedisEnabled := common.RedisEnabled
	model.LOG_DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
	})

	user := model.User{Username: "unpersisted-atomic-user", Quota: 90, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	token := model.Token{UserId: user.Id, Key: "unpersisted-atomic-token", RemainQuota: 90, UsedQuota: 10, Status: common.TokenStatusEnabled}
	if err := db.Create(&token).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-unpersisted-token-refund", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Token" {
			tx.AddError(errors.New("forced token refund failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	trackedTokenQuota := 10
	task := model.Task{
		TaskID: "task_unpersisted_atomic",
		UserId: user.Id,
		Quota:  10,
		PrivateData: model.TaskPrivateData{
			BillingSource:     BillingSourceUserBalance,
			TokenId:           token.Id,
			TokenChargedQuota: &trackedTokenQuota,
			TokenQuotaEnabled: true,
		},
	}

	if RefundUnpersistedTaskQuota(context.Background(), &task, "insert failed") {
		t.Fatal("compensation must report failure when its transaction rolls back")
	}
	var gotUser model.User
	var gotToken model.Token
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotToken, token.Id).Error; err != nil {
		t.Fatal(err)
	}
	if gotUser.Quota != 90 || gotToken.RemainQuota != 90 || gotToken.UsedQuota != 10 {
		t.Fatalf("failed compensation partially committed: wallet=%d token_remain=%d token_used=%d",
			gotUser.Quota, gotToken.RemainQuota, gotToken.UsedQuota)
	}
}

func TestTaskBillingCompensationRetriesAfterTransactionalFailure(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Token{}, &model.TaskBillingCompensation{}, &model.Log{})
	oldLogDB := model.LOG_DB
	oldRedisEnabled := common.RedisEnabled
	model.LOG_DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
	})
	user := model.User{Username: "durable-compensation-user", Quota: 90, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	token := model.Token{UserId: user.Id, Key: "durable-compensation-token", RemainQuota: 90, UsedQuota: 10, Status: common.TokenStatusEnabled}
	if err := db.Create(&token).Error; err != nil {
		t.Fatal(err)
	}
	trackedTokenQuota := 10
	task := model.Task{
		TaskID: "task_durable_compensation",
		UserId: user.Id,
		Quota:  10,
		PrivateData: model.TaskPrivateData{
			BillingSource:     BillingSourceUserBalance,
			TokenId:           token.Id,
			TokenChargedQuota: &trackedTokenQuota,
			TokenQuotaEnabled: true,
		},
	}
	compensation, err := model.CreateTaskBillingCompensation(&task, "insert failed")
	if err != nil {
		t.Fatal(err)
	}
	const callbackName = "test:fail-durable-compensation-token-refund"
	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Token" {
			tx.AddError(errors.New("forced token refund failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if RefundTaskBillingCompensation(context.Background(), compensation) {
		t.Fatal("first compensation unexpectedly succeeded")
	}
	var pending model.TaskBillingCompensation
	var unchangedUser model.User
	if err := db.First(&pending, compensation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&unchangedUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if pending.Status != model.TaskBillingCompensationPending || unchangedUser.Quota != 90 {
		t.Fatalf("failed compensation was not retryable: status=%s wallet=%d", pending.Status, unchangedUser.Quota)
	}
	db.Callback().Update().Remove(callbackName)
	if !RefundTaskBillingCompensation(context.Background(), &pending) {
		t.Fatal("compensation retry failed")
	}
	var settled model.TaskBillingCompensation
	var gotUser model.User
	var gotToken model.Token
	if err := db.First(&settled, compensation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotToken, token.Id).Error; err != nil {
		t.Fatal(err)
	}
	if settled.Status != model.TaskBillingCompensationSettled || gotUser.Quota != 100 || gotToken.RemainQuota != 100 {
		t.Fatalf("compensation retry did not settle: status=%s wallet=%d token=%d", settled.Status, gotUser.Quota, gotToken.RemainQuota)
	}
}

func TestTaskBillingReconciliationSweepsCompensationWithoutAdaptor(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Task{}, &model.TaskBillingCompensation{}, &model.Log{})
	oldLogDB := model.LOG_DB
	oldRedisEnabled := common.RedisEnabled
	oldAdaptorFactory := GetTaskAdaptorFunc
	model.LOG_DB = db
	common.RedisEnabled = false
	GetTaskAdaptorFunc = nil
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
		GetTaskAdaptorFunc = oldAdaptorFactory
	})

	user := model.User{Username: "polling-compensation-user", Quota: 90, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID: "task_polling_compensation",
		UserId: user.Id,
		Quota:  10,
		PrivateData: model.TaskPrivateData{
			BillingSource: BillingSourceUserBalance,
		},
	}
	compensation, err := model.CreateTaskBillingCompensation(&task, "insert failed")
	if err != nil {
		t.Fatal(err)
	}

	RunTaskBillingReconciliationOnce(context.Background())

	var settled model.TaskBillingCompensation
	var gotUser model.User
	if err := db.First(&settled, compensation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if settled.Status != model.TaskBillingCompensationSettled || gotUser.Quota != 100 {
		t.Fatalf("polling did not settle compensation: status=%s wallet=%d", settled.Status, gotUser.Quota)
	}
}

func TestTaskBillingCompensationRetriesAccountingWithoutDoubleRefund(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.TaskBillingCompensation{}, &model.Log{})
	oldLogDB := model.LOG_DB
	oldRedisEnabled := common.RedisEnabled
	model.LOG_DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
	})

	user := model.User{Username: "compensation-accounting-user", Quota: 90, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID: "task_compensation_accounting",
		UserId: user.Id,
		Quota:  10,
		PrivateData: model.TaskPrivateData{
			BillingSource: BillingSourceUserBalance,
		},
	}
	compensation, err := model.CreateTaskBillingCompensation(&task, "insert failed")
	if err != nil {
		t.Fatal(err)
	}

	const callbackName = "test:fail-compensation-accounting-log"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Log" {
			tx.AddError(errors.New("forced compensation log failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if RefundTaskBillingCompensation(context.Background(), compensation) {
		t.Fatal("compensation unexpectedly completed with a failed audit log")
	}
	var accounting model.TaskBillingCompensation
	var refundedUser model.User
	if err := db.First(&accounting, compensation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&refundedUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if accounting.Status != model.TaskBillingCompensationAccounting || refundedUser.Quota != 100 {
		t.Fatalf("accounting failure lost retry state: status=%s wallet=%d", accounting.Status, refundedUser.Quota)
	}

	db.Callback().Create().Remove(callbackName)
	if !RefundTaskBillingCompensation(context.Background(), &accounting) {
		t.Fatal("compensation accounting retry failed")
	}
	if !RefundTaskBillingCompensation(context.Background(), &accounting) {
		t.Fatal("settled compensation should be an idempotent success")
	}
	var settled model.TaskBillingCompensation
	var finalUser model.User
	var logCount int64
	if err := db.First(&settled, compensation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&finalUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Log{}).Where("task_billing_compensation_id = ?", compensation.ID).Count(&logCount).Error; err != nil {
		t.Fatal(err)
	}
	if settled.Status != model.TaskBillingCompensationSettled || finalUser.Quota != 100 || logCount != 1 {
		t.Fatalf("accounting retry was not idempotent: status=%s wallet=%d logs=%d", settled.Status, finalUser.Quota, logCount)
	}
}

func TestTaskBillingCompensationBackoffPreventsQueueStarvation(t *testing.T) {
	setupTaskLifecycleDB(t, &model.TaskBillingCompensation{})
	compensations := make([]*model.TaskBillingCompensation, 0, 3)
	for i := 0; i < 3; i++ {
		compensation, err := model.CreateTaskBillingCompensation(&model.Task{
			TaskID: fmt.Sprintf("task_compensation_backoff_%d", i),
			UserId: 7,
			Quota:  1,
		}, "insert failed")
		if err != nil {
			t.Fatal(err)
		}
		compensations = append(compensations, compensation)
	}

	firstWindow := model.GetPendingTaskBillingCompensations(2)
	if len(firstWindow) != 2 {
		t.Fatalf("unexpected first compensation window: %d", len(firstWindow))
	}
	for _, compensation := range firstWindow {
		if err := model.DeferTaskBillingCompensation(compensation.ID, errors.New("poison compensation")); err != nil {
			t.Fatal(err)
		}
	}
	nextWindow := model.GetPendingTaskBillingCompensations(2)
	if len(nextWindow) != 1 || nextWindow[0].ID != compensations[2].ID {
		t.Fatalf("deferred rows starved newer compensation: %+v", nextWindow)
	}
}

func TestRefundTaskQuotaRetriesOnlyUnfinishedMixedRemainder(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.UserPlan{}, &model.Task{}, &model.Log{})
	oldLogDB := model.LOG_DB
	oldBatchUpdate := common.BatchUpdateEnabled
	oldMemoryCache := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	model.LOG_DB = db
	common.BatchUpdateEnabled = false
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
		common.BatchUpdateEnabled = oldBatchUpdate
		common.MemoryCacheEnabled = oldMemoryCache
		common.RedisEnabled = oldRedisEnabled
	})

	user := model.User{Username: "mixed-refund-user", Quota: 0, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	plan := model.UserPlan{UserId: user.Id, Quota: 0, Status: model.UserPlanStatusActive}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID: "task_mixed_refund",
		UserId: user.Id,
		Status: model.TaskStatusFailure,
		Quota:  100,
		PrivateData: model.TaskPrivateData{
			BillingSource:      BillingSourcePlanAndUserBalance,
			UserPlanId:         plan.Id,
			PlanChargedQuota:   60,
			WalletChargedQuota: 40,
		},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	failedOnce := false
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-first-plan-refund", func(tx *gorm.DB) {
		if !failedOnce && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "UserPlan" {
			failedOnce = true
			tx.AddError(errors.New("forced first plan refund failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}

	if RefundTaskQuota(context.Background(), &task, "failed") {
		t.Fatal("first refund should report the incomplete plan remainder")
	}
	var pending model.Task
	if err := db.First(&pending, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if pending.Quota != 100 {
		t.Fatalf("pending refund marker = %d, want the atomic full refund 100", pending.Quota)
	}
	if pending.PrivateData.WalletChargedQuota != 40 || pending.PrivateData.PlanChargedQuota != 60 {
		t.Fatalf("unexpected persisted split after partial refund: %+v", pending.PrivateData)
	}
	var beforeRetryUser model.User
	var beforeRetryPlan model.UserPlan
	if err := db.First(&beforeRetryUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&beforeRetryPlan, plan.Id).Error; err != nil {
		t.Fatal(err)
	}
	if beforeRetryUser.Quota != 0 || beforeRetryPlan.Quota != 0 {
		t.Fatalf("failed transaction leaked a partial refund: wallet=%d plan=%d", beforeRetryUser.Quota, beforeRetryPlan.Quota)
	}

	if !RefundTaskQuota(context.Background(), &pending, "retry") {
		t.Fatal("second refund should finish the plan remainder")
	}
	var gotUser model.User
	var gotPlan model.UserPlan
	var gotTask model.Task
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotPlan, plan.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotTask, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if gotUser.Quota != 40 || gotPlan.Quota != 60 || gotTask.Quota != 0 {
		t.Fatalf("refund totals mismatch: wallet=%d plan=%d pending=%d", gotUser.Quota, gotPlan.Quota, gotTask.Quota)
	}
}

func TestRefundTaskQuotaDoesNotRefundWalletWhenSplitSnapshotFails(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.UserPlan{}, &model.Task{}, &model.Log{})
	oldLogDB := model.LOG_DB
	oldBatchUpdate := common.BatchUpdateEnabled
	oldMemoryCache := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	model.LOG_DB = db
	common.BatchUpdateEnabled = false
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
		common.BatchUpdateEnabled = oldBatchUpdate
		common.MemoryCacheEnabled = oldMemoryCache
		common.RedisEnabled = oldRedisEnabled
	})

	user := model.User{Username: "atomic-refund-user", Quota: 0, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	plan := model.UserPlan{UserId: user.Id, Quota: 0, Status: model.UserPlanStatusActive}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID: "task_atomic_refund",
		UserId: user.Id,
		Status: model.TaskStatusFailure,
		Quota:  100,
		PrivateData: model.TaskPrivateData{
			BillingSource:      BillingSourcePlanAndUserBalance,
			UserPlanId:         plan.Id,
			PlanChargedQuota:   60,
			WalletChargedQuota: 40,
		},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	updates := 0
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-private-snapshot", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Task" {
			updates++
			if updates == 1 {
				tx.AddError(errors.New("forced split snapshot failure"))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}

	if RefundTaskQuota(context.Background(), &task, "failed") {
		t.Fatal("refund must remain pending when the atomic split write fails")
	}
	var gotUser model.User
	var gotTask model.Task
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotTask, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if gotUser.Quota != 0 || gotTask.Quota != 100 {
		t.Fatalf("atomic refund rolled back incompletely: wallet=%d pending=%d", gotUser.Quota, gotTask.Quota)
	}
}

func TestRefundTaskQuotaRollsBackFundingWhenWalletWriteReportsFailure(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Task{}, &model.Log{})
	oldLogDB := model.LOG_DB
	oldBatchUpdate := common.BatchUpdateEnabled
	oldMemoryCache := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	model.LOG_DB = db
	common.BatchUpdateEnabled = false
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
		common.BatchUpdateEnabled = oldBatchUpdate
		common.MemoryCacheEnabled = oldMemoryCache
		common.RedisEnabled = oldRedisEnabled
	})

	user := model.User{Username: "transaction-refund-user", Quota: 100, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID: "task_transaction_refund",
		UserId: user.Id,
		Status: model.TaskStatusFailure,
		Quota:  10,
		PrivateData: model.TaskPrivateData{
			BillingSource: BillingSourceUserBalance,
		},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	failedOnce := false
	if err := db.Callback().Update().After("gorm:update").Register("test:fail-after-wallet-refund", func(tx *gorm.DB) {
		if !failedOnce && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "User" {
			failedOnce = true
			tx.AddError(errors.New("forced post-write wallet failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}

	if RefundTaskQuota(context.Background(), &task, "failed") {
		t.Fatal("refund should remain pending after transactional wallet failure")
	}
	var gotUser model.User
	var gotTask model.Task
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotTask, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if gotUser.Quota != 100 || gotTask.Quota != 10 {
		t.Fatalf("refund was not atomic: wallet=%d pending=%d", gotUser.Quota, gotTask.Quota)
	}
}

func TestRefundTaskQuotaRollsBackOnTokenQueryFailure(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Token{}, &model.Task{}, &model.Log{})
	oldLogDB := model.LOG_DB
	oldBatchUpdate := common.BatchUpdateEnabled
	oldRedisEnabled := common.RedisEnabled
	model.LOG_DB = db
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
		common.BatchUpdateEnabled = oldBatchUpdate
		common.RedisEnabled = oldRedisEnabled
	})

	user := model.User{Username: "token-query-refund-user", Quota: 100, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID: "task_token_query_refund",
		UserId: user.Id,
		Status: model.TaskStatusFailure,
		Quota:  10,
		PrivateData: model.TaskPrivateData{
			BillingSource: BillingSourceUserBalance,
			TokenId:       999,
		},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Query().Before("gorm:query").Register("test:fail-token-query", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Token" {
			tx.AddError(errors.New("forced token query failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}

	if RefundTaskQuota(context.Background(), &task, "failed") {
		t.Fatal("refund should remain pending after token query failure")
	}
	var gotUser model.User
	var gotTask model.Task
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotTask, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if gotUser.Quota != 100 || gotTask.Quota != 10 {
		t.Fatalf("token query failure leaked refund: wallet=%d pending=%d", gotUser.Quota, gotTask.Quota)
	}
}

func TestRefundTaskQuotaUsesTrackedTokenCharge(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Token{}, &model.Task{}, &model.Log{})
	oldLogDB := model.LOG_DB
	oldBatchUpdate := common.BatchUpdateEnabled
	oldRedisEnabled := common.RedisEnabled
	model.LOG_DB = db
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
		common.BatchUpdateEnabled = oldBatchUpdate
		common.RedisEnabled = oldRedisEnabled
	})

	user := model.User{Username: "tracked-token-refund-user", Quota: 90, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	token := model.Token{UserId: user.Id, Key: "tracked-token-key", RemainQuota: 96, UsedQuota: 4, Status: common.TokenStatusEnabled}
	if err := db.Create(&token).Error; err != nil {
		t.Fatal(err)
	}
	trackedTokenQuota := 4
	task := model.Task{
		TaskID: "task_tracked_token_refund",
		UserId: user.Id,
		Status: model.TaskStatusFailure,
		Quota:  10,
		PrivateData: model.TaskPrivateData{
			BillingSource:     BillingSourceUserBalance,
			TokenId:           token.Id,
			TokenChargedQuota: &trackedTokenQuota,
			TokenQuotaEnabled: true,
		},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	if !RefundTaskQuota(context.Background(), &task, "failed") {
		t.Fatal("refund failed")
	}
	var gotToken model.Token
	if err := db.First(&gotToken, token.Id).Error; err != nil {
		t.Fatal(err)
	}
	if gotToken.RemainQuota != 100 || gotToken.UsedQuota != 0 {
		t.Fatalf("token refund used funding quota instead of tracked quota: remain=%d used=%d", gotToken.RemainQuota, gotToken.UsedQuota)
	}
}

func TestLegacyPlanTaskRefundDoesNotMintTokenQuota(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.UserPlan{}, &model.Token{}, &model.Task{}, &model.Log{})
	oldLogDB := model.LOG_DB
	oldRedisEnabled := common.RedisEnabled
	model.LOG_DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
	})

	plan := model.UserPlan{UserId: 17, Quota: 90, Status: model.UserPlanStatusActive}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	token := model.Token{UserId: 17, Key: "legacy-plan-token", RemainQuota: 100, UsedQuota: 0, Status: common.TokenStatusEnabled}
	if err := db.Create(&token).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID: "task_legacy_plan_refund",
		UserId: 17,
		Status: model.TaskStatusFailure,
		Quota:  10,
		PrivateData: model.TaskPrivateData{
			BillingSource: BillingSourcePlan,
			UserPlanId:    plan.Id,
			TokenId:       token.Id,
		},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	if !RefundTaskQuota(context.Background(), &task, "failed") {
		t.Fatal("refund failed")
	}
	var gotToken model.Token
	if err := db.First(&gotToken, token.Id).Error; err != nil {
		t.Fatal(err)
	}
	if gotToken.RemainQuota != 100 || gotToken.UsedQuota != 0 {
		t.Fatalf("legacy plan refund minted token quota: remain=%d used=%d", gotToken.RemainQuota, gotToken.UsedQuota)
	}
}
