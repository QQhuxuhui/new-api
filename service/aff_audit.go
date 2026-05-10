package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"gorm.io/gorm"
)

// CreateAffAuditLogIfEligible 在支付成功后,根据 invitee 的邀请关系和反作弊规则,
// 决定是否在 aff_audit_logs 中创建一行记录。
//
// 行为:
//   - invitee 无邀请人(InviterId=0)→ 不写入,返回 (false, nil)
//   - 反作弊命中(同 IP / 同支付账号 / 邀请人冻结)→ 写入 status='rejected'
//   - 通过 → 写入 status='pending',eligible_at = paidAtMs + cooldown
//   - (source_type, source_id) 唯一索引冲突 → silent skip,返回 (false, nil)
//
// USD 直接使用 amountNative;CNY 按当前 priceRatio 换算后冻结到字段 PriceRatioUsed。
//
// 调用方:三个支付成功 hook(controller/topup.go / topup_order.go / plan_order.go)。
// 参数:
//   - inviteeUserId:被邀请人 ID(service 内部 fetch User,确保拿到最新 InviterId)
//   - sourceType / sourceId:充值流水定位(用于唯一索引)
//   - amountNative:原币金额(USD 或 CNY)
//   - currency:model.AffAuditCurrencyUsd / AffAuditCurrencyCny
//   - paidAtMs:支付完成时间戳(毫秒),用于计算 eligible_at
//
// 返回 (created, err);created=true 表示插入了一行(无论 pending / rejected)。
func CreateAffAuditLogIfEligible(inviteeUserId int, sourceType string, sourceId int, amountNative float64, currency string, paidAtMs int64) (bool, error) {
	if inviteeUserId == 0 {
		return false, nil
	}

	// 0. fetch invitee
	var invitee model.User
	if err := model.DB.Select("id, inviter_id").First(&invitee, inviteeUserId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if invitee.InviterId == 0 {
		return false, nil
	}

	// 1. fresh-read 邀请人(并发安全:写入瞬间读最新 aff_status)
	var inviter model.User
	if err := model.DB.Select("id, aff_status").First(&inviter, invitee.InviterId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil // 邀请人已不存在,跳过
		}
		return false, err
	}

	// 2. 计算金额:USD 直接用,CNY 按当前 priceRatio 换算并冻结
	var (
		amountUsd      float64
		priceRatioUsed float64
	)
	switch currency {
	case model.AffAuditCurrencyUsd:
		amountUsd = amountNative
		priceRatioUsed = 0
	case model.AffAuditCurrencyCny:
		ratio := operation_setting.Price
		if ratio <= 0 {
			ratio = 7.0 // safety fallback
		}
		amountUsd = amountNative / ratio
		priceRatioUsed = ratio
	default:
		return false, fmt.Errorf("unsupported currency: %q", currency)
	}
	rewardUsd := amountUsd * common.InviterRewardDefaultPercent / 100

	// 3. 反作弊预检
	rejectReason := ""
	if inviter.AffStatus == 1 {
		rejectReason = model.AffAuditRejectInviterFrozen
	} else if shared, err := model.UsersShareLoginIpRecently(inviter.Id, invitee.Id, 24); err != nil {
		return false, err
	} else if shared {
		rejectReason = model.AffAuditRejectSameIp
	} else if shared, err := model.UsersSharePaymentAccount(inviter.Id, invitee.Id); err != nil {
		return false, err
	} else if shared {
		rejectReason = model.AffAuditRejectSamePaymentAccount
	}

	// 4. 构造并插入 audit log
	status := model.AffAuditStatusPending
	if rejectReason != "" {
		status = model.AffAuditStatusRejected
	}
	cooldownMs := int64(common.InviterRewardCooldownDays) * 24 * 60 * 60 * 1000

	row := &model.AffAuditLog{
		InviterUserId:  inviter.Id,
		InviteeUserId:  invitee.Id,
		SourceType:     sourceType,
		SourceId:       sourceId,
		AmountNative:   amountNative,
		Currency:       currency,
		AmountUsd:      amountUsd,
		PriceRatioUsed: priceRatioUsed,
		RewardUsd:      rewardUsd,
		Status:         status,
		RejectReason:   rejectReason,
		EligibleAt:     paidAtMs + cooldownMs,
	}

	err := model.DB.Create(row).Error
	if err != nil {
		// 唯一索引冲突 → silent skip
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "unique") || strings.Contains(errMsg, "constraint") {
			common.SysLog(fmt.Sprintf("aff_audit_log duplicate write skipped: source=%s id=%d", sourceType, sourceId))
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// MarkRefunded 在退款回调中标记某笔充值对应的 audit log 为 refunded。
//   - status='pending' → 改为 refunded(撤销,无副作用)
//   - status='settled' → 改为 refunded(已发放的额度不自动扣减,留给管理员人工处理)
//   - status 其他(已 rejected / 已 offline_paid / 已 refunded)→ 不改
//   - 找不到对应 log → 静默成功(可能是退款的非邀请用户充值)
//
// v1 不接入任何 controller(项目当前无退款 webhook),作为 hook 等待未来对接。
func MarkRefunded(sourceType string, sourceId int) error {
	res := model.DB.Model(&model.AffAuditLog{}).
		Where("source_type = ? AND source_id = ? AND status IN ?",
			sourceType, sourceId,
			[]string{model.AffAuditStatusPending, model.AffAuditStatusSettled}).
		Update("status", model.AffAuditStatusRefunded)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		common.SysLog(fmt.Sprintf("aff_audit_log marked refunded: source=%s id=%d (status was pending/settled)", sourceType, sourceId))
	}
	return nil
}

