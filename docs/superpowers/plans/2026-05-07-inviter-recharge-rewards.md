# 邀请下级充值统计与线下激励发放 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** `docs/superpowers/specs/2026-05-07-inviter-recharge-rewards-design.md`

**Goal:** 在管理员端用户详情 Modal 增加"邀请充值"Tab，展示该用户邀请下级的充值明细 + 4 项 KPI 汇总，并支持管理员录入线下打款金额，把已发放的充值标记掉避免下次重复算入。

**Architecture:** `top_ups` 表新增 `inviter_reward_payout_id` 列；新增 `inviter_reward_payouts` 表存放每次发放台账。后端三个 admin API（GET 明细 / GET 历史 / POST 创建发放，POST 走事务 `FOR UPDATE` 防并发重复）。前端在 `UserDetailModal` 新增 TabPane + 两段表格 + 发放弹窗。

**Tech Stack:** Go (gorm + gin)、React (semi-ui + react-i18next)、SQLite/MySQL/PostgreSQL（gorm 抽象）。后端测试用 in-memory SQLite，已有 `controller/admin_plan_order_test.go` 模板可参考。

---

## File Structure

**新增文件**
- `model/inviter_reward_payout.go` — `InviterRewardPayout` GORM 模型 + 查询/创建函数
- `model/inviter_reward_payout_test.go` — 模型层单测（KPI 汇总、并发互斥）
- `controller/inviter_reward.go` — 3 个 admin HTTP handler
- `controller/inviter_reward_test.go` — controller 集成测试
- `web/src/services/inviterRewardApi.js` — 前端 API 包装
- `web/src/components/table/users/modals/InviteeRechargesTab.jsx` — Tab 主体
- `web/src/components/table/users/modals/PayoutInviterRewardModal.jsx` — 发放弹窗

**修改文件**
- `model/topup.go` — `TopUp` struct 加 `InviterRewardPayoutId int` 字段
- `model/main.go` — `migrateDB()` 与 `migrateDBFast()` 注册 `&InviterRewardPayout{}`
- `model/option.go` — `OptionMap` 注册 `InviterRewardDefaultPercent` + 解析 case
- `common/constants.go` — 新增 `var InviterRewardDefaultPercent float64 = 10`
- `router/api-router.go` — `adminRoute` 新增 3 条路由
- `web/src/components/table/users/modals/UserDetailModal.jsx` — 追加新 TabPane
- `web/src/i18n/locales/en.json` — 追加新 i18n 字符串

---

## Phase 1 — 后端数据模型与迁移

### Task 1: 定义 `InviterRewardPayout` 模型 + 在 `top_ups` 加字段 + 注册迁移

**Files:**
- Create: `model/inviter_reward_payout.go`
- Modify: `model/topup.go:15-25` (struct 字段)
- Modify: `model/main.go:260-288` (`migrateDB` AutoMigrate 列表) 和 `model/main.go:462-491` (`migrateDBFast` 列表)

- [ ] **Step 1: 写出新模型文件骨架（仅 struct，无函数）**

Create `model/inviter_reward_payout.go`:

```go
package model

// InviterRewardPayout 记录管理员一次"线下激励发放"的批次台账。
// 每个 payout 覆盖一组 status=success 的 top_ups（通过 top_ups.inviter_reward_payout_id 关联）。
type InviterRewardPayout struct {
	Id               int     `json:"id" gorm:"primaryKey;autoIncrement"`
	InviterUserId    int     `json:"inviter_user_id" gorm:"index;not null"`
	RechargeTotalUsd float64 `json:"recharge_total_usd" gorm:"not null"`
	PayoutAmountUsd  float64 `json:"payout_amount_usd" gorm:"not null"`
	DefaultPctUsed   float64 `json:"default_pct_used"`
	Note             string  `json:"note" gorm:"type:varchar(500)"`
	OperatorAdminId  int     `json:"operator_admin_id" gorm:"index;not null"`
	CreatedAt        int64   `json:"created_at" gorm:"index;autoCreateTime"`
}
```

- [ ] **Step 2: 给 `TopUp` 加 `InviterRewardPayoutId` 字段**

Edit `model/topup.go:15-25`. After `Status string \`json:"status"\``，新增一行：

```go
type TopUp struct {
	Id                    int     `json:"id"`
	UserId                int     `json:"user_id" gorm:"index"`
	Amount                int64   `json:"amount"`
	Money                 float64 `json:"money"`
	TradeNo               string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod         string  `json:"payment_method" gorm:"type:varchar(50)"`
	CreateTime            int64   `json:"create_time"`
	CompleteTime          int64   `json:"complete_time"`
	Status                string  `json:"status"`
	InviterRewardPayoutId int     `json:"inviter_reward_payout_id" gorm:"index;default:0"`
}
```

- [ ] **Step 3: 注册到 AutoMigrate 列表**

Edit `model/main.go`. 在 `migrateDB()` 的 `DB.AutoMigrate(...)` 列表（约第 260-288 行）末尾追加 `&InviterRewardPayout{}`：

```go
&ChannelDisableRule{},
&InviterRewardPayout{},
)
```

同步在 `migrateDBFast()` 的 `migrations` 切片（约第 462-491 行）末尾追加：

```go
{&ChannelDisableRule{}, "ChannelDisableRule"},
{&InviterRewardPayout{}, "InviterRewardPayout"},
}
```

- [ ] **Step 4: 启动 server 验证迁移成功**

Run: `go build ./... && SQL_DSN=local ./new-api 2>&1 | head -40`

Expected: 包含 `database migration started`，无 panic / error；表 `inviter_reward_payouts` 与 `top_ups.inviter_reward_payout_id` 列存在。

按 Ctrl+C 停止。

- [ ] **Step 5: 提交**

```bash
git add model/inviter_reward_payout.go model/topup.go model/main.go
git commit -m "feat(inviter-reward): add InviterRewardPayout model + topup payout_id column"
```

---

### Task 2: 写模型层查询函数（含并发安全的发放函数）

**Files:**
- Modify: `model/inviter_reward_payout.go`
- Create: `model/inviter_reward_payout_test.go`

- [ ] **Step 1: 写测试 setup helper 与第一个测试（汇总查询）**

Create `model/inviter_reward_payout_test.go`:

```go
package model

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupInviterRewardTestDB(t *testing.T) {
	t.Helper()
	dsn := fmt.Sprintf("file:inviter_reward_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	DB = db
	if err := db.AutoMigrate(&User{}, &TopUp{}, &InviterRewardPayout{}, &Log{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// 创建 1 个 inviter，1 个 invitee，invitee 的 N 笔成功 + M 笔 pending 充值。
// 返回 inviterId, inviteeId。
func seedInviterAndTopups(t *testing.T, success []float64, pending []float64) (int, int) {
	t.Helper()
	inviter := &User{Username: "inv-" + fmt.Sprint(time.Now().UnixNano()), Password: "x"}
	if err := DB.Create(inviter).Error; err != nil {
		t.Fatalf("create inviter: %v", err)
	}
	invitee := &User{Username: "ee-" + fmt.Sprint(time.Now().UnixNano()), Password: "x", InviterId: inviter.Id}
	if err := DB.Create(invitee).Error; err != nil {
		t.Fatalf("create invitee: %v", err)
	}
	for _, m := range success {
		if err := DB.Create(&TopUp{UserId: invitee.Id, Money: m, Status: common.TopUpStatusSuccess, TradeNo: fmt.Sprintf("ok-%d-%f", time.Now().UnixNano(), m)}).Error; err != nil {
			t.Fatalf("create topup: %v", err)
		}
	}
	for _, m := range pending {
		if err := DB.Create(&TopUp{UserId: invitee.Id, Money: m, Status: common.TopUpStatusPending, TradeNo: fmt.Sprintf("pending-%d-%f", time.Now().UnixNano(), m)}).Error; err != nil {
			t.Fatalf("create pending topup: %v", err)
		}
	}
	return inviter.Id, invitee.Id
}

func TestGetInviteeRechargeSummary_Empty(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviter := &User{Username: "lonely", Password: "x"}
	if err := DB.Create(inviter).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	s, err := GetInviteeRechargeSummary(inviter.Id)
	if err != nil {
		t.Fatalf("summary err: %v", err)
	}
	if s.InviteeCount != 0 || s.RechargeTotalUsd != 0 || s.PayoutTotalUsd != 0 || s.PendingTotalUsd != 0 {
		t.Fatalf("expected all zeros, got %+v", s)
	}
}

func TestGetInviteeRechargeSummary_SuccessOnlyCounts(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, _ := seedInviterAndTopups(t, []float64{10, 25}, []float64{99}) // pending should NOT count
	s, err := GetInviteeRechargeSummary(inviterId)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if s.InviteeCount != 1 {
		t.Fatalf("InviteeCount want 1, got %d", s.InviteeCount)
	}
	if s.RechargeTotalUsd != 35 {
		t.Fatalf("RechargeTotalUsd want 35, got %v", s.RechargeTotalUsd)
	}
	if s.PendingTotalUsd != 35 {
		t.Fatalf("PendingTotalUsd want 35, got %v", s.PendingTotalUsd)
	}
	if s.PayoutTotalUsd != 0 {
		t.Fatalf("PayoutTotalUsd want 0, got %v", s.PayoutTotalUsd)
	}
}
```

- [ ] **Step 2: 跑测试，确认 fail（函数还没写）**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go test ./model/ -run TestGetInviteeRechargeSummary -v`

Expected: `undefined: GetInviteeRechargeSummary` 编译失败。

- [ ] **Step 3: 实现 `InviteeRechargeSummary` struct + `GetInviteeRechargeSummary`**

Append to `model/inviter_reward_payout.go`:

```go
import (
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// InviteeRechargeSummary 是 GET invitee-recharges 接口的汇总块。
type InviteeRechargeSummary struct {
	InviteeCount     int     `json:"invitee_count"`
	RechargeTotalUsd float64 `json:"recharge_total_usd"`
	PayoutTotalUsd   float64 `json:"payout_total_usd"`
	PendingTotalUsd  float64 `json:"pending_total_usd"`
}

func GetInviteeRechargeSummary(inviterUserId int) (*InviteeRechargeSummary, error) {
	s := &InviteeRechargeSummary{}

	// invitee_count
	var c int64
	if err := DB.Model(&User{}).Where("inviter_id = ?", inviterUserId).Count(&c).Error; err != nil {
		return nil, err
	}
	s.InviteeCount = int(c)

	// recharge_total_usd  &  pending_total_usd
	type sumRow struct {
		Total float64
	}
	var row sumRow
	if err := DB.Table("top_ups").
		Joins("JOIN users u ON u.id = top_ups.user_id").
		Where("u.inviter_id = ? AND top_ups.status = ?", inviterUserId, common.TopUpStatusSuccess).
		Select("COALESCE(SUM(top_ups.money), 0) AS total").
		Scan(&row).Error; err != nil {
		return nil, err
	}
	s.RechargeTotalUsd = row.Total

	row = sumRow{}
	if err := DB.Table("top_ups").
		Joins("JOIN users u ON u.id = top_ups.user_id").
		Where("u.inviter_id = ? AND top_ups.status = ? AND top_ups.inviter_reward_payout_id = ?", inviterUserId, common.TopUpStatusSuccess, 0).
		Select("COALESCE(SUM(top_ups.money), 0) AS total").
		Scan(&row).Error; err != nil {
		return nil, err
	}
	s.PendingTotalUsd = row.Total

	// payout_total_usd
	row = sumRow{}
	if err := DB.Model(&InviterRewardPayout{}).
		Where("inviter_user_id = ?", inviterUserId).
		Select("COALESCE(SUM(payout_amount_usd), 0) AS total").
		Scan(&row).Error; err != nil {
		return nil, err
	}
	s.PayoutTotalUsd = row.Total

	return s, nil
}
```

- [ ] **Step 4: 跑测试，确认通过**

Run: `go test ./model/ -run TestGetInviteeRechargeSummary -v`

Expected: 两个 PASS。

- [ ] **Step 5: 写明细列表查询测试**

Append to `model/inviter_reward_payout_test.go`:

```go
func TestGetInviteeRechargeItems_PaginationAndOrder(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, _ := seedInviterAndTopups(t, []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, nil)

	items, total, err := GetInviteeRechargeItems(inviterId, &common.PageInfo{Page: 1, PageSize: 3})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if total != 10 {
		t.Fatalf("total want 10, got %d", total)
	}
	if len(items) != 3 {
		t.Fatalf("page items want 3, got %d", len(items))
	}
	// id desc 排序：最新插入的金额是 10
	if items[0].MoneyUsd != 10 {
		t.Fatalf("first item money want 10, got %v", items[0].MoneyUsd)
	}
}
```

- [ ] **Step 6: 跑测试，fail**

Run: `go test ./model/ -run TestGetInviteeRechargeItems -v`

Expected: `undefined: GetInviteeRechargeItems`.

- [ ] **Step 7: 实现 `InviteeRechargeItem` + `GetInviteeRechargeItems`**

Append to `model/inviter_reward_payout.go`:

```go
type InviteeRechargeItem struct {
	TopupId         int     `json:"topup_id"`
	InviteeUserId   int     `json:"invitee_user_id"`
	InviteeUsername string  `json:"invitee_username"`
	MoneyUsd        float64 `json:"money_usd"`
	PaymentMethod   string  `json:"payment_method"`
	TradeNo         string  `json:"trade_no"`
	CompleteTime    int64   `json:"complete_time"`
	PayoutId        int     `json:"payout_id"`
}

func GetInviteeRechargeItems(inviterUserId int, p *common.PageInfo) ([]*InviteeRechargeItem, int64, error) {
	var total int64
	q := DB.Table("top_ups").
		Joins("JOIN users u ON u.id = top_ups.user_id").
		Where("u.inviter_id = ? AND top_ups.status = ?", inviterUserId, common.TopUpStatusSuccess)

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []*InviteeRechargeItem
	err := q.
		Select(`top_ups.id          AS topup_id,
                top_ups.user_id     AS invitee_user_id,
                u.username          AS invitee_username,
                top_ups.money       AS money_usd,
                top_ups.payment_method,
                top_ups.trade_no,
                top_ups.complete_time,
                top_ups.inviter_reward_payout_id AS payout_id`).
		Order("top_ups.id DESC").
		Limit(p.GetPageSize()).
		Offset(p.GetStartIdx()).
		Scan(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
```

- [ ] **Step 8: 跑测试，确认通过**

Run: `go test ./model/ -run TestGetInviteeRechargeItems -v`

Expected: PASS.

- [ ] **Step 9: 写发放历史查询测试**

Append:

```go
func TestGetInviterRewardPayoutHistory(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, _ := seedInviterAndTopups(t, nil, nil)
	for i := 0; i < 3; i++ {
		if err := DB.Create(&InviterRewardPayout{
			InviterUserId:    inviterId,
			RechargeTotalUsd: 100,
			PayoutAmountUsd:  10,
			OperatorAdminId:  1,
		}).Error; err != nil {
			t.Fatalf("create payout: %v", err)
		}
	}
	items, total, err := GetInviterRewardPayoutHistory(inviterId, &common.PageInfo{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if total != 3 {
		t.Fatalf("total want 3, got %d", total)
	}
	if len(items) != 2 {
		t.Fatalf("page items want 2, got %d", len(items))
	}
}
```

- [ ] **Step 10: 跑测试，fail**

Run: `go test ./model/ -run TestGetInviterRewardPayoutHistory -v`

Expected: `undefined`.

- [ ] **Step 11: 实现 `GetInviterRewardPayoutHistory`**

Append:

```go
func GetInviterRewardPayoutHistory(inviterUserId int, p *common.PageInfo) ([]*InviterRewardPayout, int64, error) {
	var total int64
	q := DB.Model(&InviterRewardPayout{}).Where("inviter_user_id = ?", inviterUserId)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*InviterRewardPayout
	if err := q.Order("id DESC").Limit(p.GetPageSize()).Offset(p.GetStartIdx()).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
```

- [ ] **Step 12: 跑测试，PASS**

Run: `go test ./model/ -run TestGetInviterRewardPayoutHistory -v`

Expected: PASS.

- [ ] **Step 13: 写"创建发放"测试 — 正常路径**

Append:

```go
func TestCreateInviterRewardPayout_Happy(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, _ := seedInviterAndTopups(t, []float64{30, 70}, []float64{50})

	payout, err := CreateInviterRewardPayout(inviterId, 25.50, "test note", 10.0, 999)
	if err != nil {
		t.Fatalf("create err: %v", err)
	}
	if payout.RechargeTotalUsd != 100 {
		t.Fatalf("recharge_total want 100, got %v", payout.RechargeTotalUsd)
	}
	if payout.PayoutAmountUsd != 25.50 {
		t.Fatalf("payout amount want 25.50, got %v", payout.PayoutAmountUsd)
	}
	if payout.Note != "test note" || payout.OperatorAdminId != 999 || payout.DefaultPctUsed != 10.0 {
		t.Fatalf("metadata mismatch: %+v", payout)
	}

	// 第二次发放应该返回 ErrNoPendingRecharges（已无 pending），等价于"防双发"
	_, err = CreateInviterRewardPayout(inviterId, 5, "again", 10.0, 999)
	if !errors.Is(err, ErrNoPendingRecharges) {
		t.Fatalf("second call want ErrNoPendingRecharges, got %v", err)
	}

	// 验证受影响的 topup 行 inviter_reward_payout_id 已被更新
	// （JOIN invitee 确认该 inviter 名下未发放的 success 充值汇总现在为 0）
	row := struct{ Total float64 }{}
	if err := DB.Table("top_ups").
		Joins("JOIN users u ON u.id = top_ups.user_id").
		Where("u.inviter_id = ? AND top_ups.status = ? AND top_ups.inviter_reward_payout_id = 0",
			inviterId, common.TopUpStatusSuccess).
		Select("COALESCE(SUM(top_ups.money), 0) AS total").
		Scan(&row).Error; err != nil {
		t.Fatalf("verify pending sum: %v", err)
	}
	if row.Total != 0 {
		t.Fatalf("after payout pending sum want 0, got %v", row.Total)
	}
}

func TestCreateInviterRewardPayout_RejectsNonPositive(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, _ := seedInviterAndTopups(t, []float64{10}, nil)
	if _, err := CreateInviterRewardPayout(inviterId, 0, "", 10.0, 1); !errors.Is(err, ErrInvalidPayoutAmount) {
		t.Fatalf("zero amount want ErrInvalidPayoutAmount, got %v", err)
	}
	if _, err := CreateInviterRewardPayout(inviterId, -5, "", 10.0, 1); !errors.Is(err, ErrInvalidPayoutAmount) {
		t.Fatalf("negative amount want ErrInvalidPayoutAmount, got %v", err)
	}
}

func TestCreateInviterRewardPayout_RejectsWhenNoPending(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, _ := seedInviterAndTopups(t, nil, []float64{99}) // pending only
	if _, err := CreateInviterRewardPayout(inviterId, 10, "", 10.0, 1); !errors.Is(err, ErrNoPendingRecharges) {
		t.Fatalf("want ErrNoPendingRecharges, got %v", err)
	}
}
```

- [ ] **Step 14: 跑测试，fail**

Run: `go test ./model/ -run TestCreateInviterRewardPayout -v`

Expected: 编译失败 (`undefined`).

- [ ] **Step 15: 实现 `CreateInviterRewardPayout` + 错误常量**

Append to `model/inviter_reward_payout.go`:

```go
var (
	ErrNoPendingRecharges  = errors.New("暂无待激励充值")
	ErrInvalidPayoutAmount = errors.New("奖励金额必须大于 0")
)

// CreateInviterRewardPayout 在事务中：
//   1) FOR UPDATE 锁定该 inviter 下所有未发放 (status=success, payout_id=0) 的 top_ups
//   2) 校验金额 > 0、行数 > 0
//   3) 插入一条 InviterRewardPayout
//   4) 把锁定的 top_ups 全部 UPDATE 为新 payout_id
//   5) 写一条 LogTypeManage 日志
func CreateInviterRewardPayout(inviterUserId int, payoutAmountUsd float64, note string, defaultPctUsed float64, operatorAdminId int) (*InviterRewardPayout, error) {
	if payoutAmountUsd <= 0 {
		return nil, ErrInvalidPayoutAmount
	}

	var created *InviterRewardPayout
	err := DB.Transaction(func(tx *gorm.DB) error {
		// 1) 锁定未发放的 top_ups
		var rows []struct {
			Id    int
			Money float64
		}
		if err := tx.Table("top_ups").
			Set("gorm:query_option", "FOR UPDATE").
			Joins("JOIN users u ON u.id = top_ups.user_id").
			Where("u.inviter_id = ? AND top_ups.status = ? AND top_ups.inviter_reward_payout_id = ?",
				inviterUserId, common.TopUpStatusSuccess, 0).
			Select("top_ups.id AS id, top_ups.money AS money").
			Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNoPendingRecharges
		}

		var rechargeTotal float64
		ids := make([]int, 0, len(rows))
		for _, r := range rows {
			rechargeTotal += r.Money
			ids = append(ids, r.Id)
		}

		// 2) 插入 payout
		p := &InviterRewardPayout{
			InviterUserId:    inviterUserId,
			RechargeTotalUsd: rechargeTotal,
			PayoutAmountUsd:  payoutAmountUsd,
			DefaultPctUsed:   defaultPctUsed,
			Note:             note,
			OperatorAdminId:  operatorAdminId,
		}
		if err := tx.Create(p).Error; err != nil {
			return err
		}

		// 3) 更新 top_ups
		if err := tx.Model(&TopUp{}).Where("id IN ?", ids).Update("inviter_reward_payout_id", p.Id).Error; err != nil {
			return err
		}

		created = p
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 事务外写日志（非关键，失败不回滚业务）
	RecordLog(inviterUserId, LogTypeManage,
		fmt.Sprintf("管理员 #%d 为该用户发放邀请激励 $%.2f，覆盖充值 $%.2f，批次 #%d",
			operatorAdminId, created.PayoutAmountUsd, created.RechargeTotalUsd, created.Id))

	return created, nil
}
```

更新文件顶部的 import 块，确保包含 `"fmt"`：

```go
import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)
```

- [ ] **Step 16: 跑测试，全部 PASS**

Run: `go test ./model/ -run TestCreateInviterRewardPayout -v`

Expected: 三个测试全部 PASS。

- [ ] **Step 17: 跑全部 model 测试，确认未破坏其它**

Run: `go test ./model/ -v -count=1 2>&1 | tail -30`

Expected: 所有原有测试依旧 PASS。

- [ ] **Step 18: 提交**

```bash
git add model/inviter_reward_payout.go model/inviter_reward_payout_test.go
git commit -m "feat(inviter-reward): query + transactional payout creation in model layer"
```

---

## Phase 2 — 后端配置项

### Task 3: 新增 `InviterRewardDefaultPercent` 配置

**Files:**
- Modify: `common/constants.go:104` 附近（`QuotaForInviter` 旁边）
- Modify: `model/option.go:109` 附近（`OptionMap` 注册）+ `model/option.go:402` 附近（switch case）

- [ ] **Step 1: 添加常量声明**

Edit `common/constants.go`. 在 `var QuotaForInviter = 0` 这一行之后追加：

```go
var QuotaForInviter = 0
var InviterRewardDefaultPercent float64 = 10
```

- [ ] **Step 2: 在 OptionMap 注册（让 GET options 能读到）**

Edit `model/option.go`. 找到 `common.OptionMap["QuotaForInviter"] = strconv.Itoa(common.QuotaForInviter)` 这一行（约 109 行），其后追加：

```go
common.OptionMap["QuotaForInviter"] = strconv.Itoa(common.QuotaForInviter)
common.OptionMap["InviterRewardDefaultPercent"] = strconv.FormatFloat(common.InviterRewardDefaultPercent, 'f', -1, 64)
```

- [ ] **Step 3: 在 switch case 注册（让 PUT options 能写）**

Edit `model/option.go`. 找到 `case "QuotaForInviter":` 这一段（约 402 行），在它之后追加：

```go
case "QuotaForInviter":
	common.QuotaForInviter, _ = strconv.Atoi(value)
case "InviterRewardDefaultPercent":
	if v, err := strconv.ParseFloat(value, 64); err == nil {
		common.InviterRewardDefaultPercent = v
	}
```

- [ ] **Step 4: 编译验证**

Run: `go build ./...`

Expected: 无错误。

- [ ] **Step 5: 提交**

```bash
git add common/constants.go model/option.go
git commit -m "feat(inviter-reward): add InviterRewardDefaultPercent option (default 10%)"
```

---

## Phase 3 — 后端 HTTP 层

### Task 4: 实现 GET 明细 + GET 历史 handler

**Files:**
- Create: `controller/inviter_reward.go`
- Create: `controller/inviter_reward_test.go`

- [ ] **Step 1: 写 controller test 骨架 + GET 明细测试**

Create `controller/inviter_reward_test.go`:

```go
package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type apiEnvelope struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

func setupInviterRewardCtlTestDB(t *testing.T) {
	t.Helper()
	dsn := fmt.Sprintf("file:inviter_reward_ctl_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	model.DB = db
	if err := db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.InviterRewardPayout{}, &model.Log{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// 构造一个带 operator id=1 的 admin 路由。
func newRouterWithAdmin() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("id", 1)
		c.Set("role", common.RoleAdminUser)
		c.Next()
	})
	r.GET("/api/user/manage/:id/invitee-recharges", GetInviteeRecharges)
	r.GET("/api/user/manage/:id/inviter-reward-payouts", GetInviterRewardPayouts)
	r.POST("/api/user/manage/:id/inviter-reward-payouts", CreateInviterRewardPayoutHandler)
	return r
}

func seedTwoInviteesWithTopups(t *testing.T) int {
	t.Helper()
	inviter := &model.User{Username: "inviter-x", Password: "x"}
	if err := model.DB.Create(inviter).Error; err != nil {
		t.Fatalf("create inviter: %v", err)
	}
	for i := 0; i < 2; i++ {
		invitee := &model.User{Username: fmt.Sprintf("ee-%d", i), Password: "x", InviterId: inviter.Id}
		if err := model.DB.Create(invitee).Error; err != nil {
			t.Fatalf("create invitee: %v", err)
		}
		// each invitee: 2 success topups
		for j := 0; j < 2; j++ {
			if err := model.DB.Create(&model.TopUp{
				UserId: invitee.Id, Money: float64((i+1)*10) + float64(j),
				Status: common.TopUpStatusSuccess,
				TradeNo: fmt.Sprintf("tn-%d-%d-%d", time.Now().UnixNano(), i, j),
			}).Error; err != nil {
				t.Fatalf("create topup: %v", err)
			}
		}
	}
	return inviter.Id
}

func TestGetInviteeRecharges_SummaryAndItems(t *testing.T) {
	setupInviterRewardCtlTestDB(t)
	inviterId := seedTwoInviteesWithTopups(t)

	r := newRouterWithAdmin()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/user/manage/%d/invitee-recharges?page=1&page_size=10", inviterId), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var env apiEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.Success {
		t.Fatalf("success=false: %v", env.Message)
	}
	summary := env.Data["summary"].(map[string]interface{})
	if int(summary["invitee_count"].(float64)) != 2 {
		t.Fatalf("invitee_count want 2, got %v", summary["invitee_count"])
	}
	// 充值汇总：(10+11) + (20+21) = 62
	if summary["recharge_total_usd"].(float64) != 62 {
		t.Fatalf("recharge_total want 62, got %v", summary["recharge_total_usd"])
	}
	if summary["pending_total_usd"].(float64) != 62 {
		t.Fatalf("pending_total want 62, got %v", summary["pending_total_usd"])
	}
	if summary["payout_total_usd"].(float64) != 0 {
		t.Fatalf("payout_total want 0, got %v", summary["payout_total_usd"])
	}
	items := env.Data["items"].([]interface{})
	if len(items) != 4 {
		t.Fatalf("items want 4, got %d", len(items))
	}
}
```

- [ ] **Step 2: 跑测试，fail（handler 还没写）**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go test ./controller/ -run TestGetInviteeRecharges -v`

Expected: 编译失败 `undefined: GetInviteeRecharges`.

- [ ] **Step 3: 实现 GET 明细 handler**

> 关于分页：`common.GetPageQuery` 已存在，但读的是 `?p=` / `?page_size=`（历史命名）。我们的前端发送 `?page=` / `?page_size=`（与 spec 一致），所以本 plan 使用一个本地小 helper `parsePage`，避免命名混淆。

Create `controller/inviter_reward.go`:

```go
package controller

import (
	"encoding/json"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// parsePage reads ?page=&page_size= with sensible defaults & caps.
func parsePage(c *gin.Context) *common.PageInfo {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if size < 1 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	return &common.PageInfo{Page: page, PageSize: size}
}

func parseInviterIdParam(c *gin.Context) (int, bool) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的用户ID")
		return 0, false
	}
	return id, true
}

// GET /api/user/manage/:id/invitee-recharges
func GetInviteeRecharges(c *gin.Context) {
	inviterId, ok := parseInviterIdParam(c)
	if !ok {
		return
	}
	pageInfo := parsePage(c)

	summary, err := model.GetInviteeRechargeSummary(inviterId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items, total, err := model.GetInviteeRechargeItems(inviterId, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"summary": summary,
		"items":   items,
		"pagination": gin.H{
			"page":      pageInfo.Page,
			"page_size": pageInfo.PageSize,
			"total":     total,
		},
		"default_percent": common.InviterRewardDefaultPercent,
	})
}
```

- [ ] **Step 4: 跑测试，PASS**

Run: `go test ./controller/ -run TestGetInviteeRecharges -v`

Expected: PASS。

- [ ] **Step 5: 写 GET 历史 handler 测试**

Append to `controller/inviter_reward_test.go`:

```go
func TestGetInviterRewardPayouts_History(t *testing.T) {
	setupInviterRewardCtlTestDB(t)
	inviterId := seedTwoInviteesWithTopups(t)
	for i := 0; i < 3; i++ {
		if err := model.DB.Create(&model.InviterRewardPayout{
			InviterUserId: inviterId, RechargeTotalUsd: 100, PayoutAmountUsd: 10,
			OperatorAdminId: 1,
		}).Error; err != nil {
			t.Fatalf("seed payout: %v", err)
		}
	}
	r := newRouterWithAdmin()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/user/manage/%d/inviter-reward-payouts?page=1&page_size=2", inviterId), nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	var env apiEnvelope
	json.Unmarshal(w.Body.Bytes(), &env)
	items := env.Data["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("page items want 2, got %d", len(items))
	}
	pg := env.Data["pagination"].(map[string]interface{})
	if int(pg["total"].(float64)) != 3 {
		t.Fatalf("total want 3, got %v", pg["total"])
	}
}
```

- [ ] **Step 6: 跑测试，fail**

Run: `go test ./controller/ -run TestGetInviterRewardPayouts -v`

Expected: undefined.

- [ ] **Step 7: 实现 GET 历史 handler**

Append to `controller/inviter_reward.go`:

```go
// GET /api/user/manage/:id/inviter-reward-payouts
func GetInviterRewardPayouts(c *gin.Context) {
	inviterId, ok := parseInviterIdParam(c)
	if !ok {
		return
	}
	pageInfo := parsePage(c)
	items, total, err := model.GetInviterRewardPayoutHistory(inviterId, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items": items,
		"pagination": gin.H{
			"page":      pageInfo.Page,
			"page_size": pageInfo.PageSize,
			"total":     total,
		},
	})
}
```

- [ ] **Step 8: 跑测试，PASS**

Run: `go test ./controller/ -run TestGetInviterRewardPayouts -v`

Expected: PASS。

- [ ] **Step 9: 提交**

```bash
git add controller/inviter_reward.go controller/inviter_reward_test.go
git commit -m "feat(inviter-reward): GET handlers for invitee recharge details and payout history"
```

---

### Task 5: 实现 POST 创建发放 handler

**Files:**
- Modify: `controller/inviter_reward.go`
- Modify: `controller/inviter_reward_test.go`

- [ ] **Step 1: 写测试 — 正常创建**

Append to `controller/inviter_reward_test.go`:

```go
func TestCreateInviterRewardPayoutHandler_Happy(t *testing.T) {
	setupInviterRewardCtlTestDB(t)
	inviterId := seedTwoInviteesWithTopups(t)
	r := newRouterWithAdmin()

	body := `{"payout_amount_usd": 6.20, "note": "first batch"}`
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/user/manage/%d/inviter-reward-payouts", inviterId),
		bytesReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	var env apiEnvelope
	json.Unmarshal(w.Body.Bytes(), &env)
	if !env.Success {
		t.Fatalf("success=false: %s", env.Message)
	}
	if env.Data["payout_amount_usd"].(float64) != 6.20 {
		t.Fatalf("payout_amount mismatch: %v", env.Data["payout_amount_usd"])
	}
	if env.Data["recharge_total_usd"].(float64) != 62 {
		t.Fatalf("recharge_total want 62, got %v", env.Data["recharge_total_usd"])
	}
}

func TestCreateInviterRewardPayoutHandler_NoPending(t *testing.T) {
	setupInviterRewardCtlTestDB(t)
	inviter := &model.User{Username: "lonely", Password: "x"}
	model.DB.Create(inviter)
	r := newRouterWithAdmin()
	body := `{"payout_amount_usd": 1, "note": ""}`
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/user/manage/%d/inviter-reward-payouts", inviter.Id),
		bytesReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	var env apiEnvelope
	json.Unmarshal(w.Body.Bytes(), &env)
	if env.Success {
		t.Fatalf("expected failure, got success")
	}
	if env.Message != "暂无待激励充值" {
		t.Fatalf("message want '暂无待激励充值', got %q", env.Message)
	}
}

func TestCreateInviterRewardPayoutHandler_BadAmount(t *testing.T) {
	setupInviterRewardCtlTestDB(t)
	inviterId := seedTwoInviteesWithTopups(t)
	r := newRouterWithAdmin()
	body := `{"payout_amount_usd": 0}`
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/user/manage/%d/inviter-reward-payouts", inviterId),
		bytesReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env apiEnvelope
	json.Unmarshal(w.Body.Bytes(), &env)
	if env.Success {
		t.Fatalf("expected failure")
	}
	if env.Message != "奖励金额必须大于 0" {
		t.Fatalf("got %q", env.Message)
	}
}
```

在 `controller/inviter_reward_test.go` 顶部 import 加上 `"strings"` 和 `"io"`，并在文件底部追加辅助：

```go
func bytesReader(s string) io.Reader { return strings.NewReader(s) }
```

- [ ] **Step 2: 跑测试，fail**

Run: `go test ./controller/ -run TestCreateInviterRewardPayoutHandler -v`

Expected: undefined / 编译失败。

- [ ] **Step 3: 实现 POST handler**

Append to `controller/inviter_reward.go`:

```go
type createInviterRewardPayoutRequest struct {
	PayoutAmountUsd float64 `json:"payout_amount_usd"`
	Note            string  `json:"note"`
}

// POST /api/user/manage/:id/inviter-reward-payouts
func CreateInviterRewardPayoutHandler(c *gin.Context) {
	inviterId, ok := parseInviterIdParam(c)
	if !ok {
		return
	}
	var req createInviterRewardPayoutRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	operatorId := c.GetInt("id")

	payout, err := model.CreateInviterRewardPayout(
		inviterId,
		req.PayoutAmountUsd,
		req.Note,
		common.InviterRewardDefaultPercent,
		operatorId,
	)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, payout)
}
```

- [ ] **Step 4: 跑测试，PASS**

Run: `go test ./controller/ -run TestCreateInviterRewardPayoutHandler -v`

Expected: 三个测试全部 PASS。

- [ ] **Step 5: 跑全部 controller 测试**

Run: `go test ./controller/ -v -count=1 2>&1 | tail -30`

Expected: 全部 PASS。

- [ ] **Step 6: 提交**

```bash
git add controller/inviter_reward.go controller/inviter_reward_test.go
git commit -m "feat(inviter-reward): POST handler creating offline payout batches"
```

---

### Task 6: 注册路由

**Files:**
- Modify: `router/api-router.go:150`

- [ ] **Step 1: 在 adminRoute 块新增 3 条路由**

Edit `router/api-router.go`. 在 `adminRoute.POST("/manage/inviter", controller.SetUserInviter)` 这一行之后追加：

```go
adminRoute.POST("/manage/inviter", controller.SetUserInviter)
adminRoute.GET("/manage/:id/invitee-recharges", controller.GetInviteeRecharges)
adminRoute.GET("/manage/:id/inviter-reward-payouts", controller.GetInviterRewardPayouts)
adminRoute.POST("/manage/:id/inviter-reward-payouts", controller.CreateInviterRewardPayoutHandler)
```

- [ ] **Step 2: 编译 + 启动 server，烟雾测试**

Run: `go build ./... && SQL_DSN=local ./new-api 2>&1 &  sleep 2 && curl -s http://localhost:3000/api/user/manage/1/invitee-recharges | head -3 ; kill %1 2>/dev/null || true`

Expected: 返回 401/403 之类的鉴权错误（因为没有 cookie），**不是** 404 或 panic。这说明路由已挂上。

- [ ] **Step 3: 提交**

```bash
git add router/api-router.go
git commit -m "feat(inviter-reward): wire 3 admin routes for invitee recharge stats"
```

---

## Phase 4 — 前端实现

### Task 7: 添加前端 API 服务模块

**Files:**
- Create: `web/src/services/inviterRewardApi.js`

- [ ] **Step 1: 写 service 文件**

Create `web/src/services/inviterRewardApi.js`:

```javascript
/*
Copyright (C) 2025 QuantumNous

Inviter reward (offline payout ledger) API client.
*/

import { API, showError } from '../helpers';

const BASE = '/api/user/manage';

export const InviterRewardAPI = {
  /**
   * Fetch invitee recharge summary + paginated detail rows for inviter user.
   * @param {number} inviterId
   * @param {number} page
   * @param {number} pageSize
   * @returns {Promise<{summary, items, pagination, default_percent}>}
   */
  async fetchInviteeRecharges(inviterId, page = 1, pageSize = 20) {
    try {
      const res = await API.get(`${BASE}/${inviterId}/invitee-recharges`, {
        params: { page, page_size: pageSize },
      });
      if (res.data.success) return res.data.data;
      throw new Error(res.data.message || 'Failed to fetch invitee recharges');
    } catch (err) {
      showError(err.message || 'Failed to fetch invitee recharges');
      throw err;
    }
  },

  /**
   * Fetch payout history for inviter user.
   */
  async fetchPayoutHistory(inviterId, page = 1, pageSize = 20) {
    try {
      const res = await API.get(`${BASE}/${inviterId}/inviter-reward-payouts`, {
        params: { page, page_size: pageSize },
      });
      if (res.data.success) return res.data.data;
      throw new Error(res.data.message || 'Failed to fetch payout history');
    } catch (err) {
      showError(err.message || 'Failed to fetch payout history');
      throw err;
    }
  },

  /**
   * Create a new payout batch (mark current pending recharges as rewarded).
   * @param {number} inviterId
   * @param {{payout_amount_usd: number, note?: string}} body
   */
  async createPayout(inviterId, body) {
    try {
      const res = await API.post(`${BASE}/${inviterId}/inviter-reward-payouts`, body);
      if (res.data.success) return res.data.data;
      throw new Error(res.data.message || 'Failed to create payout');
    } catch (err) {
      showError(err.message || 'Failed to create payout');
      throw err;
    }
  },
};

export default InviterRewardAPI;
```

- [ ] **Step 2: 提交**

```bash
git add web/src/services/inviterRewardApi.js
git commit -m "feat(inviter-reward): add InviterRewardAPI service client"
```

---

### Task 8: 实现 PayoutInviterRewardModal 弹窗

**Files:**
- Create: `web/src/components/table/users/modals/PayoutInviterRewardModal.jsx`

- [ ] **Step 1: 写组件**

Create `web/src/components/table/users/modals/PayoutInviterRewardModal.jsx`:

```jsx
/*
Copyright (C) 2025 QuantumNous

Modal for admin to record an offline reward payout.
*/

import React, { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Modal,
  Form,
  InputNumber,
  Input,
  Typography,
  Space,
} from '@douyinfe/semi-ui';
import { InviterRewardAPI } from '../../../../services/inviterRewardApi';
import { showSuccess } from '../../../../helpers';
import { formatUSDAmount } from '../../../../utils/currency';

const { Text } = Typography;

const PayoutInviterRewardModal = ({
  visible,
  inviterId,
  pendingTotalUsd = 0,
  defaultPercent = 10,
  onClose,
  onSuccess,
}) => {
  const { t } = useTranslation();
  const [submitting, setSubmitting] = useState(false);
  const [actualAmount, setActualAmount] = useState(0);
  const [note, setNote] = useState('');

  const suggested = useMemo(() => {
    const v = (Number(pendingTotalUsd) || 0) * (Number(defaultPercent) || 0) / 100;
    return Math.round(v * 100) / 100;
  }, [pendingTotalUsd, defaultPercent]);

  useEffect(() => {
    if (visible) {
      setActualAmount(suggested);
      setNote('');
    }
  }, [visible, suggested]);

  const canSubmit = Number(actualAmount) > 0 && Number(pendingTotalUsd) > 0 && !submitting;

  const handleSubmit = async () => {
    if (!canSubmit) return;
    setSubmitting(true);
    try {
      const created = await InviterRewardAPI.createPayout(inviterId, {
        payout_amount_usd: Number(actualAmount),
        note,
      });
      showSuccess(
        t('已发放 {{amount}}，覆盖充值 {{recharge}}', {
          amount: formatUSDAmount(created.payout_amount_usd),
          recharge: formatUSDAmount(created.recharge_total_usd),
        })
      );
      onSuccess && onSuccess(created);
      onClose && onClose();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      title={t('发放邀请激励')}
      visible={visible}
      onCancel={onClose}
      onOk={handleSubmit}
      okButtonProps={{ disabled: !canSubmit, loading: submitting }}
      cancelText={t('取消')}
      okText={t('确认发放')}
      width={460}
    >
      <Space vertical style={{ width: '100%' }} spacing="loose">
        <div>
          <Text type="tertiary">{t('待激励充值总额')}</Text>
          <div>
            <Text strong style={{ fontSize: 18 }}>
              {formatUSDAmount(pendingTotalUsd)}
            </Text>
          </div>
        </div>
        <div>
          <Text type="tertiary">
            {t('系统默认比例')}：{defaultPercent}%　
            {t('建议奖励金额')}：{formatUSDAmount(suggested)}
          </Text>
        </div>
        <Form labelPosition="top">
          <Form.Slot label={t('实际奖励金额')}>
            <InputNumber
              value={actualAmount}
              min={0}
              step={0.01}
              precision={2}
              prefix="$"
              style={{ width: '100%' }}
              onChange={setActualAmount}
            />
          </Form.Slot>
          <Form.Slot label={t('备注（可选）')}>
            <Input
              value={note}
              onChange={setNote}
              placeholder={t('如：微信转账 流水号 xxx')}
              maxLength={500}
            />
          </Form.Slot>
        </Form>
      </Space>
    </Modal>
  );
};

export default PayoutInviterRewardModal;
```

- [ ] **Step 2: 提交**

```bash
git add web/src/components/table/users/modals/PayoutInviterRewardModal.jsx
git commit -m "feat(inviter-reward): add PayoutInviterRewardModal"
```

---

### Task 9: 实现 InviteeRechargesTab Tab 主体

**Files:**
- Create: `web/src/components/table/users/modals/InviteeRechargesTab.jsx`

- [ ] **Step 1: 写组件**

Create `web/src/components/table/users/modals/InviteeRechargesTab.jsx`:

```jsx
/*
Copyright (C) 2025 QuantumNous

Tab inside UserDetailModal showing the user's invitee recharge stats and payout history.
*/

import React, { useEffect, useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Row,
  Col,
  Table,
  Button,
  Empty,
  Space,
  Typography,
  Tag,
  Tooltip,
} from '@douyinfe/semi-ui';
import { InviterRewardAPI } from '../../../../services/inviterRewardApi';
import { formatUSDAmount } from '../../../../utils/currency';
import { timestamp2string } from '../../../../helpers';
import PayoutInviterRewardModal from './PayoutInviterRewardModal';

const { Text, Title } = Typography;

const KpiCard = ({ title, value, color }) => (
  <Card bodyStyle={{ padding: 16, textAlign: 'center' }}>
    <Text type="tertiary" style={{ fontSize: 12 }}>{title}</Text>
    <div style={{ marginTop: 8 }}>
      <Text strong style={{ fontSize: 20, color }}>{value}</Text>
    </div>
  </Card>
);

const InviteeRechargesTab = ({ visible, inviterId }) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [summary, setSummary] = useState({
    invitee_count: 0,
    recharge_total_usd: 0,
    payout_total_usd: 0,
    pending_total_usd: 0,
  });
  const [defaultPercent, setDefaultPercent] = useState(10);
  const [items, setItems] = useState([]);
  const [pagination, setPagination] = useState({ currentPage: 1, pageSize: 10, total: 0 });
  const [history, setHistory] = useState([]);
  const [historyPagination, setHistoryPagination] = useState({ currentPage: 1, pageSize: 10, total: 0 });
  const [payoutModalVisible, setPayoutModalVisible] = useState(false);

  const fetchDetail = useCallback(async (page = 1, pageSize = 10) => {
    if (!inviterId) return;
    setLoading(true);
    try {
      const res = await InviterRewardAPI.fetchInviteeRecharges(inviterId, page, pageSize);
      setSummary(res.summary || {});
      setItems(res.items || []);
      setDefaultPercent(res.default_percent ?? 10);
      setPagination({
        currentPage: res.pagination?.page || page,
        pageSize: res.pagination?.page_size || pageSize,
        total: res.pagination?.total || 0,
      });
    } finally {
      setLoading(false);
    }
  }, [inviterId]);

  const fetchHistory = useCallback(async (page = 1, pageSize = 10) => {
    if (!inviterId) return;
    setHistoryLoading(true);
    try {
      const res = await InviterRewardAPI.fetchPayoutHistory(inviterId, page, pageSize);
      setHistory(res.items || []);
      setHistoryPagination({
        currentPage: res.pagination?.page || page,
        pageSize: res.pagination?.page_size || pageSize,
        total: res.pagination?.total || 0,
      });
    } finally {
      setHistoryLoading(false);
    }
  }, [inviterId]);

  useEffect(() => {
    if (visible && inviterId) {
      fetchDetail(1, pagination.pageSize);
      fetchHistory(1, historyPagination.pageSize);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, inviterId]);

  const detailColumns = [
    { title: t('被邀请人'), dataIndex: 'invitee_username' },
    {
      title: t('完成时间'),
      dataIndex: 'complete_time',
      render: (v) => {
        const n = Number(v);
        if (!n) return '-';
        const sec = n > 1e12 ? Math.floor(n / 1000) : Math.floor(n);
        return timestamp2string(sec);
      },
    },
    { title: t('金额'), dataIndex: 'money_usd', render: (v) => formatUSDAmount(v) },
    { title: t('支付方式'), dataIndex: 'payment_method' },
    { title: t('订单号'), dataIndex: 'trade_no' },
    {
      title: t('激励状态'),
      dataIndex: 'payout_id',
      render: (v) =>
        v && v > 0
          ? <Tag color="green" size="small">{t('批次')} #{v}</Tag>
          : <Tag color="orange" size="small">{t('待激励')}</Tag>,
    },
  ];

  const historyColumns = [
    { title: t('批次'), dataIndex: 'id', render: (v) => `#${v}` },
    { title: t('发放金额'), dataIndex: 'payout_amount_usd', render: (v) => formatUSDAmount(v) },
    { title: t('涉及充值'), dataIndex: 'recharge_total_usd', render: (v) => formatUSDAmount(v) },
    { title: t('备注'), dataIndex: 'note', render: (v) => v || '-' },
    { title: t('操作管理员'), dataIndex: 'operator_admin_id', render: (v) => `#${v}` },
    {
      title: t('时间'),
      dataIndex: 'created_at',
      render: (v) => {
        const n = Number(v);
        if (!n) return '-';
        const sec = n > 1e12 ? Math.floor(n / 1000) : Math.floor(n);
        return timestamp2string(sec);
      },
    },
  ];

  const noPending = !(Number(summary.pending_total_usd) > 0);

  return (
    <Space vertical style={{ width: '100%' }} size="large">
      <Row gutter={16}>
        <Col span={6}>
          <KpiCard title={t('累计邀请人数')} value={summary.invitee_count || 0} color="#1890ff" />
        </Col>
        <Col span={6}>
          <KpiCard title={t('下级累计充值')} value={formatUSDAmount(summary.recharge_total_usd || 0)} color="#52c41a" />
        </Col>
        <Col span={6}>
          <KpiCard title={t('已发放奖励')} value={formatUSDAmount(summary.payout_total_usd || 0)} color="#722ed1" />
        </Col>
        <Col span={6}>
          <KpiCard title={t('待激励充值')} value={formatUSDAmount(summary.pending_total_usd || 0)} color="#faad14" />
        </Col>
      </Row>

      <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <Tooltip content={noPending ? t('暂无待激励充值') : ''}>
          <Button
            type="primary"
            disabled={noPending}
            onClick={() => setPayoutModalVisible(true)}
          >
            {t('发放激励')}
          </Button>
        </Tooltip>
      </div>

      <Card>
        <Title heading={5} style={{ marginBottom: 12 }}>{t('邀请下级充值明细')}</Title>
        <Table
          columns={detailColumns}
          dataSource={items}
          loading={loading}
          rowKey="topup_id"
          size="small"
          pagination={{
            currentPage: pagination.currentPage,
            pageSize: pagination.pageSize,
            total: pagination.total,
            showSizeChanger: true,
            pageSizeOpts: [10, 20, 50, 100],
            onPageChange: (p, ps) => fetchDetail(p, ps || pagination.pageSize),
            onPageSizeChange: (ps) => fetchDetail(1, ps),
          }}
          empty={<Empty description={t('暂无下级充值记录')} />}
          scroll={{ x: 900 }}
        />
      </Card>

      <Card>
        <Title heading={5} style={{ marginBottom: 12 }}>{t('激励发放历史')}</Title>
        <Table
          columns={historyColumns}
          dataSource={history}
          loading={historyLoading}
          rowKey="id"
          size="small"
          pagination={{
            currentPage: historyPagination.currentPage,
            pageSize: historyPagination.pageSize,
            total: historyPagination.total,
            showSizeChanger: true,
            pageSizeOpts: [10, 20, 50, 100],
            onPageChange: (p, ps) => fetchHistory(p, ps || historyPagination.pageSize),
            onPageSizeChange: (ps) => fetchHistory(1, ps),
          }}
          empty={<Empty description={t('暂无激励发放记录')} />}
          scroll={{ x: 800 }}
        />
      </Card>

      <PayoutInviterRewardModal
        visible={payoutModalVisible}
        inviterId={inviterId}
        pendingTotalUsd={summary.pending_total_usd || 0}
        defaultPercent={defaultPercent}
        onClose={() => setPayoutModalVisible(false)}
        onSuccess={() => {
          fetchDetail(1, pagination.pageSize);
          fetchHistory(1, historyPagination.pageSize);
        }}
      />
    </Space>
  );
};

export default InviteeRechargesTab;
```

- [ ] **Step 2: 提交**

```bash
git add web/src/components/table/users/modals/InviteeRechargesTab.jsx
git commit -m "feat(inviter-reward): add InviteeRechargesTab with KPIs, detail/history tables, payout button"
```

---

### Task 10: 把新 Tab 嵌入 UserDetailModal

**Files:**
- Modify: `web/src/components/table/users/modals/UserDetailModal.jsx`

- [ ] **Step 1: 添加 import**

Edit `web/src/components/table/users/modals/UserDetailModal.jsx`. 在文件顶部 import 区追加（约第 41-43 行附近）：

```jsx
import { AnalyticsAPI } from '../../../../services/analyticsApi';
import { formatUSDAmount } from '../../../../utils/currency';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import { timestamp2string } from '../../../../helpers';
import InviteeRechargesTab from './InviteeRechargesTab';
```

- [ ] **Step 2: 在 Tabs 末尾追加新 TabPane**

Edit `UserDetailModal.jsx`. 找到 `<TabPane tab={t('消费记录')} itemKey="records">` 这一段（约第 831 行），在它对应 `</TabPane>` 之后追加：

```jsx
              </TabPane>

              <TabPane
                tab={t('邀请充值')}
                itemKey="inviteeRecharges"
              >
                <InviteeRechargesTab
                  visible={visible && activeTab === 'inviteeRecharges'}
                  inviterId={user?.id}
                />
              </TabPane>
            </Tabs>
```

注意：原来的 `</Tabs>` 闭合标签位置不变，新 `<TabPane>` 插入在它之前。

- [ ] **Step 3: 启动前端 dev server，肉眼验证**

Run（在另一个终端）：

```bash
cd /usr/src/workspace/github/QQhuxuhui/new-api && go run main.go &
cd web && npm install && npm run dev
```

打开浏览器访问 `http://localhost:5173`（或 vite 提示的端口），用 root 登录 → 用户管理页 → 点任意用户 → 切到"邀请充值"Tab → 应能看到 4 个 KPI 卡片 + 两个空表格 + "发放激励"按钮（disabled，因为没有下级充值数据）。

- [ ] **Step 4: 提交**

```bash
git add web/src/components/table/users/modals/UserDetailModal.jsx
git commit -m "feat(inviter-reward): expose InviteeRechargesTab inside UserDetailModal"
```

---

### Task 11: 添加 i18n 英文翻译

**Files:**
- Modify: `web/src/i18n/locales/en.json`

- [ ] **Step 1: 找到 en.json 中合适位置（按照已有"邀请..."字符串的临近行）追加**

Edit `web/src/i18n/locales/en.json`. 在文件末尾的 `}` 之前追加新条目（注意最后一条已有项要补 `,`）。完整新增条目：

```json
  "邀请充值": "Invitee Recharges",
  "累计邀请人数": "Total Invitees",
  "下级累计充值": "Total Invitee Recharge",
  "已发放奖励": "Paid Out",
  "待激励充值": "Pending Recharge",
  "发放激励": "Pay Out Reward",
  "邀请下级充值明细": "Invitee Recharge Details",
  "激励发放历史": "Payout History",
  "被邀请人": "Invitee",
  "激励状态": "Reward Status",
  "批次": "Batch",
  "待激励": "Pending",
  "发放金额": "Payout Amount",
  "涉及充值": "Recharge Covered",
  "操作管理员": "Operator Admin",
  "暂无下级充值记录": "No invitee recharge records",
  "暂无激励发放记录": "No payout records",
  "暂无待激励充值": "No pending recharges",
  "发放邀请激励": "Pay Out Invitee Reward",
  "待激励充值总额": "Pending Recharge Total",
  "系统默认比例": "System Default %",
  "建议奖励金额": "Suggested Reward",
  "实际奖励金额": "Actual Reward",
  "备注（可选）": "Note (optional)",
  "如：微信转账 流水号 xxx": "e.g. WeChat transfer ref xxx",
  "确认发放": "Confirm Payout",
  "已发放 {{amount}}，覆盖充值 {{recharge}}": "Paid out {{amount}}, covering {{recharge}} of recharges"
```

- [ ] **Step 2: 验证 JSON 合法**

Run: `python3 -c "import json,sys; json.load(open('web/src/i18n/locales/en.json'))" && echo OK`

Expected: `OK`

- [ ] **Step 3: 提交**

```bash
git add web/src/i18n/locales/en.json
git commit -m "i18n(inviter-reward): add EN translations for invitee recharge tab"
```

---

## Phase 5 — 端到端验证

### Task 12: 端到端手动烟雾测试

**Files:** 无修改，仅验证

- [ ] **Step 1: 准备数据**

打开 sqlite/MySQL，造一个 inviter + 至少 1 个 invitee（`users.inviter_id = inviter.id`），并对 invitee 写 2-3 条 status='success' 的 top_ups 记录（不同 money）。

也可以走 UI：
1. 用一个测试账号注册（A），生成它的 aff code
2. 用第二个账号通过 A 的 aff code 注册（B）
3. 在 admin 后台给 B 走"管理员补单"操作，造 1-2 笔 success 充值

- [ ] **Step 2: 验证 GET 明细**

打开 admin Web → 用户管理 → 点用户 A → 切到"邀请充值"Tab。

期望：
- "累计邀请人数" = 1
- "下级累计充值" = B 的 success 充值汇总
- "已发放奖励" = $0
- "待激励充值" = 同"下级累计充值"
- 明细表显示 B 的 1-2 行充值，激励状态都是"待激励"
- "发放激励"按钮可点

- [ ] **Step 3: 验证 POST 发放**

点"发放激励"，弹窗显示 "待激励充值总额 = $X / 默认比例 10% / 建议奖励 = $X*0.1"，编辑实际金额，点确认。

期望：
- Toast 显示发放成功
- 弹窗关闭，KPI 刷新："已发放奖励" 增加，"待激励充值" 变 0
- 明细表所有行变成"批次 #1"
- "激励发放历史"出现一行新批次
- "发放激励"按钮变 disabled

- [ ] **Step 4: 验证幂等 / 422**

直接用 curl 再 POST 一次：

```bash
curl -X POST http://localhost:3000/api/user/manage/<inviterId>/inviter-reward-payouts \
  -H 'Content-Type: application/json' -H 'Cookie: <admin session>' \
  -d '{"payout_amount_usd":1}'
```

Expected: `{"success":false,"message":"暂无待激励充值"...}`

- [ ] **Step 5: 验证重绑邀请人后行为**

用 SetInviterModal 把 invitee B 从 inviter A 重绑给 inviter C。

期望：
- 进入 A 的"邀请充值"Tab：累计邀请 0 / 待激励 $0 / 已发放 $X（历史保留）
- 进入 C 的"邀请充值"Tab：累计邀请 1 / 累计充值 = B 的总充值（含已发放给 A 的部分）/ 待激励 = 0（因 B 的充值已被 A 的批次覆盖，payout_id != 0）
- 现在给 B 再造一笔新的 success 充值 → 切回 C 的 Tab → "待激励"应显示新这笔的金额

- [ ] **Step 6: 总结确认 + 不需要 commit**

无修改，无需提交。

---

## 已知留档（非阻塞）

- **复合索引**：spec 给出的 `idx_top_ups_user_status_payout(user_id, status, inviter_reward_payout_id)` 在本计划中只通过 GORM 的 `gorm:"index"` 标签建了单列索引。功能正确性不受影响。如果上线后发现"邀请充值"Tab 加载慢，再手动 `CREATE INDEX` 优化。
- **并发测试**：`CreateInviterRewardPayout` 的事务正确性靠"先调用一次成功 → 第二次必返回 ErrNoPendingRecharges"这个顺序测试覆盖。SQLite in-memory 不支持真 `FOR UPDATE`，所以 unit test 层做不了真并发。MySQL/Postgres 上的真并发请在集成环境验证（spec 验收标准 #4）。

## Self-Review 备忘

完成全部任务后，对照 `docs/superpowers/specs/2026-05-07-inviter-recharge-rewards-design.md` 自检：

- [ ] 4 个 KPI（invitee_count、recharge_total、payout_total、pending_total）都在 GET 接口返回，且前端展示
- [ ] 明细表 + 发放历史 + 发放弹窗三处 UI 全部就位
- [ ] POST 走事务 FOR UPDATE 防并发
- [ ] payout_amount <= 0 / 无 pending 都返回 422 风格的 success=false 响应
- [ ] 重绑邀请人语义符合"已被覆盖的不再算 / 未被覆盖的归新邀请人"
- [ ] 写一条 LogTypeManage 审计日志
- [ ] `User.Quota` / `User.AffQuota` / `User.AffHistoryQuota` 完全没动
- [ ] 没有添加 payout 撤销 / 编辑接口（out of scope）
