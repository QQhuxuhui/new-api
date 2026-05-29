# 错误抓取（Error Capture）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让超级管理员配置「错误关键词规则」，当上游报错日志详情命中关键词时，额外完整记录该次请求的请求体与元数据，每条规则只保留最近 N 条，并能在独立管理页查看完整请求体。

**Architecture:** 后端新增独立表 `ErrorCaptureLog`（落 `LOG_DB`）存储捕获记录；配置以 `error_capture_setting` 注册进现有 `GlobalConfig`，复用 `Option`/`UpdateOption` 持久化。在 `controller/relay.go` 重试循环 `processChannelError` 调用点之后挂一个钩子 `captureErrorRequestIfMatched`，命中且每请求一次时异步写库并按规则裁剪。前端新增超管可见的「错误抓取」菜单页（配置 + 查看记录 + 完整请求体抽屉）。

**Tech Stack:** Go + Gin + GORM（SQLite/MySQL/Postgres）；React + Semi UI（`@douyinfe/semi-ui`）；i18n（react-i18next）。

**Spec:** `docs/superpowers/specs/2026-05-29-error-capture-design.md`

---

## File Structure

后端：
- `setting/console_setting/error_capture.go`（新建）— 配置结构、注册、`ParsedRules`、`MatchErrorCaptureRules`、`NormalizeRulesJSON`（纯函数为主，可独立单测）
- `setting/console_setting/error_capture_test.go`（新建）— 匹配 / 归一化单测
- `constant/context_key.go`（修改）— 增 `ContextKeyErrorCaptureDone`
- `model/error_capture_log.go`（新建）— `ErrorCaptureLog` 模型、`truncateBody`、`RecordErrorCaptureLogs`、裁剪、查询
- `model/error_capture_log_test.go`（新建）— 截断 / 裁剪 / 查询单测
- `model/main.go`（修改）— `migrateLOGDB()` 注册新表
- `controller/relay.go`（修改）— 重试循环内挂钩 `captureErrorRequestIfMatched` + `buildCaptureTargets`
- `controller/option.go`（修改）— `UpdateOption` 增 `error_capture_setting.rules` 归一 case
- `controller/error_capture.go`（新建）— 查看记录的 3 个接口
- `router/api-router.go`（修改）— 注册 `/api/error_capture/logs*` 路由（RootAuth）

前端：
- `web/src/pages/ErrorCapture/index.jsx`（新建）— 页面
- `web/src/App.jsx`（修改）— 路由
- `web/src/components/layout/SiderBar.jsx`（修改）— 菜单项（仅超管）
- `web/src/i18n/locales/zh.json` / `en.json`（修改）— 文案

---

## Task 1: 配置结构与匹配/归一化纯函数

**Files:**
- Create: `setting/console_setting/error_capture.go`
- Test: `setting/console_setting/error_capture_test.go`

- [ ] **Step 1: 写失败测试**

Create `setting/console_setting/error_capture_test.go`:

```go
package console_setting

import "testing"

func TestMatchErrorCaptureRules(t *testing.T) {
	rules := []ErrorCaptureRule{
		{Id: "a", Keyword: "Rate limit", Enabled: true, MaxRecords: 100},
		{Id: "b", Keyword: "insufficient_quota", Enabled: true, MaxRecords: 50},
		{Id: "c", Keyword: "timeout", Enabled: false, MaxRecords: 100}, // disabled
		{Id: "d", Keyword: "", Enabled: true, MaxRecords: 100},          // empty -> skip
	}

	got := MatchErrorCaptureRules("Error: RATE LIMIT exceeded", rules) // 大小写不敏感
	if len(got) != 1 || got[0].Id != "a" {
		t.Fatalf("expected rule a, got %+v", got)
	}

	if m := MatchErrorCaptureRules("upstream timeout occurred", rules); len(m) != 0 {
		t.Fatalf("disabled rule must not match, got %+v", m)
	}

	if m := MatchErrorCaptureRules("anything", rules); len(m) != 0 {
		t.Fatalf("empty keyword must not match, got %+v", m)
	}
}

func TestNormalizeRulesJSON(t *testing.T) {
	n := 0
	genID := func() string { n++; return "gen" }
	in := `[
		{"keyword":"  spaces  ","label":"x","enabled":true,"max_records":0},
		{"id":"keep","keyword":"k","enabled":true,"max_records":99999},
		{"keyword":"   ","enabled":true,"max_records":10}
	]`
	out, err := NormalizeRulesJSON(in, genID)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	rules, err := parseRules(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rules) != 2 { // 空白关键词那条被丢弃
		t.Fatalf("expected 2 rules, got %d: %+v", len(rules), rules)
	}
	if rules[0].Id != "gen" || rules[0].Keyword != "spaces" || rules[0].MaxRecords != 100 {
		t.Fatalf("rule0 not normalized: %+v", rules[0])
	}
	if rules[1].Id != "keep" || rules[1].MaxRecords != 1000 { // 夹紧到上限
		t.Fatalf("rule1 not clamped: %+v", rules[1])
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./setting/console_setting/ -run 'TestMatchErrorCaptureRules|TestNormalizeRulesJSON' -v`
Expected: 编译失败（`ErrorCaptureRule` 等未定义）。

- [ ] **Step 3: 实现**

Create `setting/console_setting/error_capture.go`:

```go
package console_setting

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	errorCaptureMaxRecordsDefault = 100
	errorCaptureMaxRecordsMin     = 1
	errorCaptureMaxRecordsMax     = 1000
)

// ErrorCaptureRule 单条错误抓取规则
type ErrorCaptureRule struct {
	Id         string `json:"id"`
	Keyword    string `json:"keyword"`
	Label      string `json:"label"`
	Enabled    bool   `json:"enabled"`
	MaxRecords int    `json:"max_records"`
}

// ErrorCaptureSetting 错误抓取配置（持久化为 Option: error_capture_setting.*）
type ErrorCaptureSetting struct {
	Enabled bool   `json:"enabled"` // 总开关
	Rules   string `json:"rules"`   // JSON 数组字符串
}

var defaultErrorCaptureSetting = ErrorCaptureSetting{
	Enabled: false,
	Rules:   "",
}

var errorCaptureSetting = defaultErrorCaptureSetting

func init() {
	config.GlobalConfig.Register("error_capture_setting", &errorCaptureSetting)
}

// GetErrorCaptureSetting 返回全局配置实例
func GetErrorCaptureSetting() *ErrorCaptureSetting {
	return &errorCaptureSetting
}

// ParsedRules 解析 Rules JSON，非法返回空切片
func (s *ErrorCaptureSetting) ParsedRules() []ErrorCaptureRule {
	rules, err := parseRules(s.Rules)
	if err != nil {
		return nil
	}
	return rules
}

func parseRules(jsonStr string) ([]ErrorCaptureRule, error) {
	if strings.TrimSpace(jsonStr) == "" {
		return []ErrorCaptureRule{}, nil
	}
	var rules []ErrorCaptureRule
	if err := json.Unmarshal([]byte(jsonStr), &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// MatchErrorCaptureRules 返回命中的启用规则（大小写不敏感子串，空关键词跳过）
func MatchErrorCaptureRules(content string, rules []ErrorCaptureRule) []ErrorCaptureRule {
	lc := strings.ToLower(content)
	matched := make([]ErrorCaptureRule, 0, len(rules))
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		kw := strings.TrimSpace(r.Keyword)
		if kw == "" {
			continue
		}
		if strings.Contains(lc, strings.ToLower(kw)) {
			matched = append(matched, r)
		}
	}
	return matched
}

// NormalizeRulesJSON 解析、归一并序列化规则：生成缺失 id、夹紧 max_records、
// trim 关键词、丢弃空关键词规则。genID 用于生成新规则 id（便于测试注入）。
func NormalizeRulesJSON(jsonStr string, genID func() string) (string, error) {
	rules, err := parseRules(jsonStr)
	if err != nil {
		return "", err
	}
	out := make([]ErrorCaptureRule, 0, len(rules))
	for _, r := range rules {
		r.Keyword = strings.TrimSpace(r.Keyword)
		r.Label = strings.TrimSpace(r.Label)
		if r.Keyword == "" {
			continue
		}
		if strings.TrimSpace(r.Id) == "" {
			r.Id = genID()
		}
		if r.MaxRecords <= 0 {
			r.MaxRecords = errorCaptureMaxRecordsDefault
		}
		if r.MaxRecords < errorCaptureMaxRecordsMin {
			r.MaxRecords = errorCaptureMaxRecordsMin
		}
		if r.MaxRecords > errorCaptureMaxRecordsMax {
			r.MaxRecords = errorCaptureMaxRecordsMax
		}
		out = append(out, r)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./setting/console_setting/ -run 'TestMatchErrorCaptureRules|TestNormalizeRulesJSON' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add setting/console_setting/error_capture.go setting/console_setting/error_capture_test.go
git commit -m "feat(error-capture): add config struct, matcher and rule normalization"
```

---

## Task 2: 新增 context key 常量（每请求去重）

**Files:**
- Modify: `constant/context_key.go`（文件末尾常量块内，`ContextKeyReturnImmediately` 行之后）

- [ ] **Step 1: 修改**

在 `constant/context_key.go` 中 `ContextKeyReturnImmediately ContextKey = "return_immediately"` 这一行之后、闭合 `)` 之前，新增：

```go
	ContextKeyErrorCaptureDone ContextKey = "error_capture_done" // bool: 本请求已捕获过错误请求体，避免重试重复捕获
```

- [ ] **Step 2: 编译确认**

Run: `go build ./constant/...`
Expected: 无输出（成功）。

- [ ] **Step 3: 提交**

```bash
git add constant/context_key.go
git commit -m "feat(error-capture): add ContextKeyErrorCaptureDone for per-request dedup"
```

---

## Task 3: 捕获表模型与请求体截断

**Files:**
- Create: `model/error_capture_log.go`
- Test: `model/error_capture_log_test.go`

- [ ] **Step 1: 写失败测试（截断）**

Create `model/error_capture_log_test.go`:

```go
package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupErrorCaptureTestDB(t *testing.T) {
	t.Helper()
	dsn := fmt.Sprintf("file:error_capture_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	DB = db
	LOG_DB = db
	if err := db.AutoMigrate(&ErrorCaptureLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestTruncateBody(t *testing.T) {
	short := []byte("hello")
	if got := truncateBody(short); got != "hello" {
		t.Fatalf("short body changed: %q", got)
	}
	big := make([]byte, ErrorCaptureBodyMaxBytes+1000)
	for i := range big {
		big[i] = 'a'
	}
	got := truncateBody(big)
	if len(got) > ErrorCaptureBodyMaxBytes+len(errorCaptureTruncateMark)+4 {
		t.Fatalf("truncated body too large: %d", len(got))
	}
	if !strings.HasSuffix(got, errorCaptureTruncateMark) {
		t.Fatalf("missing truncate mark: ...%q", got[len(got)-20:])
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./model/ -run 'TestTruncateBody' -v`
Expected: 编译失败（`ErrorCaptureLog` / `truncateBody` 未定义）。

- [ ] **Step 3: 实现模型与截断**

Create `model/error_capture_log.go`:

```go
package model

import (
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"gorm.io/gorm"
)

const (
	ErrorCaptureBodyMaxBytes  = 64 * 1024
	errorCaptureTruncateMark  = "...[truncated]"
)

// ErrorCaptureLog 命中关键词规则时捕获的完整请求记录，存于 LOG_DB
type ErrorCaptureLog struct {
	Id          int    `json:"id"`
	RuleId      string `json:"rule_id" gorm:"index:idx_ecl_rule_id_id,priority:1;index"`
	Keyword     string `json:"keyword"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint;index:idx_ecl_rule_id_id,priority:2"`
	UserId      int    `json:"user_id" gorm:"index"`
	Username    string `json:"username"`
	ModelName   string `json:"model_name"`
	ChannelId   int    `json:"channel_id"`
	TokenName   string `json:"token_name"`
	StatusCode  int    `json:"status_code"`
	ErrorType   string `json:"error_type"`
	ErrorCode   string `json:"error_code"`
	RequestPath string `json:"request_path"`
	Content     string `json:"content"`
	RequestBody string `json:"request_body" gorm:"type:text"`
	Other       string `json:"other"`
}

// ErrorCaptureTarget 命中的规则目标（由 controller 从 console_setting 规则映射而来，
// 用以避免 model 反向依赖 setting 包）
type ErrorCaptureTarget struct {
	RuleId     string
	Keyword    string
	MaxRecords int
}

// ErrorCapturePayload 主协程取出的请求快照（异步写库时不再触碰 gin.Context）
type ErrorCapturePayload struct {
	CreatedAt   int64
	UserId      int
	Username    string
	ModelName   string
	ChannelId   int
	TokenName   string
	StatusCode  int
	ErrorType   string
	ErrorCode   string
	RequestPath string
	Content     string
	RequestBody []byte
	Other       string
}

func truncateBody(body []byte) string {
	if len(body) <= ErrorCaptureBodyMaxBytes {
		return string(body)
	}
	cut := body[:ErrorCaptureBodyMaxBytes]
	// 回退到合法 UTF-8 边界，避免截断半个字符
	for len(cut) > 0 && !utf8.Valid(cut) {
		cut = cut[:len(cut)-1]
	}
	return string(cut) + errorCaptureTruncateMark
}

// RecordErrorCaptureLogs 对每条命中规则各插入一行，并裁剪到该规则的 MaxRecords。
// 设计为在 gopool.Go 中异步调用。
func RecordErrorCaptureLogs(targets []ErrorCaptureTarget, p ErrorCapturePayload) {
	body := truncateBody(p.RequestBody)
	for _, tgt := range targets {
		row := &ErrorCaptureLog{
			RuleId:      tgt.RuleId,
			Keyword:     tgt.Keyword,
			CreatedAt:   p.CreatedAt,
			UserId:      p.UserId,
			Username:    p.Username,
			ModelName:   p.ModelName,
			ChannelId:   p.ChannelId,
			TokenName:   p.TokenName,
			StatusCode:  p.StatusCode,
			ErrorType:   p.ErrorType,
			ErrorCode:   p.ErrorCode,
			RequestPath: p.RequestPath,
			Content:     p.Content,
			RequestBody: body,
			Other:       p.Other,
		}
		if err := LOG_DB.Create(row).Error; err != nil {
			common.SysLog("failed to record error capture log: " + err.Error())
			continue
		}
		if err := trimErrorCaptureRule(tgt.RuleId, tgt.MaxRecords); err != nil {
			logger.SysError("failed to trim error capture rule " + tgt.RuleId + ": " + err.Error())
		}
	}
}

// trimErrorCaptureRule 仅保留某规则最新 maxRecords 条
func trimErrorCaptureRule(ruleId string, maxRecords int) error {
	if maxRecords <= 0 {
		maxRecords = 100
	}
	// 包一层子查询规避 MySQL "不能 DELETE 同时被子查询引用的表" 限制；SQLite/PG 同样兼容
	sub := LOG_DB.Model(&ErrorCaptureLog{}).
		Select("id").
		Where("rule_id = ?", ruleId).
		Order("id DESC").
		Limit(maxRecords)
	return LOG_DB.
		Where("rule_id = ?", ruleId).
		Where("id NOT IN (?)", LOG_DB.Table("(?) as t", sub).Select("id")).
		Delete(&ErrorCaptureLog{}).Error
}

// GetErrorCaptureLogs 按规则分页列出捕获记录摘要（不含 request_body）
func GetErrorCaptureLogs(ruleId string, page, pageSize int) (logs []*ErrorCaptureLog, total int64, err error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	q := LOG_DB.Model(&ErrorCaptureLog{}).Where("rule_id = ?", ruleId)
	if err = q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = q.Omit("request_body").
		Order("id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&logs).Error
	return logs, total, err
}

// GetErrorCaptureLogDetail 取单条完整记录（含 request_body）
func GetErrorCaptureLogDetail(id int) (*ErrorCaptureLog, error) {
	var row ErrorCaptureLog
	if err := LOG_DB.Where("id = ?", id).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// DeleteErrorCaptureLogsByRule 清空某规则下所有记录
func DeleteErrorCaptureLogsByRule(ruleId string) (int64, error) {
	res := LOG_DB.Where("rule_id = ?", ruleId).Delete(&ErrorCaptureLog{})
	return res.RowsAffected, res.Error
}

var _ = gorm.ErrRecordNotFound // 保留 gorm 引用占位（如未用到可删除该行）
```

> 注：若 `logger.SysError` 不存在，用 `common.SysLog` 代替；若 `gorm` 未被其它引用使用，删除最后的占位行与 import。实现时以 `go build` 报错为准修正。

- [ ] **Step 4: 运行确认通过**

Run: `go test ./model/ -run 'TestTruncateBody' -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add model/error_capture_log.go model/error_capture_log_test.go
git commit -m "feat(error-capture): add ErrorCaptureLog model, truncation and queries"
```

---

## Task 4: 裁剪与查询的行为测试

**Files:**
- Modify: `model/error_capture_log_test.go`

- [ ] **Step 1: 追加裁剪/查询测试**

在 `model/error_capture_log_test.go` 追加：

```go
func TestRecordAndTrimErrorCapture(t *testing.T) {
	setupErrorCaptureTestDB(t)

	targets := []ErrorCaptureTarget{{RuleId: "r1", Keyword: "k", MaxRecords: 3}}
	for i := 0; i < 5; i++ {
		RecordErrorCaptureLogs(targets, ErrorCapturePayload{
			CreatedAt:   int64(i),
			Content:     "boom",
			RequestBody: []byte(fmt.Sprintf("body-%d", i)),
		})
	}
	// 另一个规则不受影响
	RecordErrorCaptureLogs([]ErrorCaptureTarget{{RuleId: "r2", Keyword: "k", MaxRecords: 100}},
		ErrorCapturePayload{RequestBody: []byte("other")})

	logs, total, err := GetErrorCaptureLogs("r1", 1, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 || len(logs) != 3 {
		t.Fatalf("expected 3 kept, got total=%d len=%d", total, len(logs))
	}
	// 列表不返回 request_body
	if logs[0].RequestBody != "" {
		t.Fatalf("list should omit request_body, got %q", logs[0].RequestBody)
	}
	// 详情返回完整 body
	detail, err := GetErrorCaptureLogDetail(logs[0].Id)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.RequestBody == "" {
		t.Fatalf("detail must contain request_body")
	}

	if _, _, err := GetErrorCaptureLogs("r2", 1, 50); err != nil {
		t.Fatalf("r2 list: %v", err)
	}
	var r2Total int64
	GetErrorCaptureLogs("r2", 1, 50)
	if logs2, total2, _ := GetErrorCaptureLogs("r2", 1, 50); total2 != 1 || len(logs2) != 1 {
		r2Total = total2
		t.Fatalf("r2 should keep 1, got total=%d", r2Total)
	}

	if n, err := DeleteErrorCaptureLogsByRule("r1"); err != nil || n != 3 {
		t.Fatalf("delete r1 expected 3, got n=%d err=%v", n, err)
	}
}
```

- [ ] **Step 2: 运行确认通过**

Run: `go test ./model/ -run 'TestRecordAndTrimErrorCapture' -v`
Expected: PASS。（若 `id NOT IN (子查询)` 在 sqlite 上报错，将 `trimErrorCaptureRule` 改为先 `Pluck` 取要保留的 id 列表，再 `Where("rule_id = ? AND id NOT IN ?", ruleId, keepIds).Delete(...)`，重跑至 PASS。）

- [ ] **Step 3: 提交**

```bash
git add model/error_capture_log_test.go
git commit -m "test(error-capture): cover trim retention and list/detail queries"
```

---

## Task 5: 注册迁移

**Files:**
- Modify: `model/main.go`（`migrateLOGDB` 函数，约 line 526-532）

- [ ] **Step 1: 修改 migrateLOGDB**

将：

```go
func migrateLOGDB() error {
	var err error
	if err = LOG_DB.AutoMigrate(&Log{}); err != nil {
		return err
	}
	return nil
}
```

改为：

```go
func migrateLOGDB() error {
	var err error
	if err = LOG_DB.AutoMigrate(&Log{}, &ErrorCaptureLog{}); err != nil {
		return err
	}
	return nil
}
```

- [ ] **Step 2: 编译确认**

Run: `go build ./...`
Expected: 成功（无输出）。

- [ ] **Step 3: 提交**

```bash
git add model/main.go
git commit -m "feat(error-capture): register ErrorCaptureLog in LOG_DB migration"
```

---

## Task 6: 在 relay 重试循环挂捕获钩子

**Files:**
- Modify: `controller/relay.go`（重试循环内 `processChannelError(...)` 调用点，约 line 338；并在文件内新增两个辅助函数）

- [ ] **Step 1: 在调用点后挂钩**

在 `controller/relay.go` 约 line 338 的：

```go
				processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)
```

之后紧接着新增一行：

```go
				captureErrorRequestIfMatched(c, newAPIError)
```

- [ ] **Step 2: 新增辅助函数**

在 `controller/relay.go` 的 `processChannelError` 函数定义之后，新增：

```go
// captureErrorRequestIfMatched 当错误详情命中已启用的错误抓取规则时，
// 异步保存该次请求的完整请求体与元数据。每个请求最多捕获一次。
// 独立于全局 ErrorLogEnabled，仅受 error_capture_setting.Enabled 控制。
func captureErrorRequestIfMatched(c *gin.Context, err *types.NewAPIError) {
	if err == nil {
		return
	}
	s := console_setting.GetErrorCaptureSetting()
	if !s.Enabled || !types.IsRecordErrorLog(err) {
		return
	}
	if common.GetContextKeyBool(c, constant.ContextKeyErrorCaptureDone) {
		return
	}
	content := err.MaskSensitiveError()
	matched := console_setting.MatchErrorCaptureRules(content, s.ParsedRules())
	if len(matched) == 0 {
		return
	}
	common.SetContextKey(c, constant.ContextKeyErrorCaptureDone, true)

	targets := make([]model.ErrorCaptureTarget, 0, len(matched))
	for _, r := range matched {
		targets = append(targets, model.ErrorCaptureTarget{
			RuleId:     r.Id,
			Keyword:    r.Keyword,
			MaxRecords: r.MaxRecords,
		})
	}

	body, _ := common.GetRequestBody(c) // 已缓存，安全
	other := map[string]interface{}{
		"channel_name": c.GetString("channel_name"),
		"channel_type": c.GetInt("channel_type"),
	}
	payload := model.ErrorCapturePayload{
		CreatedAt:   common.GetTimestamp(),
		UserId:      c.GetInt("id"),
		Username:    c.GetString("username"),
		ModelName:   c.GetString("original_model"),
		ChannelId:   c.GetInt("channel_id"),
		TokenName:   c.GetString("token_name"),
		StatusCode:  err.StatusCode,
		ErrorType:   string(err.GetErrorType()),
		ErrorCode:   fmt.Sprintf("%v", err.GetErrorCode()),
		RequestPath: requestPathOf(c),
		Content:     content,
		RequestBody: body,
		Other:       common.MapToJsonStr(other),
	}
	gopool.Go(func() {
		model.RecordErrorCaptureLogs(targets, payload)
	})
}

func requestPathOf(c *gin.Context) string {
	if c.Request != nil && c.Request.URL != nil {
		return c.Request.URL.Path
	}
	return ""
}
```

- [ ] **Step 3: 补充 import**

确认 `controller/relay.go` 顶部 import 含：`"github.com/QuantumNous/new-api/setting/console_setting"`、`"github.com/QuantumNous/new-api/model"`、`"github.com/bytedance/gopkg/util/gopool"`、`"github.com/QuantumNous/new-api/common"`、`"github.com/QuantumNous/new-api/constant"`、`"github.com/QuantumNous/new-api/types"`、`"fmt"`。缺哪个补哪个（多数已存在）。

- [ ] **Step 4: 编译确认**

Run: `go build ./controller/... ./...`
Expected: 成功。若报 `gopool` 未引入或重复，按 `go build` 提示修正 import。

- [ ] **Step 5: 提交**

```bash
git add controller/relay.go
git commit -m "feat(error-capture): hook capture into relay retry loop with per-request dedup"
```

---

## Task 7: UpdateOption 归一 error_capture_setting.rules

**Files:**
- Modify: `controller/option.go`（`UpdateOption` 的 `switch option.Key` 内新增一个 case；确认顶部已 import `console_setting` 与 `common`）

- [ ] **Step 1: 新增 case**

在 `controller/option.go` 的 `switch option.Key {` 块内（与 `case "console_setting.api_info":` 同级），新增：

```go
	case "error_capture_setting.rules":
		normalized, nerr := console_setting.NormalizeRulesJSON(option.Value.(string), common.GetUUID)
		if nerr != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "错误抓取规则格式无效: " + nerr.Error(),
			})
			return
		}
		option.Value = normalized // 写回归一后的 JSON（已生成 id、夹紧 max_records）
```

- [ ] **Step 2: 编译确认**

Run: `go build ./controller/...`
Expected: 成功。（`controller/option.go` 顶部已 import `setting/console_setting` —— 见 line 12；`common` 亦已 import。）

- [ ] **Step 3: 提交**

```bash
git add controller/option.go
git commit -m "feat(error-capture): normalize rules JSON on option update (gen id, clamp)"
```

---

## Task 8: 查看记录接口与路由

**Files:**
- Create: `controller/error_capture.go`
- Modify: `router/api-router.go`（在 `logRoute` 注册块附近新增 `errorCaptureRoute`）

- [ ] **Step 1: 实现 controller**

Create `controller/error_capture.go`:

```go
package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetErrorCaptureLogs GET /api/error_capture/logs?rule_id=&p=&page_size=
func GetErrorCaptureLogs(c *gin.Context) {
	ruleId := c.Query("rule_id")
	if ruleId == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "缺少 rule_id"})
		return
	}
	page, _ := strconv.Atoi(c.Query("p"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}
	logs, total, err := model.GetErrorCaptureLogs(ruleId, page, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items": logs,
			"total": total,
			"page":  page,
		},
	})
}

// GetErrorCaptureLogDetail GET /api/error_capture/logs/:id
func GetErrorCaptureLogDetail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的 id"})
		return
	}
	row, err := model.GetErrorCaptureLogDetail(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// DeleteErrorCaptureLogs DELETE /api/error_capture/logs?rule_id=
func DeleteErrorCaptureLogs(c *gin.Context) {
	ruleId := c.Query("rule_id")
	if ruleId == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "缺少 rule_id"})
		return
	}
	n, err := model.DeleteErrorCaptureLogsByRule(ruleId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"deleted": n}})
}
```

- [ ] **Step 2: 注册路由**

在 `router/api-router.go` 的 `logRoute := apiRouter.Group("/log")` 注册块（约 line 272-280）之后，新增：

```go
		errorCaptureRoute := apiRouter.Group("/error_capture")
		errorCaptureRoute.Use(middleware.RootAuth())
		{
			errorCaptureRoute.GET("/logs", controller.GetErrorCaptureLogs)
			errorCaptureRoute.GET("/logs/:id", controller.GetErrorCaptureLogDetail)
			errorCaptureRoute.DELETE("/logs", controller.DeleteErrorCaptureLogs)
		}
```

- [ ] **Step 3: 编译确认**

Run: `go build ./...`
Expected: 成功。

- [ ] **Step 4: 提交**

```bash
git add controller/error_capture.go router/api-router.go
git commit -m "feat(error-capture): add root-only view/delete routes for capture logs"
```

---

## Task 9: 后端整体回归

**Files:** 无（验证）

- [ ] **Step 1: 全量测试 + 构建**

Run: `go build ./... && go test ./setting/console_setting/ ./model/ -run 'ErrorCapture|TruncateBody|RecordAndTrim|MatchError|NormalizeRules' -v`
Expected: 构建成功，相关测试全部 PASS。

- [ ] **Step 2: 无提交**（纯验证；若有修正则单独 commit）

---

## Task 10: 前端页面（配置 + 记录 + 完整请求体抽屉）

**Files:**
- Create: `web/src/pages/ErrorCapture/index.jsx`

参照现有 `web/src/pages/FailoverRules/index.jsx` 的页面骨架与 `API` 调用风格（`import { API, showError, showSuccess } from '../../helpers';`）。配置读写走 `GET /api/option/` 与 `PUT /api/option/`（key：`error_capture_setting.enabled` / `error_capture_setting.rules`）。

- [ ] **Step 1: 实现页面**

Create `web/src/pages/ErrorCapture/index.jsx`:

```jsx
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/
import React, { useEffect, useState } from 'react';
import {
  Button, Card, Table, Switch, Input, InputNumber, Space, Typography,
  Modal, Toast, Banner, Popconfirm, Select, Empty, Tag,
} from '@douyinfe/semi-ui';
import { IconPlus, IconDelete, IconRefresh } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';

const genLocalId = () =>
  'r_' + Date.now().toString(36) + Math.random().toString(36).slice(2, 8);

const ErrorCapture = () => {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState(false);
  const [rules, setRules] = useState([]);
  const [saving, setSaving] = useState(false);

  const [selectedRuleId, setSelectedRuleId] = useState('');
  const [logs, setLogs] = useState([]);
  const [logTotal, setLogTotal] = useState(0);
  const [logPage, setLogPage] = useState(1);
  const [detail, setDetail] = useState(null);

  const loadConfig = async () => {
    const res = await API.get('/api/option/');
    if (!res.data.success) {
      showError(res.data.message);
      return;
    }
    const opts = res.data.data || [];
    let en = false;
    let rs = [];
    opts.forEach((o) => {
      if (o.key === 'error_capture_setting.enabled') en = o.value === 'true';
      if (o.key === 'error_capture_setting.rules') {
        try { rs = JSON.parse(o.value || '[]'); } catch (e) { rs = []; }
      }
    });
    setEnabled(en);
    setRules(Array.isArray(rs) ? rs : []);
  };

  useEffect(() => { loadConfig(); }, []);

  const updateOption = async (key, value) => {
    const res = await API.put('/api/option/', { key, value });
    if (!res.data.success) throw new Error(res.data.message);
  };

  const saveAll = async () => {
    setSaving(true);
    try {
      await updateOption('error_capture_setting.enabled', enabled);
      await updateOption('error_capture_setting.rules', JSON.stringify(rules));
      showSuccess(t('保存成功'));
      await loadConfig(); // 拉回后端归一后的规则（含生成的 id）
    } catch (e) {
      showError(e.message);
    } finally {
      setSaving(false);
    }
  };

  const addRule = () => {
    setRules([...rules, { id: genLocalId(), keyword: '', label: '', enabled: true, max_records: 100 }]);
  };
  const updateRule = (idx, patch) => {
    const next = rules.slice();
    next[idx] = { ...next[idx], ...patch };
    setRules(next);
  };
  const removeRule = (idx) => {
    const next = rules.slice();
    next.splice(idx, 1);
    setRules(next);
  };

  const loadLogs = async (ruleId, page = 1) => {
    if (!ruleId) return;
    const res = await API.get(`/api/error_capture/logs?rule_id=${encodeURIComponent(ruleId)}&p=${page}&page_size=20`);
    if (!res.data.success) { showError(res.data.message); return; }
    setLogs(res.data.data.items || []);
    setLogTotal(res.data.data.total || 0);
    setLogPage(page);
  };

  const openDetail = async (id) => {
    const res = await API.get(`/api/error_capture/logs/${id}`);
    if (!res.data.success) { showError(res.data.message); return; }
    setDetail(res.data.data);
  };

  const clearRuleLogs = async (ruleId) => {
    const res = await API.delete(`/api/error_capture/logs?rule_id=${encodeURIComponent(ruleId)}`);
    if (!res.data.success) { showError(res.data.message); return; }
    showSuccess(t('已清空'));
    loadLogs(ruleId, 1);
  };

  const ruleColumns = [
    { title: t('备注'), dataIndex: 'label', render: (v, r, i) => (
      <Input value={v} placeholder={t('备注名')} onChange={(val) => updateRule(i, { label: val })} />
    )},
    { title: t('关键词（详情包含即匹配）'), dataIndex: 'keyword', render: (v, r, i) => (
      <Input value={v} placeholder={t('如 insufficient_quota')} onChange={(val) => updateRule(i, { keyword: val })} />
    )},
    { title: t('保留条数'), dataIndex: 'max_records', width: 120, render: (v, r, i) => (
      <InputNumber min={1} max={1000} value={v} onChange={(val) => updateRule(i, { max_records: val })} />
    )},
    { title: t('启用'), dataIndex: 'enabled', width: 90, render: (v, r, i) => (
      <Switch checked={v} onChange={(val) => updateRule(i, { enabled: val })} />
    )},
    { title: t('操作'), width: 80, render: (_, r, i) => (
      <Button icon={<IconDelete />} type='danger' theme='borderless' onClick={() => removeRule(i)} />
    )},
  ];

  const logColumns = [
    { title: t('时间'), dataIndex: 'created_at', width: 170,
      render: (v) => new Date(v * 1000).toLocaleString() },
    { title: t('用户'), dataIndex: 'username', width: 120 },
    { title: t('模型'), dataIndex: 'model_name', width: 160 },
    { title: t('状态码'), dataIndex: 'status_code', width: 90,
      render: (v) => <Tag color='red'>{v}</Tag> },
    { title: t('错误详情'), dataIndex: 'content', ellipsis: true },
    { title: t('操作'), width: 100,
      render: (_, r) => <Button theme='borderless' onClick={() => openDetail(r.id)}>{t('查看请求')}</Button> },
  ];

  const savedRules = rules.filter((r) => r.id);

  return (
    <div style={{ padding: 16 }}>
      <Card style={{ marginBottom: 16 }}>
        <Banner type='warning' description={t('捕获的请求体可能包含用户敏感数据，仅超级管理员可见。每个请求最多记录一次，每条规则只保留最近设定的条数。')} />
        <Space style={{ marginTop: 12, marginBottom: 12 }}>
          <Typography.Text strong>{t('总开关')}</Typography.Text>
          <Switch checked={enabled} onChange={setEnabled} />
          <Button icon={<IconPlus />} onClick={addRule}>{t('添加规则')}</Button>
          <Button theme='solid' loading={saving} onClick={saveAll}>{t('保存配置')}</Button>
        </Space>
        <Table columns={ruleColumns} dataSource={rules} pagination={false} rowKey={(r) => r.id || r.keyword} />
      </Card>

      <Card title={t('抓取记录')}>
        <Space style={{ marginBottom: 12 }}>
          <Select
            placeholder={t('选择规则查看记录')}
            style={{ width: 280 }}
            value={selectedRuleId || undefined}
            onChange={(v) => { setSelectedRuleId(v); loadLogs(v, 1); }}
            optionList={savedRules.map((r) => ({
              label: (r.label ? r.label + ' — ' : '') + r.keyword, value: r.id,
            }))}
          />
          <Button icon={<IconRefresh />} disabled={!selectedRuleId} onClick={() => loadLogs(selectedRuleId, logPage)}>{t('刷新')}</Button>
          {selectedRuleId && (
            <Popconfirm title={t('确认清空该规则下所有记录？')} onConfirm={() => clearRuleLogs(selectedRuleId)}>
              <Button type='danger'>{t('清空记录')}</Button>
            </Popconfirm>
          )}
        </Space>
        {selectedRuleId ? (
          <Table
            columns={logColumns}
            dataSource={logs}
            rowKey='id'
            pagination={{
              currentPage: logPage,
              pageSize: 20,
              total: logTotal,
              onPageChange: (p) => loadLogs(selectedRuleId, p),
            }}
          />
        ) : <Empty description={t('请选择规则')} />}
      </Card>

      <Modal
        title={t('完整请求数据')}
        visible={!!detail}
        onCancel={() => setDetail(null)}
        footer={null}
        width={760}
      >
        {detail && (
          <div>
            <Typography.Paragraph>
              <b>{t('路径')}:</b> {detail.request_path} &nbsp; <b>{t('渠道')}:</b> {detail.channel_id}
            </Typography.Paragraph>
            <Typography.Paragraph><b>{t('错误详情')}:</b> {detail.content}</Typography.Paragraph>
            <Typography.Text strong>{t('请求体')}:</Typography.Text>
            <pre style={{ maxHeight: 420, overflow: 'auto', background: 'var(--semi-color-fill-0)', padding: 12, borderRadius: 6 }}>
              {(() => {
                try { return JSON.stringify(JSON.parse(detail.request_body), null, 2); }
                catch (e) { return detail.request_body; }
              })()}
            </pre>
          </div>
        )}
      </Modal>
    </div>
  );
};

export default ErrorCapture;
```

- [ ] **Step 2: 前端构建确认**

Run: `cd web && npm run build 2>&1 | tail -20`（或项目惯用的 `bun run build` —— 见 `web/package.json` 的 scripts）
Expected: 构建成功，无该文件相关报错。若图标名不存在（如 `IconRefresh`），按 `@douyinfe/semi-icons` 实际导出名替换。

- [ ] **Step 3: 提交**

```bash
git add web/src/pages/ErrorCapture/index.jsx
git commit -m "feat(error-capture): add admin page for rules config and capture viewing"
```

---

## Task 11: 路由 + 菜单 + i18n

**Files:**
- Modify: `web/src/App.jsx`（import + Route）
- Modify: `web/src/components/layout/SiderBar.jsx`（菜单项 + 路径映射）
- Modify: `web/src/i18n/locales/zh.json`、`web/src/i18n/locales/en.json`

- [ ] **Step 1: App.jsx 增路由**

在 `web/src/App.jsx` 顶部 import 区（与 `import FailoverRules from './pages/FailoverRules';` 同区）新增：

```jsx
import ErrorCapture from './pages/ErrorCapture';
```

在 `/console/failover-rules` 的 `<Route>` 块（约 line 200-207）之后新增：

```jsx
        <Route
          path='/console/error-capture'
          element={
            <AdminRoute>
              <ErrorCapture />
            </AdminRoute>
          }
        />
```

> 说明：`AdminRoute` 仅做 role>=10 的路由级兜底；菜单项用 `isRoot()` 控制仅超管可见（下一步）。接口层已是 `RootAuth`，普通管理员即便手输 URL 也取不到数据。

- [ ] **Step 2: SiderBar 增菜单项 + 路径映射**

在 `web/src/components/layout/SiderBar.jsx` 约 line 37 的路径映射对象里，`'failover-rules': '/console/failover-rules',` 之后新增：

```js
  'error-capture': '/console/error-capture',
```

在 `adminItems` 数组里（约 line 178-183 的 failover-rules 项之后）新增：

```jsx
      {
        text: t('错误抓取'),
        itemKey: 'error-capture',
        to: '/console/error-capture',
        className: isRoot() ? '' : 'tableHiddle',
      },
```

确认 `SiderBar.jsx` 顶部已从 `../../helpers/utils` 或 `../../helpers` import `isRoot`；若只 import 了 `isAdmin`，把 `isRoot` 一并加入 import。

- [ ] **Step 3: i18n 文案**

在 `web/src/i18n/locales/zh.json` 增（键为中文原文 → 中文，保持现有约定；若该项目 zh 文件为恒等映射，按现有风格补齐键）：

```json
  "错误抓取": "错误抓取",
  "捕获的请求体可能包含用户敏感数据，仅超级管理员可见。每个请求最多记录一次，每条规则只保留最近设定的条数。": "捕获的请求体可能包含用户敏感数据，仅超级管理员可见。每个请求最多记录一次，每条规则只保留最近设定的条数。",
  "关键词（详情包含即匹配）": "关键词（详情包含即匹配）",
  "保留条数": "保留条数",
  "抓取记录": "抓取记录",
  "查看请求": "查看请求",
  "完整请求数据": "完整请求数据",
  "选择规则查看记录": "选择规则查看记录",
  "清空记录": "清空记录"
```

在 `web/src/i18n/locales/en.json` 增对应英文：

```json
  "错误抓取": "Error Capture",
  "捕获的请求体可能包含用户敏感数据，仅超级管理员可见。每个请求最多记录一次，每条规则只保留最近设定的条数。": "Captured request bodies may contain sensitive user data and are visible to root admins only. At most one record per request; each rule keeps only the configured most-recent count.",
  "关键词（详情包含即匹配）": "Keyword (matches if detail contains it)",
  "保留条数": "Keep count",
  "抓取记录": "Capture Records",
  "查看请求": "View Request",
  "完整请求数据": "Full Request Data",
  "选择规则查看记录": "Select a rule to view records",
  "清空记录": "Clear Records"
```

> 实现时先确认 `zh.json` / `en.json` 现有 key 风格（中文原文做 key 还是语义 key），按现有风格补齐；其余复用键（保存配置/添加规则/总开关/备注/启用/操作/时间/用户/模型/状态码/错误详情/路径/渠道/请求体/刷新/请选择规则/确认清空该规则下所有记录？/已清空/保存成功）若已存在则无需重复添加。

- [ ] **Step 4: 前端构建确认**

Run: `cd web && npm run build 2>&1 | tail -20`
Expected: 构建成功。

- [ ] **Step 5: 提交**

```bash
git add web/src/App.jsx web/src/components/layout/SiderBar.jsx web/src/i18n/locales/zh.json web/src/i18n/locales/en.json
git commit -m "feat(error-capture): wire route, root-only menu and i18n"
```

---

## Task 12: 端到端手测（验证，非自动化）

**Files:** 无

- [ ] **Step 1: 启动并验证**

1. 以超管登录，打开「错误抓取」菜单页（普通管理员看不到该菜单）。
2. 打开总开关，添加一条规则（如关键词 `model_not_found`，保留条数 5），保存；刷新后规则带上后端生成的 `id`。
3. 用一个会触发该错误的请求打 relay 接口（如请求不存在的模型），制造命中。
4. 回页面选中该规则 → 看到 1 条记录；点「查看请求」→ 弹窗显示完整请求体（JSON 美化）。
5. 连续触发 >5 次同类错误（多个不同请求），确认记录数稳定在 5 条（最新优先）。
6. 同一请求跨多渠道重试失败时，确认只新增 1 条（每请求去重）。
7. 关闭总开关后再触发，确认不再新增记录。

- [ ] **Step 2: 标记完成**（无提交）

---

## Self-Review 结果

- **Spec 覆盖**：触发范围(Task6)、匹配方式(Task1)、抓取内容+截断(Task3)、按条数过期(Task3/4)、独立表+迁移(Task3/5)、配置复用 Option(Task1/7)、查看接口 RootAuth(Task8)、独立页面(Task10/11)、每请求去重(Task2/6)、独立开关(Task6)、隐私 RootAuth(Task8/11) —— 均有对应任务。
- **占位符**：无 TBD / 模糊步骤，代码均给出。
- **类型一致**：`ErrorCaptureRule`/`ErrorCaptureSetting`/`ParsedRules`/`MatchErrorCaptureRules`/`NormalizeRulesJSON`（Task1）与 `ErrorCaptureLog`/`ErrorCaptureTarget`/`ErrorCapturePayload`/`RecordErrorCaptureLogs`/`truncateBody`/`trimErrorCaptureRule`/`GetErrorCaptureLogs`/`GetErrorCaptureLogDetail`/`DeleteErrorCaptureLogsByRule`（Task3）在 Task6/8 调用处签名一致；`ContextKeyErrorCaptureDone`（Task2）在 Task6 使用一致。
- **已知风险点**：`trimErrorCaptureRule` 的 `NOT IN (子查询)` 在不同 DB 行为差异，Task4 给了 sqlite fallback；`logger.SysError` / 图标名 / i18n key 风格三处以实际编译/构建为准微调，已在对应步骤标注。
