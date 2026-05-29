package model

import (
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

const (
	ErrorCaptureBodyMaxBytes = 64 * 1024
	errorCaptureTruncateMark = "...[truncated]"
)

// ErrorCaptureLog 命中关键词规则时捕获的完整请求记录，存于 LOG_DB
type ErrorCaptureLog struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	RuleId      string `json:"rule_id" gorm:"index:idx_ecl_rule_id_id,priority:1"`
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
			common.SysError("failed to record error capture log: " + err.Error())
			continue
		}
		if err := trimErrorCaptureRule(tgt.RuleId, tgt.MaxRecords); err != nil {
			common.SysError("failed to trim error capture rule " + tgt.RuleId + ": " + err.Error())
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
	if pageSize > 200 {
		pageSize = 200
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
