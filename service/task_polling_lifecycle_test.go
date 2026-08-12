package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type lifecycleTestAdaptor struct {
	adjustCalls   int
	adjustQuota   int
	parseCalls    int
	parseResult   *relaycommon.TaskInfo
	fetchCalls    int
	fetchBaseURL  string
	fetchKey      string
	fetchProxy    string
	fetchBody     map[string]any
	fetchResponse string
	requireFetch  bool
	parsedBody    string
}

func (a *lifecycleTestAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *lifecycleTestAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string, extraHeaders http.Header) (*http.Response, error) {
	a.fetchCalls++
	a.fetchBaseURL = baseURL
	a.fetchKey = key
	a.fetchProxy = proxy
	a.fetchBody = body
	responseBody := a.fetchResponse
	if responseBody == "" {
		responseBody = `{"code":"success","data":{"status":"SUCCESS","progress":"100%"}}`
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(responseBody))}, nil
}
func (a *lifecycleTestAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	a.parseCalls++
	a.parsedBody = string(body)
	if a.requireFetch && a.fetchCalls == 0 {
		return nil, fmt.Errorf("polling transport was not initialized by FetchTask")
	}
	return a.parseResult, nil
}

func TestRecoverSuccessfulTaskSettlementFromCalculatingState(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Task{}, &model.Channel{}, &model.Log{})
	oldBatchUpdate := common.BatchUpdateEnabled
	oldRedisEnabled := common.RedisEnabled
	oldLogDB := model.LOG_DB
	oldFactory := GetTaskAdaptorFunc
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	model.LOG_DB = db
	t.Cleanup(func() {
		common.BatchUpdateEnabled = oldBatchUpdate
		common.RedisEnabled = oldRedisEnabled
		model.LOG_DB = oldLogDB
		GetTaskAdaptorFunc = oldFactory
	})

	user := model.User{Username: "recover-settlement-user", Quota: 100, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	channel := model.Channel{Name: "recover-channel", Type: 1, Key: "current-key", Status: common.ChannelStatusEnabled}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID:        "task_recover_calculating",
		Platform:      constant.TaskPlatform("video"),
		UserId:        user.Id,
		ChannelId:     channel.Id,
		Status:        model.TaskStatusSuccess,
		Progress:      "100%",
		Quota:         10,
		BillingStatus: model.TaskBillingCalculating,
		Data:          []byte(`{"code":"success","data":{"status":"SUCCESS","progress":"100%"}}`),
		PrivateData: model.TaskPrivateData{
			Key:           "submitted-key",
			BillingSource: BillingSourceUserBalance,
		},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	adaptor := &lifecycleTestAdaptor{
		adjustQuota: 20,
	}
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }

	if err := recoverSuccessfulTaskSettlement(context.Background(), &task); err != nil {
		t.Fatal(err)
	}
	var gotTask model.Task
	var gotUser model.User
	if err := db.First(&gotTask, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if adaptor.parseCalls != 0 || adaptor.adjustCalls != 1 {
		t.Fatalf("recovery calls parse=%d adjust=%d, want envelope parse and one adjustment", adaptor.parseCalls, adaptor.adjustCalls)
	}
	if gotTask.BillingStatus != model.TaskBillingSettled || gotTask.Quota != 20 || gotUser.Quota != 90 {
		t.Fatalf("recovery did not settle exactly once: task=%+v wallet=%d", gotTask, gotUser.Quota)
	}
}

func TestRecoverSuccessfulTaskSettlementMirrorsPollingTransport(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Task{}, &model.Channel{}, &model.Log{})
	oldRedisEnabled := common.RedisEnabled
	oldLogDB := model.LOG_DB
	oldFactory := GetTaskAdaptorFunc
	common.RedisEnabled = false
	model.LOG_DB = db
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		model.LOG_DB = oldLogDB
		GetTaskAdaptorFunc = oldFactory
	})

	user := model.User{Username: "recover-transport-user", Quota: 100, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	setting := `{"proxy":"http://saved-proxy.example"}`
	channel := model.Channel{
		Name:    "recover-transport-channel",
		Type:    constant.ChannelTypeMiniMax,
		Key:     "current-channel-key",
		Setting: &setting,
		Status:  common.ChannelStatusEnabled,
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID:        "task_recover_transport",
		Platform:      constant.TaskPlatform("video"),
		UserId:        user.Id,
		ChannelId:     channel.Id,
		Status:        model.TaskStatusSuccess,
		Progress:      "100%",
		Quota:         10,
		BillingStatus: model.TaskBillingCalculating,
		Action:        constant.TaskActionGenerate,
		Data:          []byte(`{"provider":"stored-terminal-response"}`),
		PrivateData: model.TaskPrivateData{
			Key:            "selected-task-key",
			UpstreamTaskID: "provider-task-id",
			BillingSource:  BillingSourceUserBalance,
		},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	adaptor := &lifecycleTestAdaptor{
		adjustQuota:   10,
		parseResult:   &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess), Progress: "100%"},
		fetchResponse: `{"provider":"fresh-terminal-response"}`,
		requireFetch:  true,
	}
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }

	if err := recoverSuccessfulTaskSettlement(context.Background(), &task); err != nil {
		t.Fatalf("recover settlement with polling transport: %v", err)
	}
	if adaptor.fetchCalls != 1 {
		t.Fatalf("FetchTask calls = %d, want 1", adaptor.fetchCalls)
	}
	if adaptor.fetchBaseURL != constant.ChannelBaseURLs[constant.ChannelTypeMiniMax] {
		t.Fatalf("base URL = %q, want default %q", adaptor.fetchBaseURL, constant.ChannelBaseURLs[constant.ChannelTypeMiniMax])
	}
	if adaptor.fetchKey != "selected-task-key" || adaptor.fetchProxy != "http://saved-proxy.example" {
		t.Fatalf("polling transport key=%q proxy=%q", adaptor.fetchKey, adaptor.fetchProxy)
	}
	if adaptor.fetchBody["task_id"] != "provider-task-id" || adaptor.fetchBody["action"] != constant.TaskActionGenerate {
		t.Fatalf("unexpected polling body: %+v", adaptor.fetchBody)
	}
	if adaptor.parsedBody != `{"provider":"stored-terminal-response"}` {
		t.Fatalf("parsed response = %q, want stored terminal response", adaptor.parsedBody)
	}

	customBaseURL := "https://custom-minimax.example/v1"
	customChannel := model.Channel{
		Name:    "recover-custom-base-url-channel",
		Type:    constant.ChannelTypeMiniMax,
		Key:     "custom-channel-key",
		BaseURL: &customBaseURL,
		Status:  common.ChannelStatusEnabled,
	}
	if err := db.Create(&customChannel).Error; err != nil {
		t.Fatal(err)
	}
	customTask := model.Task{
		TaskID:        "task_recover_custom_base_url",
		Platform:      constant.TaskPlatform("video"),
		UserId:        user.Id,
		ChannelId:     customChannel.Id,
		Status:        model.TaskStatusSuccess,
		Progress:      "100%",
		Quota:         10,
		BillingStatus: model.TaskBillingCalculating,
		Action:        constant.TaskActionGenerate,
		Data:          []byte(`{"provider":"stored-terminal-response"}`),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "provider-custom-task-id",
			BillingSource:  BillingSourceUserBalance,
		},
	}
	if err := db.Create(&customTask).Error; err != nil {
		t.Fatal(err)
	}
	customAdaptor := &lifecycleTestAdaptor{
		adjustQuota:  10,
		parseResult:  &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess), Progress: "100%"},
		requireFetch: true,
	}
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return customAdaptor }
	if err := recoverSuccessfulTaskSettlement(context.Background(), &customTask); err != nil {
		t.Fatalf("recover settlement with custom base URL: %v", err)
	}
	if customAdaptor.fetchBaseURL != customBaseURL {
		t.Fatalf("base URL = %q, want custom %q", customAdaptor.fetchBaseURL, customBaseURL)
	}
}

func TestRecoverSuccessfulTaskAccountingExactlyOnce(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Task{}, &model.Channel{}, &model.Log{})
	oldBatchUpdate := common.BatchUpdateEnabled
	oldRedisEnabled := common.RedisEnabled
	oldLogEnabled := common.LogConsumeEnabled
	oldLogDB := model.LOG_DB
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	common.LogConsumeEnabled = true
	model.LOG_DB = db
	t.Cleanup(func() {
		common.BatchUpdateEnabled = oldBatchUpdate
		common.RedisEnabled = oldRedisEnabled
		common.LogConsumeEnabled = oldLogEnabled
		model.LOG_DB = oldLogDB
	})

	user := model.User{Username: "recover-accounting-user", Status: common.UserStatusEnabled, RequestCount: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	channel := model.Channel{Name: "recover-accounting-channel", Status: common.ChannelStatusEnabled}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID:        "task_recover_accounting",
		UserId:        user.Id,
		ChannelId:     channel.Id,
		Status:        model.TaskStatusSuccess,
		Quota:         20,
		PendingQuota:  10,
		BillingStatus: model.TaskBillingAccounting,
		PrivateData: model.TaskPrivateData{
			BillingSource: BillingSourceUserBalance,
			BillingContext: &model.TaskBillingContext{
				OriginModelName: "accounting-model",
			},
		},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		TaskBillingTaskId: task.ID,
		UserId:            user.Id,
		LogType:           model.LogTypeConsume,
		Content:           "异步任务终态差额结算",
		ChannelId:         channel.Id,
		Quota:             10,
	}); err != nil {
		t.Fatal(err)
	}

	if err := recoverSuccessfulTaskSettlement(context.Background(), &task); err != nil {
		t.Fatal(err)
	}
	if err := recoverSuccessfulTaskSettlement(context.Background(), &task); err != nil {
		t.Fatal(err)
	}
	var gotTask model.Task
	var gotUser model.User
	var gotChannel model.Channel
	var logCount int64
	if err := db.First(&gotTask, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotChannel, channel.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Log{}).Where("content = ?", "异步任务终态差额结算").Count(&logCount).Error; err != nil {
		t.Fatal(err)
	}
	if gotTask.BillingStatus != model.TaskBillingSettled || gotTask.PendingQuota != 0 {
		t.Fatalf("accounting task was not completed: status=%s pending=%d", gotTask.BillingStatus, gotTask.PendingQuota)
	}
	if gotUser.UsedQuota != 10 || gotUser.RequestCount != 1 || gotChannel.UsedQuota != 10 || logCount != 1 {
		t.Fatalf("accounting side effects were not exactly once: user_quota=%d requests=%d channel=%d logs=%d",
			gotUser.UsedQuota, gotUser.RequestCount, gotChannel.UsedQuota, logCount)
	}
}

func (a *lifecycleTestAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	a.adjustCalls++
	return a.adjustQuota
}

func TestSuccessfulTaskSettlementRollsBackAndRemainsRetryable(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Task{}, &model.Channel{}, &model.Log{})
	oldBatchUpdate := common.BatchUpdateEnabled
	oldRedisEnabled := common.RedisEnabled
	oldLogDB := model.LOG_DB
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	model.LOG_DB = db
	t.Cleanup(func() {
		common.BatchUpdateEnabled = oldBatchUpdate
		common.RedisEnabled = oldRedisEnabled
		model.LOG_DB = oldLogDB
	})

	user := model.User{Username: "settlement-user", Quota: 100, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID:   "task_settlement",
		UserId:   user.Id,
		Status:   model.TaskStatusInProgress,
		Progress: "50%",
		Quota:    10,
		PrivateData: model.TaskPrivateData{
			BillingSource: BillingSourceUserBalance,
			BillingContext: &model.TaskBillingContext{
				OriginModelName: "ratio-task",
			},
		},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	failedOnce := false
	if err := db.Callback().Update().After("gorm:update").Register("test:fail-after-settlement-wallet-write", func(tx *gorm.DB) {
		if !failedOnce && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "User" {
			failedOnce = true
			tx.AddError(fmt.Errorf("forced post-write settlement failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}

	adaptor := &lifecycleTestAdaptor{adjustQuota: 20}
	won, err := ApplyTaskResult(context.Background(), adaptor, &task, &relaycommon.TaskInfo{
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
	}, nil)
	if !won || err == nil {
		t.Fatalf("terminal CAS should win while settlement remains pending: won=%v err=%v", won, err)
	}

	var pending model.Task
	var unchangedUser model.User
	if err := db.First(&pending, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&unchangedUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if pending.Status != model.TaskStatusSuccess || pending.BillingStatus != model.TaskBillingPending || pending.PendingQuota != 20 {
		t.Fatalf("unexpected pending settlement state: %+v", pending)
	}
	if pending.Quota != 10 || unchangedUser.Quota != 100 {
		t.Fatalf("failed settlement leaked funding changes: task_quota=%d wallet=%d", pending.Quota, unchangedUser.Quota)
	}

	if err := settlePendingTaskBilling(context.Background(), &pending); err != nil {
		t.Fatalf("retry pending settlement: %v", err)
	}
	var settled model.Task
	var chargedUser model.User
	if err := db.First(&settled, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&chargedUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if settled.BillingStatus != model.TaskBillingSettled || settled.PendingQuota != 0 || settled.Quota != 20 || chargedUser.Quota != 90 {
		t.Fatalf("retry did not settle exactly once: task=%+v wallet=%d", settled, chargedUser.Quota)
	}
}

func TestSuccessfulTaskPlanRefundKeepsHistoricalUsedQuota(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.UserPlan{}, &model.Task{}, &model.Log{})
	oldBatchUpdate := common.BatchUpdateEnabled
	oldRedisEnabled := common.RedisEnabled
	oldLogDB := model.LOG_DB
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	model.LOG_DB = db
	t.Cleanup(func() {
		common.BatchUpdateEnabled = oldBatchUpdate
		common.RedisEnabled = oldRedisEnabled
		model.LOG_DB = oldLogDB
	})

	plan := model.UserPlan{
		UserId:    42,
		Quota:     90,
		UsedQuota: 10,
		Status:    model.UserPlanStatusActive,
		StartedAt: time.Now().Add(-time.Hour).UnixMilli(),
		ExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
	}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID:        "task_plan_refund",
		UserId:        plan.UserId,
		Status:        model.TaskStatusSuccess,
		Quota:         10,
		BillingStatus: model.TaskBillingPending,
		PendingQuota:  5,
		PrivateData: model.TaskPrivateData{
			BillingSource: BillingSourcePlan,
			UserPlanId:    plan.Id,
		},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	if err := settlePendingTaskBilling(context.Background(), &task); err != nil {
		t.Fatal(err)
	}
	var got model.UserPlan
	if err := db.First(&got, plan.Id).Error; err != nil {
		t.Fatal(err)
	}
	if got.Quota != 95 {
		t.Fatalf("plan quota = %d, want 95", got.Quota)
	}
	if got.UsedQuota != 10 {
		t.Fatalf("historical used quota = %d, want 10", got.UsedQuota)
	}
}

func TestSuccessfulZeroQuotaTaskSettles(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.Task{})
	task := model.Task{
		TaskID:        "task_zero_quota",
		Status:        model.TaskStatusInProgress,
		Progress:      "50%",
		Quota:         0,
		BillingStatus: model.TaskBillingWaiting,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	won, err := ApplyTaskResult(context.Background(), &lifecycleTestAdaptor{}, &task, &relaycommon.TaskInfo{
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
	}, nil)
	if err != nil || !won {
		t.Fatalf("zero-quota success should settle: won=%v err=%v", won, err)
	}
	var got model.Task
	if err := db.First(&got, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.BillingStatus != model.TaskBillingSettled || got.PendingQuota != 0 || got.Quota != 0 {
		t.Fatalf("unexpected zero-quota settlement: %+v", got)
	}
}

func TestSuccessfulTaskSettlementUsesTrackedTokenBaseline(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Token{}, &model.Task{}, &model.Channel{}, &model.Log{})
	oldBatchUpdate := common.BatchUpdateEnabled
	oldRedisEnabled := common.RedisEnabled
	oldLogDB := model.LOG_DB
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	model.LOG_DB = db
	t.Cleanup(func() {
		common.BatchUpdateEnabled = oldBatchUpdate
		common.RedisEnabled = oldRedisEnabled
		model.LOG_DB = oldLogDB
	})

	user := model.User{Username: "settlement-token-user", Quota: 100, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	token := model.Token{UserId: user.Id, Key: "settlement-token-key", RemainQuota: 96, UsedQuota: 4, Status: common.TokenStatusEnabled}
	if err := db.Create(&token).Error; err != nil {
		t.Fatal(err)
	}
	trackedTokenQuota := 4
	task := model.Task{
		TaskID:        "task_settlement_token_baseline",
		UserId:        user.Id,
		Status:        model.TaskStatusSuccess,
		Quota:         10,
		BillingStatus: model.TaskBillingPending,
		PendingQuota:  20,
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

	if err := settlePendingTaskBilling(context.Background(), &task); err != nil {
		t.Fatal(err)
	}
	var gotToken model.Token
	if err := db.First(&gotToken, token.Id).Error; err != nil {
		t.Fatal(err)
	}
	if gotToken.RemainQuota != 80 || gotToken.UsedQuota != 20 {
		t.Fatalf("token settlement did not use tracked baseline: remain=%d used=%d", gotToken.RemainQuota, gotToken.UsedQuota)
	}
}

func setupTaskLifecycleDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:task_lifecycle_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB })
	return db
}

func TestApplyTaskResultSettlesOnlyForCASWinner(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.Task{})
	task := model.Task{
		TaskID:   "task_public",
		Platform: constant.TaskPlatform("video"),
		Status:   model.TaskStatusInProgress,
		Progress: "50%",
		Quota:    10,
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

	adaptor := &lifecycleTestAdaptor{}
	result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, Progress: "100%", Url: "https://example.com/video.mp4"}
	won, err := ApplyTaskResult(context.Background(), adaptor, &first, result, nil)
	if err != nil || !won {
		t.Fatalf("first transition should win: won=%v err=%v", won, err)
	}
	won, err = ApplyTaskResult(context.Background(), adaptor, &second, result, nil)
	if err != nil {
		t.Fatal(err)
	}
	if won {
		t.Fatal("stale second transition unexpectedly won CAS")
	}
	if adaptor.adjustCalls != 1 {
		t.Fatalf("expected exactly one settlement call, got %d", adaptor.adjustCalls)
	}
}

func TestChannelLookupFailureDoesNotOverwriteTerminalTask(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, int, []string, map[string]*model.Task) error
	}{
		{
			name: "video",
			run: func(ctx context.Context, channelID int, taskIDs []string, tasks map[string]*model.Task) error {
				return updateVideoTasks(ctx, constant.TaskPlatform("video"), channelID, taskIDs, tasks)
			},
		},
		{name: "suno", run: updateSunoTasks},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTaskLifecycleDB(t, &model.Task{}, &model.Channel{})
			oldCache := common.MemoryCacheEnabled
			common.MemoryCacheEnabled = false
			t.Cleanup(func() { common.MemoryCacheEnabled = oldCache })

			persisted := model.Task{
				TaskID:   "task_" + tt.name,
				Status:   model.TaskStatusSuccess,
				Progress: "100%",
				PrivateData: model.TaskPrivateData{
					UpstreamTaskID: "upstream-" + tt.name,
				},
			}
			if err := db.Create(&persisted).Error; err != nil {
				t.Fatal(err)
			}
			stale := persisted
			stale.Status = model.TaskStatusInProgress
			stale.Progress = "50%"
			upstreamID := stale.GetUpstreamTaskID()

			_ = tt.run(context.Background(), 999999, []string{upstreamID}, map[string]*model.Task{upstreamID: &stale})

			var got model.Task
			if err := db.First(&got, persisted.ID).Error; err != nil {
				t.Fatal(err)
			}
			if got.Status != model.TaskStatusSuccess {
				t.Fatalf("channel lookup failure overwrote terminal status with %s", got.Status)
			}
		})
	}
}

func TestGroupSunoTaskIDsBySelectedKey(t *testing.T) {
	channel := &model.Channel{Key: "key-a\nkey-b"}
	tasks := map[string]*model.Task{
		"upstream-a": {PrivateData: model.TaskPrivateData{Key: "key-a"}},
		"upstream-b": {PrivateData: model.TaskPrivateData{Key: "key-b"}},
	}

	groups, err := groupSunoTaskIDsByKey(channel, []string{"upstream-a", "upstream-b"}, tasks)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || len(groups["key-a"]) != 1 || len(groups["key-b"]) != 1 {
		t.Fatalf("expected tasks grouped by selected key, got %#v", groups)
	}
}

func TestGroupSunoTasksUsesSavedKeyWhenCurrentPoolIsDisabled(t *testing.T) {
	channel := &model.Channel{
		Key: "current-a\ncurrent-b",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusAutoDisabled},
		},
	}
	tasks := map[string]*model.Task{
		"saved-task": {PrivateData: model.TaskPrivateData{Key: "saved-key"}},
	}
	groups, err := groupSunoTaskIDsByKey(channel, []string{"saved-task"}, tasks)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups["saved-key"]) != 1 {
		t.Fatalf("saved task key was not used: %#v", groups)
	}
}

func TestPollingTaskLookupSeparatesSameUpstreamIDAcrossChannels(t *testing.T) {
	first := &model.Task{ChannelId: 11, TaskID: "public-a"}
	second := &model.Task{ChannelId: 22, TaskID: "public-b"}
	tasks := map[string]*model.Task{
		pollingTaskMapKey(first.ChannelId, "same-upstream-id"):  first,
		pollingTaskMapKey(second.ChannelId, "same-upstream-id"): second,
	}

	if got := findPollingTask(tasks, first.ChannelId, "same-upstream-id"); got != first {
		t.Fatalf("first channel resolved wrong task: %#v", got)
	}
	if got := findPollingTask(tasks, second.ChannelId, "same-upstream-id"); got != second {
		t.Fatalf("second channel resolved wrong task: %#v", got)
	}
}

func TestPollingTaskRefsSeparateSameUpstreamIDAcrossKeys(t *testing.T) {
	first := &model.Task{ID: 101, ChannelId: 11, TaskID: "public-a", PrivateData: model.TaskPrivateData{Key: "key-a", UpstreamTaskID: "same-upstream-id"}}
	second := &model.Task{ID: 102, ChannelId: 11, TaskID: "public-b", PrivateData: model.TaskPrivateData{Key: "key-b", UpstreamTaskID: "same-upstream-id"}}
	firstRef := pollingTaskRef(first)
	secondRef := pollingTaskRef(second)
	tasks := map[string]*model.Task{firstRef: first, secondRef: second}

	groups, err := groupSunoTaskIDsByKey(&model.Channel{Id: 11}, []string{firstRef, secondRef}, tasks)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 || len(groups["key-a"]) != 1 || len(groups["key-b"]) != 1 {
		t.Fatalf("same upstream ID tasks collided across keys: tasks=%d groups=%#v", len(tasks), groups)
	}
	if resolvePollingTask(tasks, 11, firstRef) != first || resolvePollingTask(tasks, 11, secondRef) != second {
		t.Fatal("polling references did not resolve the exact persisted task")
	}
}
