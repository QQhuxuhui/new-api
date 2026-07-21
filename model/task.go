package model

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TaskStatus string

func (t TaskStatus) ToVideoStatus() string {
	var status string
	switch t {
	case TaskStatusQueued, TaskStatusSubmitted:
		status = dto.VideoStatusQueued
	case TaskStatusInProgress:
		status = dto.VideoStatusInProgress
	case TaskStatusSuccess:
		status = dto.VideoStatusCompleted
	case TaskStatusFailure:
		status = dto.VideoStatusFailed
	default:
		status = dto.VideoStatusUnknown // Default fallback
	}
	return status
}

const (
	TaskStatusNotStart   TaskStatus = "NOT_START"
	TaskStatusSubmitted             = "SUBMITTED"
	TaskStatusQueued                = "QUEUED"
	TaskStatusInProgress            = "IN_PROGRESS"
	TaskStatusFailure               = "FAILURE"
	TaskStatusSuccess               = "SUCCESS"
	TaskStatusUnknown               = "UNKNOWN"
)

const (
	TaskBillingWaiting     = "waiting"
	TaskBillingCalculating = "calculating"
	TaskBillingPending     = "pending"
	TaskBillingAccounting  = "accounting"
	TaskBillingSettled     = "settled"
)

// TaskRefundLegacyCutoff separates legacy timeout tasks that intentionally
// do not receive automatic refunds from tasks covered by reconciliation.
const TaskRefundLegacyCutoff int64 = 1740182400 // 2025-02-22 00:00:00 UTC

type Task struct {
	ID        int64                 `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	CreatedAt int64                 `json:"created_at" gorm:"index"`
	UpdatedAt int64                 `json:"updated_at"`
	TaskID    string                `json:"task_id" gorm:"type:varchar(191);index"` // 第三方id，不一定有/ song id\ Task id
	Platform  constant.TaskPlatform `json:"platform" gorm:"type:varchar(30);index"` // 平台
	UserId    int                   `json:"user_id" gorm:"index"`
	Group     string                `json:"group" gorm:"type:varchar(50)"` // 修正计费用
	ChannelId int                   `json:"channel_id" gorm:"index"`
	Quota     int                   `json:"quota"`
	// BillingStatus/PendingQuota form a durable outbox for success settlement.
	// Pending stores the target quota before funding; accounting stores the old
	// quota until audit/stat side effects have completed.
	BillingStatus       string     `json:"-" gorm:"type:varchar(20);index"`
	PendingQuota        int        `json:"-"`
	AccountingPlanDelta int64      `json:"-"`
	Action              string     `json:"action" gorm:"type:varchar(40);index"` // 任务类型, song, lyrics, description-mode
	Status              TaskStatus `json:"status" gorm:"type:varchar(20);index"` // 任务状态
	FailReason          string     `json:"fail_reason"`
	SubmitTime          int64      `json:"submit_time" gorm:"index"`
	StartTime           int64      `json:"start_time" gorm:"index"`
	FinishTime          int64      `json:"finish_time" gorm:"index"`
	Progress            string     `json:"progress" gorm:"type:varchar(20);index"`
	Properties          Properties `json:"properties" gorm:"type:json"`
	Username            string     `json:"username,omitempty" gorm:"-"`
	// 禁止返回给用户，内部可能包含key等隐私信息
	PrivateData TaskPrivateData `json:"-" gorm:"column:private_data;type:json"`
	Data        json.RawMessage `json:"data" gorm:"type:json"`
}

const (
	TaskBillingCompensationPending    = "pending"
	TaskBillingCompensationAccounting = "accounting"
	TaskBillingCompensationSettled    = "settled"
)

type TaskBillingCompensation struct {
	ID          int64  `gorm:"primaryKey;autoIncrement"`
	CreatedAt   int64  `gorm:"autoCreateTime:milli"`
	UpdatedAt   int64  `gorm:"autoUpdateTime:milli"`
	TaskID      string `gorm:"type:varchar(191);uniqueIndex"`
	UserId      int    `gorm:"index"`
	ChannelId   int    `gorm:"index"`
	Group       string `gorm:"type:varchar(50)"`
	Quota       int
	Reason      string
	Status      string `gorm:"type:varchar(20);index"`
	Attempts    int
	RetryAt     int64 `gorm:"index"`
	LastError   string
	Properties  Properties      `gorm:"type:json"`
	PrivateData TaskPrivateData `gorm:"type:json"`
}

func (c *TaskBillingCompensation) TaskSnapshot() *Task {
	if c == nil {
		return nil
	}
	return &Task{
		TaskID:      c.TaskID,
		UserId:      c.UserId,
		ChannelId:   c.ChannelId,
		Group:       c.Group,
		Quota:       c.Quota,
		Properties:  c.Properties,
		PrivateData: c.PrivateData,
	}
}

func (t *Task) SetData(data any) {
	b, _ := json.Marshal(data)
	t.Data = json.RawMessage(b)
}

func (t *Task) GetData(v any) error {
	err := json.Unmarshal(t.Data, &v)
	return err
}

type Properties struct {
	Input             string `json:"input"`
	UpstreamModelName string `json:"upstream_model_name,omitempty"`
	OriginModelName   string `json:"origin_model_name,omitempty"`
}

// TaskInitContext 用于解耦 model 与 relay/common（dev 的 relay/common 反向依赖 model，
// InitTask 不能直接接收 *relaycommon.RelayInfo）。
type TaskInitContext struct {
	UserId            int
	UsingGroup        string
	ChannelId         int
	ChannelType       int
	ChannelApiKey     string
	UpstreamModelName string
	OriginModelName   string
	// PublicTaskID 是提交时预生成的 task_xxxx 公开 ID；为空时 InitTask 会新生成。
	PublicTaskID string
	// 计费上下文（异步退款/差额结算依赖）
	UserPlanId     int
	BillingSource  string
	DailyPoolDate  string
	TokenId        int
	NodeName       string
	BillingContext *TaskBillingContext
}

func (m *Properties) Scan(val interface{}) error {
	bytesValue, _ := val.([]byte)
	if len(bytesValue) == 0 {
		*m = Properties{}
		return nil
	}
	return json.Unmarshal(bytesValue, m)
}

func (m Properties) Value() (driver.Value, error) {
	if m == (Properties{}) {
		return nil, nil
	}
	return json.Marshal(m)
}

type TaskPrivateData struct {
	Key            string `json:"key,omitempty"`
	UpstreamTaskID string `json:"upstream_task_id,omitempty"` // 上游真实 task ID
	ResultURL      string `json:"result_url,omitempty"`       // 任务成功后的结果 URL（视频地址等）
	// 计费上下文：用于异步退款/差额结算（轮询阶段读取）
	BillingSource     string              `json:"billing_source,omitempty"`      // 计费来源（daily_pool/plan/plan_and_user_balance/user_balance）
	DailyPoolDate     string              `json:"daily_pool_date,omitempty"`     // 日卡实际扣款日期，跨日退款/补扣必须使用同一天
	UserPlanId        int                 `json:"user_plan_id,omitempty"`        // 用户套餐 ID，用于套餐额度退款/补扣
	TokenId           int                 `json:"token_id,omitempty"`            // 令牌 ID，用于令牌额度退款
	TokenChargedQuota *int                `json:"token_charged_quota,omitempty"` // nil 表示旧任务，按 task quota 兼容
	TokenQuotaEnabled bool                `json:"token_quota_enabled,omitempty"` // 新任务是否启用令牌额度跟踪
	NodeName          string              `json:"node_name,omitempty"`           // 发起任务的节点名，轮询结算阶段据此归属日志而非最后查询节点
	BillingContext    *TaskBillingContext `json:"billing_context,omitempty"`     // 计费参数快照（用于轮询阶段重新计算）
	// 混合计费分账：结算时实际从套餐/钱包扣除的额度，异步退款按此逆向
	PlanChargedQuota   int64 `json:"plan_charged_quota,omitempty"`
	WalletChargedQuota int64 `json:"wallet_charged_quota,omitempty"`
}

// TaskBillingContext 记录任务提交时的计费参数，以便轮询阶段可以重新计算额度。
type TaskBillingContext struct {
	ModelPrice        float64            `json:"model_price,omitempty"`         // 模型单价
	GroupRatio        float64            `json:"group_ratio,omitempty"`         // 分组倍率
	ModelRatio        float64            `json:"model_ratio,omitempty"`         // 模型倍率
	ChannelRatio      float64            `json:"channel_ratio,omitempty"`       // 提交时渠道倍率
	ChannelModelRatio float64            `json:"channel_model_ratio,omitempty"` // 提交时渠道模型倍率
	OtherRatios       map[string]float64 `json:"other_ratios,omitempty"`        // 附加倍率（时长、分辨率等）
	OriginModelName   string             `json:"origin_model_name,omitempty"`   // 模型名称，必须为OriginModelName
	PerCallBilling    bool               `json:"per_call_billing,omitempty"`    // 按次计费：跳过轮询阶段的差额结算
}

// GetUpstreamTaskID 获取上游真实 task ID（用于与 provider 通信）
// 旧数据没有 UpstreamTaskID 时，TaskID 本身就是上游 ID
func (t *Task) GetUpstreamTaskID() string {
	if t.PrivateData.UpstreamTaskID != "" {
		return t.PrivateData.UpstreamTaskID
	}
	return t.TaskID
}

// GetResultURL 获取任务结果 URL（视频地址等）
// 新数据存在 PrivateData.ResultURL 中；旧数据回退到 FailReason（历史兼容）
func (t *Task) GetResultURL() string {
	if t.PrivateData.ResultURL != "" {
		return t.PrivateData.ResultURL
	}
	return t.FailReason
}

// GenerateTaskID 生成对外暴露的 task_xxxx 格式 ID
func GenerateTaskID() string {
	key, _ := common.GenerateRandomCharsKey(32)
	return "task_" + key
}

func (p *TaskPrivateData) Scan(val interface{}) error {
	bytesValue, _ := val.([]byte)
	if len(bytesValue) == 0 {
		return nil
	}
	return json.Unmarshal(bytesValue, p)
}

func (p TaskPrivateData) Value() (driver.Value, error) {
	if (p == TaskPrivateData{}) {
		return nil, nil
	}
	return json.Marshal(p)
}

// SyncTaskQueryParams 用于包含所有搜索条件的结构体，可以根据需求添加更多字段
type SyncTaskQueryParams struct {
	Platform       constant.TaskPlatform
	ChannelID      string
	TaskID         string
	UserID         string
	Action         string
	Status         string
	StartTimestamp int64
	EndTimestamp   int64
	UserIDs        []int
}

func InitTask(platform constant.TaskPlatform, ctx *TaskInitContext) *Task {
	properties := Properties{}
	privateData := TaskPrivateData{}
	if ctx == nil {
		ctx = &TaskInitContext{}
	}
	{
		// The selected key is a per-task credential snapshot. Multi-key channels
		// must reuse the same key during polling instead of the aggregate channel key.
		if ctx.ChannelApiKey != "" {
			privateData.Key = ctx.ChannelApiKey
		}
		if ctx.UpstreamModelName != "" {
			properties.UpstreamModelName = ctx.UpstreamModelName
		}
		if ctx.OriginModelName != "" {
			properties.OriginModelName = ctx.OriginModelName
		}
		privateData.UserPlanId = ctx.UserPlanId
		privateData.BillingSource = ctx.BillingSource
		privateData.DailyPoolDate = ctx.DailyPoolDate
		privateData.TokenId = ctx.TokenId
		privateData.NodeName = ctx.NodeName
		privateData.BillingContext = ctx.BillingContext
	}

	// 使用预生成的公开 ID（如果有），否则新生成
	taskID := ctx.PublicTaskID
	if taskID == "" {
		taskID = GenerateTaskID()
	}

	t := &Task{
		TaskID:      taskID,
		UserId:      ctx.UserId,
		Group:       ctx.UsingGroup,
		SubmitTime:  time.Now().Unix(),
		Status:      TaskStatusNotStart,
		Progress:    "0%",
		ChannelId:   ctx.ChannelId,
		Platform:    platform,
		Properties:  properties,
		PrivateData: privateData,
	}
	return t
}

func TaskGetAllUserTask(userId int, startIdx int, num int, queryParams SyncTaskQueryParams) []*Task {
	var tasks []*Task
	var err error

	// 初始化查询构建器
	query := DB.Where("user_id = ?", userId)

	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.StartTimestamp != 0 {
		// 假设您已将前端传来的时间戳转换为数据库所需的时间格式，并处理了时间戳的验证和解析
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Omit("channel_id").Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func TaskGetAllTasks(startIdx int, num int, queryParams SyncTaskQueryParams) []*Task {
	var tasks []*Task
	var err error

	// 初始化查询构建器
	query := DB

	// 添加过滤条件
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.UserID != "" {
		query = query.Where("user_id = ?", queryParams.UserID)
	}
	if len(queryParams.UserIDs) != 0 {
		query = query.Where("user_id in (?)", queryParams.UserIDs)
	}
	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func GetTimedOutUnfinishedTasks(cutoffUnix int64, limit int) []*Task {
	var tasks []*Task
	err := DB.Where("progress != ?", "100%").
		Where("status NOT IN ?", []string{TaskStatusFailure, TaskStatusSuccess}).
		Where("submit_time < ?", cutoffUnix).
		Order("submit_time").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

// GetUnrefundedFailedTasks returns failed tasks whose non-zero quota marks a
// pending refund. Legacy timeout tasks are excluded before LIMIT is applied so
// they cannot starve refundable tasks from the reconciliation sweep.
func GetUnrefundedFailedTasks(updatedBefore int64, limit int) []*Task {
	if limit <= 0 {
		return nil
	}

	var tasks []*Task
	err := DB.Where("status = ?", TaskStatusFailure).
		Where("quota != ?", 0).
		Where("updated_at <= ?", updatedBefore).
		Where("(submit_time <= ? OR submit_time >= ?)", 0, TaskRefundLegacyCutoff).
		Order("id").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

func GetRecoverableSuccessfulTaskSettlements(limit int) []*Task {
	if limit <= 0 {
		return nil
	}
	var tasks []*Task
	if err := DB.Where("status = ? AND billing_status IN ?", TaskStatusSuccess, []string{TaskBillingCalculating, TaskBillingPending, TaskBillingAccounting}).
		Order("id").Limit(limit).Find(&tasks).Error; err != nil {
		return nil
	}
	return tasks
}

func GetAllUnFinishSyncTasks(limit int) []*Task {
	var tasks []*Task
	var err error
	// get all tasks progress is not 100%
	err = DB.Where("progress != ?", "100%").Where("status != ?", TaskStatusFailure).Where("status != ?", TaskStatusSuccess).Limit(limit).Order("id").Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

// HasUnfinishedSyncTasks reports whether at least one async (Suno/video) task is
// still in progress. It is a cheap existence check (LIMIT 1) used to decide
// whether the polling loop has pending work at all.
func HasUnfinishedSyncTasks() bool {
	var id int64
	err := DB.Model(&Task{}).
		Where("progress != ?", "100%").
		Where("status != ?", TaskStatusFailure).
		Where("status != ?", TaskStatusSuccess).
		Limit(1).
		Pluck("id", &id).Error
	return err == nil && id != 0
}

// HasTaskPollingWork reports whether polling has either an unfinished task or
// a failed task with a pending, non-legacy refund. The latter keeps polling
// active when reconciliation is the only work left.
func HasTaskPollingWork() bool {
	if HasUnfinishedSyncTasks() {
		return true
	}
	if HasPendingTaskBillingCompensation() {
		return true
	}
	var pendingSettlementID int64
	if err := DB.Model(&Task{}).
		Where("status = ? AND billing_status IN ?", TaskStatusSuccess, []string{TaskBillingCalculating, TaskBillingPending, TaskBillingAccounting}).
		Limit(1).Pluck("id", &pendingSettlementID).Error; err == nil && pendingSettlementID != 0 {
		return true
	}

	var id int64
	err := DB.Model(&Task{}).
		Where("status = ?", TaskStatusFailure).
		Where("quota != ?", 0).
		Where("(submit_time <= ? OR submit_time >= ?)", 0, TaskRefundLegacyCutoff).
		Limit(1).
		Pluck("id", &id).Error
	return err == nil && id != 0
}

func GetByOnlyTaskId(taskId string) (*Task, bool, error) {
	if taskId == "" {
		return nil, false, nil
	}
	var task *Task
	var err error
	err = DB.Where("task_id = ?", taskId).First(&task).Error
	exist, err := RecordExist(err)
	if err != nil {
		return nil, false, err
	}
	return task, exist, err
}

func GetByTaskId(userId int, taskId string) (*Task, bool, error) {
	if taskId == "" {
		return nil, false, nil
	}
	var task *Task
	var err error
	err = DB.Where("user_id = ? and task_id = ?", userId, taskId).
		First(&task).Error
	exist, err := RecordExist(err)
	if err != nil {
		return nil, false, err
	}
	return task, exist, err
}

func GetByTaskIds(userId int, taskIds []any) ([]*Task, error) {
	if len(taskIds) == 0 {
		return nil, nil
	}
	var task []*Task
	var err error
	err = DB.Where("user_id = ? and task_id in (?)", userId, taskIds).
		Find(&task).Error
	if err != nil {
		return nil, err
	}
	return task, nil
}

func TaskUpdateProgress(id int64, progress string) error {
	return DB.Model(&Task{}).Where("id = ?", id).Update("progress", progress).Error
}

func (Task *Task) Insert() error {
	var err error
	err = DB.Create(Task).Error
	return err
}

type taskSnapshot struct {
	Status     TaskStatus
	Progress   string
	StartTime  int64
	FinishTime int64
	FailReason string
	ResultURL  string
	Data       json.RawMessage
}

func (s taskSnapshot) Equal(other taskSnapshot) bool {
	return s.Status == other.Status &&
		s.Progress == other.Progress &&
		s.StartTime == other.StartTime &&
		s.FinishTime == other.FinishTime &&
		s.FailReason == other.FailReason &&
		s.ResultURL == other.ResultURL &&
		bytes.Equal(s.Data, other.Data)
}

func (t *Task) Snapshot() taskSnapshot {
	return taskSnapshot{
		Status:     t.Status,
		Progress:   t.Progress,
		StartTime:  t.StartTime,
		FinishTime: t.FinishTime,
		FailReason: t.FailReason,
		ResultURL:  t.PrivateData.ResultURL,
		Data:       t.Data,
	}
}

func (Task *Task) Update() error {
	var err error
	err = DB.Save(Task).Error
	return err
}

func (t *Task) UpdateQuota() error {
	return DB.Model(t).Update("quota", t.Quota).Error
}

// UpdatePrivateData 仅回写 private_data 列。
// 异步结算阶段计费来源/分账字段可能变化（如套餐补扣回退钱包），需要单独持久化。
func (t *Task) UpdatePrivateData() error {
	return DB.Model(t).Update("private_data", t.PrivateData).Error
}

// RefundUserQuotaAndUpdateTaskPrivateData commits a wallet refund together
// with the billing split snapshot. A failed snapshot write must not leave the
// wallet refunded while the task still claims that amount is outstanding.
func RefundUserQuotaAndUpdateTaskPrivateData(taskID int64, userID, amount int, privateData TaskPrivateData) error {
	if taskID <= 0 {
		return errors.New("task id is required for atomic refund")
	}
	if amount <= 0 {
		return errors.New("refund amount must be positive")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", userID).Update("quota", gorm.Expr("quota + ?", amount)).Error; err != nil {
			return err
		}
		return tx.Model(&Task{}).Where("id = ?", taskID).Update("private_data", privateData).Error
	})
	if err == nil && common.RedisEnabled {
		gopool.Go(func() {
			if cacheErr := cacheIncrUserQuota(userID, int64(amount)); cacheErr != nil {
				common.SysLog("failed to update refunded user quota cache: " + cacheErr.Error())
			}
		})
	}
	return err
}

// RefundTaskQuotaAtomic locks one failed task and commits all database-backed
// refund effects together with clearing its quota marker. This makes a crash
// or SQL error either leave the entire refund pending or commit it exactly once.
func RefundTaskQuotaAtomic(id int64, expectedQuota int) (bool, error) {
	if id <= 0 || expectedQuota <= 0 {
		return false, nil
	}

	var refunded bool
	var walletRefund int64
	var refundedUserID int
	var refundedPlanUserID int
	var tokenKey string
	var tokenRefundQuota int64
	var legacyTokenAccountingSkipped bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		var task Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, id).Error; err != nil {
			return err
		}
		if task.Status != TaskStatusFailure || task.Quota != expectedQuota {
			return nil
		}
		refundedUserID = task.UserId

		privateData := task.PrivateData
		refund := int64(expectedQuota)
		addWallet := func(amount int64) error {
			if amount <= 0 {
				return nil
			}
			result := tx.Model(&User{}).Where("id = ?", task.UserId).
				Update("quota", gorm.Expr("quota + ?", amount))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return fmt.Errorf("refund user %d not found", task.UserId)
			}
			walletRefund += amount
			return nil
		}
		addPlan := func(amount int64) error {
			if amount <= 0 {
				return nil
			}
			var plan UserPlan
			if err := tx.Select("id", "user_id").First(&plan, privateData.UserPlanId).Error; err != nil {
				return err
			}
			result := tx.Model(&UserPlan{}).Where("id = ?", privateData.UserPlanId).
				Update("quota", gorm.Expr("quota + ?", amount))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return fmt.Errorf("refund user plan %d not found", privateData.UserPlanId)
			}
			refundedPlanUserID = plan.UserId
			return nil
		}

		switch privateData.BillingSource {
		case "plan", "plan_and_user_balance":
			if privateData.UserPlanId <= 0 {
				if err := addWallet(refund); err != nil {
					return err
				}
				break
			}
			walletPart := privateData.WalletChargedQuota
			if walletPart > refund {
				walletPart = refund
			}
			if err := addWallet(walletPart); err != nil {
				return err
			}
			if err := addPlan(refund - walletPart); err != nil {
				return err
			}
			privateData.WalletChargedQuota -= walletPart
			if privateData.PlanChargedQuota > 0 {
				privateData.PlanChargedQuota -= refund - walletPart
				if privateData.PlanChargedQuota < 0 {
					privateData.PlanChargedQuota = 0
				}
			}
		case "daily_pool":
			walletPart := privateData.WalletChargedQuota
			if walletPart > refund {
				walletPart = refund
			}
			if err := addWallet(walletPart); err != nil {
				return err
			}
			privateData.WalletChargedQuota -= walletPart
			poolPart := refund - walletPart
			if poolPart > 0 {
				billingDate := privateData.DailyPoolDate
				if billingDate == "" {
					billingDate = GetTodayDate()
				}
				if err := increaseDailyPoolQuotaWithDB(tx, task.UserId, billingDate, poolPart); err != nil {
					return err
				}
			}
		default:
			if err := addWallet(refund); err != nil {
				return err
			}
		}

		tokenQuotaEnabled, trackedTokenQuota, legacySkipped := taskTokenBillingState(privateData, int64(expectedQuota))
		tokenRefundQuota = trackedTokenQuota
		legacyTokenAccountingSkipped = legacySkipped
		if tokenQuotaEnabled && tokenRefundQuota > 0 {
			var token Token
			if err := tx.Select("id", "key").First(&token, privateData.TokenId).Error; err == nil {
				tokenKey = token.Key
				if err := tx.Model(&Token{}).Where("id = ?", privateData.TokenId).Updates(map[string]interface{}{
					"remain_quota":  gorm.Expr("remain_quota + ?", tokenRefundQuota),
					"used_quota":    gorm.Expr("used_quota - ?", tokenRefundQuota),
					"accessed_time": common.GetTimestamp(),
				}).Error; err != nil {
					return err
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if privateData.TokenChargedQuota != nil {
			zero := 0
			privateData.TokenChargedQuota = &zero
		}

		result := tx.Model(&Task{}).
			Where("id = ? AND quota = ?", id, expectedQuota).
			Updates(map[string]interface{}{
				"quota":          0,
				"private_data":   privateData,
				"billing_status": TaskBillingSettled,
				"pending_quota":  0,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("task refund marker changed during transaction")
		}
		refunded = true
		return nil
	})
	if err != nil || !refunded {
		return refunded, err
	}
	if walletRefund > 0 && common.RedisEnabled {
		gopool.Go(func() {
			if cacheErr := cacheIncrUserQuota(refundedUserID, walletRefund); cacheErr != nil {
				common.SysLog("failed to update task refund user quota cache: " + cacheErr.Error())
			}
		})
	}
	if refundedPlanUserID > 0 {
		if cacheErr := InvalidateUserPlanCache(refundedPlanUserID); cacheErr != nil {
			common.SysLog("failed to invalidate refunded user plan cache: " + cacheErr.Error())
		}
	}
	if tokenKey != "" && common.RedisEnabled {
		gopool.Go(func() {
			if cacheErr := cacheIncrTokenQuota(tokenKey, tokenRefundQuota); cacheErr != nil {
				common.SysLog("failed to update task refund token quota cache: " + cacheErr.Error())
			}
		})
	}
	if legacyTokenAccountingSkipped {
		common.SysLog(fmt.Sprintf("legacy task %d token refund skipped because exact token charge is unavailable", id))
	}
	return true, nil
}

func taskTokenBillingState(privateData TaskPrivateData, legacyQuota int64) (enabled bool, quota int64, legacySkipped bool) {
	if privateData.TokenId <= 0 {
		return false, 0, false
	}
	if privateData.TokenChargedQuota != nil {
		return privateData.TokenQuotaEnabled, int64(*privateData.TokenChargedQuota), false
	}
	switch privateData.BillingSource {
	case "plan", "plan_and_user_balance", "daily_pool":
		// Old task rows cannot distinguish a finite token debit from an
		// unlimited/playground token that was intentionally not debited. Avoid
		// creating quota; operators can reconcile the conservative under-refund.
		return false, 0, true
	default:
		return true, legacyQuota, false
	}
}

// RefundUnpersistedTaskQuotaAtomic compensates a submitted task whose row could
// not be created. Funding and token ledgers move in one transaction so callers
// never observe a partially compensated request.
func RefundUnpersistedTaskQuotaAtomic(task *Task, expectedQuota int) error {
	if task == nil || expectedQuota <= 0 {
		return nil
	}
	effects := taskRefundEffects{userID: task.UserId}
	err := DB.Transaction(func(tx *gorm.DB) error {
		return refundTaskSnapshotTx(tx, task, expectedQuota, &effects)
	})
	if err != nil {
		return err
	}
	applyTaskRefundCacheEffects(effects)
	return nil
}

type taskRefundEffects struct {
	userID           int
	walletRefund     int64
	planUserID       int
	tokenKey         string
	tokenRefundQuota int64
}

func refundTaskSnapshotTx(tx *gorm.DB, task *Task, expectedQuota int, effects *taskRefundEffects) error {
	privateData := task.PrivateData
	refund := int64(expectedQuota)
	addWallet := func(amount int64) error {
		if amount <= 0 {
			return nil
		}
		result := tx.Model(&User{}).Where("id = ?", task.UserId).
			Update("quota", gorm.Expr("quota + ?", amount))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("refund user %d not found", task.UserId)
		}
		effects.walletRefund += amount
		return nil
	}
	addPlan := func(amount int64) error {
		if amount <= 0 {
			return nil
		}
		var plan UserPlan
		if err := tx.Select("id", "user_id").First(&plan, privateData.UserPlanId).Error; err != nil {
			return err
		}
		result := tx.Model(&UserPlan{}).Where("id = ?", privateData.UserPlanId).
			Update("quota", gorm.Expr("quota + ?", amount))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("refund user plan %d not found", privateData.UserPlanId)
		}
		effects.planUserID = plan.UserId
		return nil
	}

	switch privateData.BillingSource {
	case "plan", "plan_and_user_balance":
		if privateData.UserPlanId <= 0 {
			if err := addWallet(refund); err != nil {
				return err
			}
			break
		}
		walletPart := privateData.WalletChargedQuota
		if walletPart > refund {
			walletPart = refund
		}
		if err := addWallet(walletPart); err != nil {
			return err
		}
		if err := addPlan(refund - walletPart); err != nil {
			return err
		}
	case "daily_pool":
		walletPart := privateData.WalletChargedQuota
		if walletPart > refund {
			walletPart = refund
		}
		if err := addWallet(walletPart); err != nil {
			return err
		}
		poolPart := refund - walletPart
		if poolPart > 0 {
			billingDate := privateData.DailyPoolDate
			if billingDate == "" {
				billingDate = GetTodayDate()
			}
			if err := increaseDailyPoolQuotaWithDB(tx, task.UserId, billingDate, poolPart); err != nil {
				return err
			}
		}
	default:
		if err := addWallet(refund); err != nil {
			return err
		}
	}

	tokenEnabled, trackedTokenQuota, _ := taskTokenBillingState(privateData, refund)
	effects.tokenRefundQuota = trackedTokenQuota
	if tokenEnabled && effects.tokenRefundQuota > 0 {
		var token Token
		if err := tx.Select("id", "key").First(&token, privateData.TokenId).Error; err == nil {
			effects.tokenKey = token.Key
			if err := tx.Model(&Token{}).Where("id = ?", privateData.TokenId).Updates(map[string]interface{}{
				"remain_quota":  gorm.Expr("remain_quota + ?", effects.tokenRefundQuota),
				"used_quota":    gorm.Expr("used_quota - ?", effects.tokenRefundQuota),
				"accessed_time": common.GetTimestamp(),
			}).Error; err != nil {
				return err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	return nil
}

func applyTaskRefundCacheEffects(effects taskRefundEffects) {
	if effects.walletRefund > 0 && common.RedisEnabled {
		gopool.Go(func() {
			if cacheErr := cacheIncrUserQuota(effects.userID, effects.walletRefund); cacheErr != nil {
				common.SysLog("failed to update unpersisted task refund user quota cache: " + cacheErr.Error())
			}
		})
	}
	if effects.planUserID > 0 {
		if cacheErr := InvalidateUserPlanCache(effects.planUserID); cacheErr != nil {
			common.SysLog("failed to invalidate unpersisted task refund plan cache: " + cacheErr.Error())
		}
	}
	if effects.tokenKey != "" && common.RedisEnabled {
		gopool.Go(func() {
			if cacheErr := cacheIncrTokenQuota(effects.tokenKey, effects.tokenRefundQuota); cacheErr != nil {
				common.SysLog("failed to update unpersisted task refund token quota cache: " + cacheErr.Error())
			}
		})
	}
}

func CreateTaskBillingCompensation(task *Task, reason string) (*TaskBillingCompensation, error) {
	if task == nil || task.TaskID == "" || task.Quota <= 0 {
		return nil, errors.New("task billing compensation requires task id and quota")
	}
	compensation := &TaskBillingCompensation{
		TaskID:      task.TaskID,
		UserId:      task.UserId,
		ChannelId:   task.ChannelId,
		Group:       task.Group,
		Quota:       task.Quota,
		Reason:      reason,
		Status:      TaskBillingCompensationPending,
		Properties:  task.Properties,
		PrivateData: task.PrivateData,
	}
	result := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "task_id"}},
		DoNothing: true,
	}).Create(compensation)
	if result.Error != nil {
		// The insert may have committed even if the acknowledgement was lost.
		// Read by the deterministic task ID before reporting an ambiguous error;
		// callers must never issue an unmarked direct refund in that case.
		if lookupErr := DB.Where("task_id = ?", task.TaskID).First(compensation).Error; lookupErr == nil {
			return compensation, nil
		} else {
			return nil, fmt.Errorf("create task billing compensation: %w; verify insert: %v", result.Error, lookupErr)
		}
	}
	if result.RowsAffected == 0 {
		if err := DB.Where("task_id = ?", task.TaskID).First(compensation).Error; err != nil {
			return nil, err
		}
	}
	return compensation, nil
}

func GetPendingTaskBillingCompensations(limit int) []*TaskBillingCompensation {
	if limit <= 0 {
		return nil
	}
	var compensations []*TaskBillingCompensation
	if err := DB.Where("status IN ? AND retry_at <= ?", []string{TaskBillingCompensationPending, TaskBillingCompensationAccounting}, time.Now().Unix()).
		Order("retry_at, id").Limit(limit).Find(&compensations).Error; err != nil {
		return nil
	}
	return compensations
}

func HasPendingTaskBillingCompensation() bool {
	var id int64
	err := DB.Model(&TaskBillingCompensation{}).
		Where("status IN ?", []string{TaskBillingCompensationPending, TaskBillingCompensationAccounting}).
		Limit(1).Pluck("id", &id).Error
	return err == nil && id != 0
}

func PrepareTaskBillingCompensationAccountingAtomic(id int64) (*Task, bool, error) {
	if id <= 0 {
		return nil, false, nil
	}
	var accountingTask *Task
	var ready bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		var compensation TaskBillingCompensation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&compensation, id).Error; err != nil {
			return err
		}
		switch compensation.Status {
		case TaskBillingCompensationSettled:
			return nil
		case TaskBillingCompensationAccounting:
			accountingTask = compensation.TaskSnapshot()
			ready = true
			return nil
		case TaskBillingCompensationPending:
		default:
			return fmt.Errorf("unknown task billing compensation status %q", compensation.Status)
		}
		task := compensation.TaskSnapshot()
		effects := taskRefundEffects{userID: task.UserId}
		if err := refundTaskSnapshotTx(tx, task, compensation.Quota, &effects); err != nil {
			return err
		}
		result := tx.Model(&TaskBillingCompensation{}).
			Where("id = ? AND status = ?", compensation.ID, TaskBillingCompensationPending).
			Updates(map[string]interface{}{
				"status":     TaskBillingCompensationAccounting,
				"retry_at":   0,
				"last_error": "",
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("task billing compensation marker changed during transaction")
		}
		accountingTask = task
		ready = true
		return nil
	})
	return accountingTask, ready, err
}

func CompleteTaskBillingCompensationAccounting(id int64) (bool, error) {
	if id <= 0 {
		return false, nil
	}
	result := DB.Model(&TaskBillingCompensation{}).
		Where("id = ? AND status = ?", id, TaskBillingCompensationAccounting).
		Updates(map[string]interface{}{
			"status":     TaskBillingCompensationSettled,
			"retry_at":   0,
			"last_error": "",
		})
	return result.RowsAffected > 0, result.Error
}

func DeferTaskBillingCompensation(id int64, cause error) error {
	if id <= 0 || cause == nil {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var compensation TaskBillingCompensation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&compensation, id).Error; err != nil {
			return err
		}
		if compensation.Status == TaskBillingCompensationSettled {
			return nil
		}
		attempts := compensation.Attempts + 1
		delaySeconds := int64(15)
		for i := 1; i < attempts && delaySeconds < 300; i++ {
			delaySeconds *= 2
		}
		if delaySeconds > 300 {
			delaySeconds = 300
		}
		lastError := cause.Error()
		if len(lastError) > 1024 {
			lastError = lastError[:1024]
		}
		return tx.Model(&TaskBillingCompensation{}).Where("id = ?", id).Updates(map[string]interface{}{
			"attempts":   attempts,
			"retry_at":   time.Now().Unix() + delaySeconds,
			"last_error": lastError,
		}).Error
	})
}

func taskPlanFundedQuotaSnapshot(privateData TaskPrivateData, quota int) int64 {
	if privateData.UserPlanId <= 0 || (privateData.BillingSource != "plan" && privateData.BillingSource != "plan_and_user_balance") {
		return 0
	}
	if privateData.PlanChargedQuota > 0 || privateData.WalletChargedQuota > 0 {
		return privateData.PlanChargedQuota
	}
	return int64(quota)
}

// SettlePendingTaskQuotaAtomic applies a successful task's pending quota and
// advances the outbox to accounting in the same transaction. It returns false
// when another worker has already moved the task out of pending.
func SettlePendingTaskQuotaAtomic(id int64, allowPlanCharge bool) (*Task, bool, error) {
	if id <= 0 {
		return nil, false, nil
	}

	var settledTask Task
	var settled bool
	var walletDelta int64
	var tokenSettlementDelta int64
	var planUserID int
	var tokenKey string
	var legacyTokenAccountingSkipped bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		var task Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, id).Error; err != nil {
			return err
		}
		if task.Status != TaskStatusSuccess || task.BillingStatus != TaskBillingPending {
			return nil
		}
		actualQuota := task.PendingQuota
		if actualQuota < 0 {
			return errors.New("pending task settlement quota cannot be negative")
		}
		preConsumedQuota := task.Quota
		delta := int64(actualQuota - preConsumedQuota)
		privateData := task.PrivateData
		prePlanFundedQuota := taskPlanFundedQuotaSnapshot(privateData, preConsumedQuota)

		adjustWallet := func(fundingDelta int64) error {
			if fundingDelta == 0 {
				return nil
			}
			query := tx.Model(&User{}).Where("id = ?", task.UserId)
			if fundingDelta > 0 {
				query = query.Where("quota >= ?", fundingDelta)
			}
			result := query.Update("quota", gorm.Expr("quota - ?", fundingDelta))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return fmt.Errorf("wallet cannot settle task %s delta %d", task.TaskID, fundingDelta)
			}
			walletDelta -= fundingDelta
			return nil
		}
		adjustPlan := func(fundingDelta int64) (bool, error) {
			if fundingDelta == 0 || privateData.UserPlanId <= 0 {
				return fundingDelta == 0, nil
			}
			if fundingDelta > 0 && !allowPlanCharge {
				return false, nil
			}
			var plan UserPlan
			if err := tx.First(&plan, privateData.UserPlanId).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) && fundingDelta > 0 {
					return false, nil
				}
				return false, err
			}
			if fundingDelta > 0 && (!plan.IsValid() || plan.Quota < fundingDelta) {
				return false, nil
			}
			query := tx.Model(&UserPlan{}).Where("id = ?", privateData.UserPlanId)
			if fundingDelta > 0 {
				query = query.Where("quota >= ?", fundingDelta)
			}
			updates := map[string]interface{}{
				"quota":      gorm.Expr("quota - ?", fundingDelta),
				"updated_at": time.Now().UnixMilli(),
			}
			if fundingDelta > 0 {
				updates["used_quota"] = gorm.Expr("used_quota + ?", fundingDelta)
			}
			result := query.Updates(updates)
			if result.Error != nil {
				return false, result.Error
			}
			if result.RowsAffected == 0 {
				return false, nil
			}
			planUserID = plan.UserId
			return true, nil
		}
		adjustDailyPool := func(fundingDelta int64) (bool, error) {
			if fundingDelta == 0 {
				return true, nil
			}
			billingDate := privateData.DailyPoolDate
			if billingDate == "" {
				billingDate = GetTodayDate()
			}
			if fundingDelta > 0 {
				result := tx.Model(&UserDailyPool{}).
					Where("user_id = ? AND date = ? AND (total_quota - used_quota) >= ?", task.UserId, billingDate, fundingDelta).
					Updates(map[string]interface{}{
						"used_quota": gorm.Expr("used_quota + ?", fundingDelta),
						"updated_at": time.Now().UnixMilli(),
					})
				if result.Error != nil {
					return false, result.Error
				}
				return result.RowsAffected > 0, nil
			}
			refund := -fundingDelta
			if err := increaseDailyPoolQuotaWithDB(tx, task.UserId, billingDate, refund); err != nil {
				return false, err
			}
			return true, nil
		}

		if delta > 0 {
			switch privateData.BillingSource {
			case "plan", "plan_and_user_balance":
				charged, err := adjustPlan(delta)
				if err != nil {
					return err
				}
				if charged {
					if privateData.PlanChargedQuota > 0 || privateData.WalletChargedQuota > 0 {
						privateData.PlanChargedQuota += delta
					}
				} else {
					if err := adjustWallet(delta); err != nil {
						return err
					}
					if privateData.BillingSource == "plan" {
						privateData.BillingSource = "plan_and_user_balance"
						if privateData.PlanChargedQuota == 0 {
							privateData.PlanChargedQuota = int64(task.Quota)
						}
					}
					privateData.WalletChargedQuota += delta
				}
			case "daily_pool":
				charged, err := adjustDailyPool(delta)
				if err != nil {
					return err
				}
				if !charged {
					if err := adjustWallet(delta); err != nil {
						return err
					}
					privateData.WalletChargedQuota += delta
				}
			default:
				if err := adjustWallet(delta); err != nil {
					return err
				}
			}
		} else if delta < 0 {
			refund := -delta
			switch privateData.BillingSource {
			case "plan", "plan_and_user_balance":
				walletPart := privateData.WalletChargedQuota
				if walletPart > refund {
					walletPart = refund
				}
				if err := adjustWallet(-walletPart); err != nil {
					return err
				}
				if _, err := adjustPlan(-(refund - walletPart)); err != nil {
					return err
				}
				privateData.WalletChargedQuota -= walletPart
				if privateData.PlanChargedQuota > 0 {
					privateData.PlanChargedQuota -= refund - walletPart
					if privateData.PlanChargedQuota < 0 {
						privateData.PlanChargedQuota = 0
					}
				}
			case "daily_pool":
				walletPart := privateData.WalletChargedQuota
				if walletPart > refund {
					walletPart = refund
				}
				if err := adjustWallet(-walletPart); err != nil {
					return err
				}
				if _, err := adjustDailyPool(-(refund - walletPart)); err != nil {
					return err
				}
				privateData.WalletChargedQuota -= walletPart
			default:
				if err := adjustWallet(delta); err != nil {
					return err
				}
			}
		}

		tokenQuotaEnabled, currentTokenQuota, legacySkipped := taskTokenBillingState(privateData, int64(task.Quota))
		legacyTokenAccountingSkipped = legacySkipped
		tokenSettlementDelta = int64(actualQuota) - currentTokenQuota
		if tokenQuotaEnabled && tokenSettlementDelta != 0 {
			var token Token
			if err := tx.Select("id", "key").First(&token, privateData.TokenId).Error; err == nil {
				tokenKey = token.Key
				if err := tx.Model(&Token{}).Where("id = ?", privateData.TokenId).Updates(map[string]interface{}{
					"remain_quota":  gorm.Expr("remain_quota - ?", tokenSettlementDelta),
					"used_quota":    gorm.Expr("used_quota + ?", tokenSettlementDelta),
					"accessed_time": common.GetTimestamp(),
				}).Error; err != nil {
					return err
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if privateData.TokenChargedQuota != nil && tokenQuotaEnabled {
			settledTokenQuota := actualQuota
			privateData.TokenChargedQuota = &settledTokenQuota
		}

		accountingPlanDelta := taskPlanFundedQuotaSnapshot(privateData, actualQuota) - prePlanFundedQuota
		if accountingPlanDelta < 0 {
			accountingPlanDelta = 0
		}
		result := tx.Model(&Task{}).
			Where("id = ? AND billing_status = ?", task.ID, TaskBillingPending).
			Updates(map[string]interface{}{
				"quota":                 actualQuota,
				"pending_quota":         preConsumedQuota,
				"accounting_plan_delta": accountingPlanDelta,
				"billing_status":        TaskBillingAccounting,
				"private_data":          privateData,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("task settlement marker changed during transaction")
		}
		task.Quota = actualQuota
		task.PendingQuota = preConsumedQuota
		task.AccountingPlanDelta = accountingPlanDelta
		task.BillingStatus = TaskBillingAccounting
		task.PrivateData = privateData
		settledTask = task
		settled = true
		return nil
	})
	if err != nil || !settled {
		return nil, settled, err
	}
	if walletDelta != 0 && common.RedisEnabled {
		userID := settledTask.UserId
		gopool.Go(func() {
			var cacheErr error
			if walletDelta > 0 {
				cacheErr = cacheIncrUserQuota(userID, walletDelta)
			} else {
				cacheErr = cacheDecrUserQuota(userID, -walletDelta)
			}
			if cacheErr != nil {
				common.SysLog("failed to update task settlement user quota cache: " + cacheErr.Error())
			}
		})
	}
	if planUserID > 0 {
		if cacheErr := InvalidateUserPlanCache(planUserID); cacheErr != nil {
			common.SysLog("failed to invalidate settled user plan cache: " + cacheErr.Error())
		}
	}
	if tokenKey != "" && common.RedisEnabled {
		gopool.Go(func() {
			var cacheErr error
			if tokenSettlementDelta > 0 {
				cacheErr = cacheDecrTokenQuota(tokenKey, tokenSettlementDelta)
			} else if tokenSettlementDelta < 0 {
				cacheErr = cacheIncrTokenQuota(tokenKey, -tokenSettlementDelta)
			}
			if cacheErr != nil {
				common.SysLog("failed to update task settlement token quota cache: " + cacheErr.Error())
			}
		})
	}
	if legacyTokenAccountingSkipped {
		common.SysLog(fmt.Sprintf("legacy task %d token settlement skipped because exact token charge is unavailable", id))
	}
	return &settledTask, true, nil
}

// CompleteTaskBillingAccountingAtomic applies the primary-database statistics
// and closes the accounting outbox in one transaction. Repeated recovery is a
// no-op after the status leaves accounting.
func CompleteTaskBillingAccountingAtomic(id int64) (*Task, bool, error) {
	if id <= 0 {
		return nil, false, nil
	}
	var completedTask Task
	var completed bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		var task Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, id).Error; err != nil {
			return err
		}
		if task.Status != TaskStatusSuccess || task.BillingStatus != TaskBillingAccounting {
			return nil
		}
		quotaDelta := task.Quota - task.PendingQuota
		if quotaDelta > 0 {
			if err := tx.Model(&User{}).Where("id = ?", task.UserId).
				Update("used_quota", gorm.Expr("used_quota + ?", quotaDelta)).Error; err != nil {
				return err
			}
			if err := tx.Model(&Channel{}).Where("id = ?", task.ChannelId).
				Update("used_quota", gorm.Expr("used_quota + ?", quotaDelta)).Error; err != nil {
				return err
			}
		}
		result := tx.Model(&Task{}).
			Where("id = ? AND billing_status = ?", task.ID, TaskBillingAccounting).
			Updates(map[string]interface{}{
				"pending_quota":         0,
				"accounting_plan_delta": 0,
				"billing_status":        TaskBillingSettled,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("task accounting marker changed during transaction")
		}
		task.PendingQuota = 0
		task.AccountingPlanDelta = 0
		task.BillingStatus = TaskBillingSettled
		completedTask = task
		completed = true
		return nil
	})
	if err != nil || !completed {
		return nil, completed, err
	}
	return &completedTask, true, nil
}

func MarkTaskSettlementPending(id int64, actualQuota int) (bool, error) {
	if id <= 0 || actualQuota < 0 {
		return false, errors.New("task id is required and actual quota cannot be negative")
	}
	result := DB.Model(&Task{}).
		Where("id = ? AND status = ? AND billing_status = ?", id, TaskStatusSuccess, TaskBillingCalculating).
		Updates(map[string]interface{}{
			"pending_quota":  actualQuota,
			"billing_status": TaskBillingPending,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// UpdateWithStatus performs a conditional UPDATE guarded by fromStatus (CAS).
// Returns (true, nil) if this caller won the update, (false, nil) if
// another process already moved the task out of fromStatus. MySQL commonly
// reports changed rows rather than matched rows, so a same-value no-op update
// can also return false even when the status predicate still matched.
//
// Uses Model().Select("*").Updates() instead of Save() because GORM's Save
// falls back to INSERT ON CONFLICT when the WHERE-guarded UPDATE matches
// zero rows, which silently bypasses the CAS guard.
func (t *Task) UpdateWithStatus(fromStatus TaskStatus) (bool, error) {
	result := DB.Model(t).Where("status = ?", fromStatus).Select("*").Updates(t)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// TaskBulkUpdate performs an unconditional bulk UPDATE by upstream task_id strings.
// Same caveats as TaskBulkUpdateByID — no CAS guard.
func TaskBulkUpdate(TaskIds []string, params map[string]any) error {
	if len(TaskIds) == 0 {
		return nil
	}
	return DB.Model(&Task{}).
		Where("task_id in (?)", TaskIds).
		Updates(params).Error
}

func TaskBulkUpdateByTaskIds(taskIDs []int64, params map[string]any) error {
	if len(taskIDs) == 0 {
		return nil
	}
	return DB.Model(&Task{}).
		Where("id in (?)", taskIDs).
		Updates(params).Error
}

// TaskBulkUpdateByID performs an unconditional bulk UPDATE by primary key IDs.
// WARNING: This function has NO CAS (Compare-And-Swap) guard — it will overwrite
// any concurrent status changes. DO NOT use in billing/quota lifecycle flows
// (e.g., timeout, success, failure transitions that trigger refunds or settlements).
// For status transitions that involve billing, use Task.UpdateWithStatus() instead.
func TaskBulkUpdateByID(ids []int64, params map[string]any) error {
	if len(ids) == 0 {
		return nil
	}
	return DB.Model(&Task{}).
		Where("id in (?)", ids).
		Updates(params).Error
}

type TaskQuotaUsage struct {
	Mode  string  `json:"mode"`
	Count float64 `json:"count"`
}

func SumUsedTaskQuota(queryParams SyncTaskQueryParams) (stat []TaskQuotaUsage, err error) {
	query := DB.Model(Task{})
	// 添加过滤条件
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.UserID != "" {
		query = query.Where("user_id = ?", queryParams.UserID)
	}
	if len(queryParams.UserIDs) != 0 {
		query = query.Where("user_id in (?)", queryParams.UserIDs)
	}
	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	err = query.Select("mode, sum(quota) as count").Group("mode").Find(&stat).Error
	return stat, err
}

// TaskCountAllTasks returns total tasks that match the given query params (admin usage)
func TaskCountAllTasks(queryParams SyncTaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Task{})
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.UserID != "" {
		query = query.Where("user_id = ?", queryParams.UserID)
	}
	if len(queryParams.UserIDs) != 0 {
		query = query.Where("user_id in (?)", queryParams.UserIDs)
	}
	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}

// TaskCountAllUserTask returns total tasks for given user
func TaskCountAllUserTask(userId int, queryParams SyncTaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Task{}).Where("user_id = ?", userId)
	if queryParams.TaskID != "" {
		query = query.Where("task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.Platform != "" {
		query = query.Where("platform = ?", queryParams.Platform)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}

func (t *Task) ToOpenAIVideo() *dto.OpenAIVideo {
	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = t.TaskID
	openAIVideo.Status = t.Status.ToVideoStatus()
	openAIVideo.Model = t.Properties.OriginModelName
	openAIVideo.SetProgressStr(t.Progress)
	openAIVideo.CreatedAt = t.CreatedAt
	openAIVideo.CompletedAt = t.UpdatedAt
	openAIVideo.SetMetadata("url", t.GetResultURL())
	return openAIVideo
}
