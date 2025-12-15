package model

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// TopupOrder represents a wallet topup order (pay-as-you-go)
type TopupOrder struct {
	Id      int    `json:"id" gorm:"primaryKey;autoIncrement"`
	OrderNo string `json:"order_no" gorm:"type:varchar(64);uniqueIndex;not null"` // Unique order number

	UserId int `json:"user_id" gorm:"not null;index"`

	// Topup amount info
	Amount       float64 `json:"amount" gorm:"type:decimal(10,2);not null"`        // Topup amount (quota units, e.g., USD)
	Quota        int64   `json:"quota" gorm:"not null"`                            // Quota to be added (internal units)
	OriginalPrice float64 `json:"original_price" gorm:"type:decimal(10,2);not null"` // Original price before discount
	FinalPrice   float64 `json:"final_price" gorm:"type:decimal(10,2);not null"`   // Final payment amount (after discount)
	DiscountRate float64 `json:"discount_rate" gorm:"type:decimal(5,4);default:1"` // Discount rate (e.g., 0.9 for 10% off)

	// Payment information
	PaymentMethod  string `json:"payment_method" gorm:"type:varchar(50)"`         // alipay, wechat, stripe, creem
	PaymentTradeNo string `json:"payment_trade_no" gorm:"type:varchar(255);index"` // Payment gateway transaction ID

	// Status management
	Status string `json:"status" gorm:"type:varchar(20);default:'pending';index"` // pending, paid, cancelled, expired

	// Timestamps (milliseconds)
	CreatedAt   int64 `json:"created_at" gorm:"index;not null"` // Order creation time
	ExpiredAt   int64 `json:"expired_at"`                       // Expiration time for pending orders
	PaidAt      int64 `json:"paid_at"`                          // Payment completion time
	CancelledAt int64 `json:"cancelled_at"`                     // Cancellation time

	// Associations
	User *User `json:"user,omitempty" gorm:"foreignKey:UserId"`
}

// Topup order status constants (reuse plan order constants)
const (
	TopupOrderStatusPending   = "pending"
	TopupOrderStatusPaid      = "paid"
	TopupOrderStatusExpired   = "expired"
	TopupOrderStatusCancelled = "cancelled"
)

// Topup order expiration time (30 minutes)
const TopupOrderExpirationMinutes = 30

func (to *TopupOrder) TableName() string {
	return "topup_orders"
}

// GenerateTopupOrderNo generates a unique order number for topup
// Format: TO{userId}NO{timestamp}{4-digit-random}
func GenerateTopupOrderNo(userId int) string {
	timestamp := time.Now().UnixMilli()

	// Generate 4-digit random number
	randomBytes := make([]byte, 2)
	rand.Read(randomBytes)
	randomNum := int(randomBytes[0])<<8 | int(randomBytes[1])
	randomNum = randomNum % 10000 // Ensure 4 digits

	return fmt.Sprintf("TO%dNO%d%04d", userId, timestamp, randomNum)
}

// CreateTopupOrder creates a new topup order
// amount: topup amount in quota units (e.g., USD)
// priceRatio: price per quota unit (e.g., 7 CNY per 1 USD)
// discountRate: discount rate (e.g., 0.9 for 10% off)
func CreateTopupOrder(userId int, amount float64, priceRatio float64, discountRate float64) (*TopupOrder, error) {
	if amount <= 0 {
		return nil, errors.New("充值金额必须大于0")
	}

	if discountRate <= 0 || discountRate > 1 {
		discountRate = 1.0
	}

	// Calculate quota (convert amount to internal quota units)
	// amount is in USD, quota is amount * QuotaPerUnit
	quota := int64(amount * float64(common.QuotaPerUnit))

	// Calculate prices
	originalPrice := amount * priceRatio
	finalPrice := originalPrice * discountRate

	// Generate unique order number
	orderNo := GenerateTopupOrderNo(userId)

	// Create order
	now := time.Now().UnixMilli()
	expiredAt := now + (TopupOrderExpirationMinutes * 60 * 1000) // 30 minutes from now

	order := &TopupOrder{
		OrderNo:       orderNo,
		UserId:        userId,
		Amount:        amount,
		Quota:         quota,
		OriginalPrice: originalPrice,
		FinalPrice:    finalPrice,
		DiscountRate:  discountRate,
		Status:        TopupOrderStatusPending,
		CreatedAt:     now,
		ExpiredAt:     expiredAt,
	}

	if err := DB.Create(order).Error; err != nil {
		return nil, err
	}

	return order, nil
}

// GetTopupOrderById retrieves a topup order by ID
func GetTopupOrderById(orderId int) (*TopupOrder, error) {
	var order TopupOrder
	err := DB.Where("id = ?", orderId).First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("订单不存在")
		}
		return nil, err
	}
	return &order, nil
}

// GetTopupOrderByOrderNo retrieves a topup order by order number
func GetTopupOrderByOrderNo(orderNo string) (*TopupOrder, error) {
	var order TopupOrder
	err := DB.Where("order_no = ?", orderNo).First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("订单不存在")
		}
		return nil, err
	}
	return &order, nil
}

// GetTopupOrderByTradeNo retrieves a topup order by payment trade number
func GetTopupOrderByTradeNo(tradeNo string) (*TopupOrder, error) {
	var order TopupOrder
	err := DB.Where("payment_trade_no = ?", tradeNo).First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("订单不存在")
		}
		return nil, err
	}
	return &order, nil
}

// CompleteTopupOrder marks order as paid and adds quota to user
func CompleteTopupOrder(orderId int, paymentTradeNo string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		// 1. Get and lock the order
		var order TopupOrder
		err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ?", orderId).
			First(&order).Error
		if err != nil {
			return err
		}

		// 2. Validate order status
		if order.Status != TopupOrderStatusPending {
			if order.Status == TopupOrderStatusPaid {
				return nil // Already paid, idempotent
			}
			return fmt.Errorf("订单状态不正确: %s", order.Status)
		}

		// 3. Update order status
		now := time.Now().UnixMilli()
		err = tx.Model(&TopupOrder{}).
			Where("id = ?", orderId).
			Updates(map[string]interface{}{
				"status":           TopupOrderStatusPaid,
				"paid_at":          now,
				"payment_trade_no": paymentTradeNo,
			}).Error
		if err != nil {
			return err
		}

		// 4. Add quota to user
		err = tx.Model(&User{}).
			Where("id = ?", order.UserId).
			Update("quota", gorm.Expr("quota + ?", order.Quota)).Error
		if err != nil {
			return err
		}

		common.SysLog(fmt.Sprintf("topup order completed: order_id=%d, user_id=%d, quota=%d", orderId, order.UserId, order.Quota))
		return nil
	})
}

// CancelTopupOrder cancels a pending topup order
func CancelTopupOrder(orderId int, userId int) error {
	now := time.Now().UnixMilli()

	result := DB.Model(&TopupOrder{}).
		Where("id = ? AND user_id = ? AND status = ?", orderId, userId, TopupOrderStatusPending).
		Updates(map[string]interface{}{
			"status":       TopupOrderStatusCancelled,
			"cancelled_at": now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		var order TopupOrder
		err := DB.Where("id = ?", orderId).First(&order).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("订单不存在")
			}
			return err
		}
		if order.UserId != userId {
			return errors.New("无权操作此订单")
		}
		if order.Status != TopupOrderStatusPending {
			return fmt.Errorf("只能取消待支付订单,当前状态: %s", order.Status)
		}
		return errors.New("取消订单失败")
	}

	common.SysLog(fmt.Sprintf("topup order cancelled: order_id=%d, user_id=%d", orderId, userId))
	return nil
}

// GetUserTopupOrders retrieves user's topup orders with pagination
func GetUserTopupOrders(userId int, page int, pageSize int) ([]*TopupOrder, int64, error) {
	var orders []*TopupOrder
	var total int64

	err := DB.Model(&TopupOrder{}).
		Where("user_id = ?", userId).
		Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = DB.Where("user_id = ?", userId).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&orders).Error

	if err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// ExpireOldTopupOrders marks old pending topup orders as expired
func ExpireOldTopupOrders() error {
	now := time.Now().UnixMilli()

	result := DB.Model(&TopupOrder{}).
		Where("status = ? AND expired_at < ?", TopupOrderStatusPending, now).
		Update("status", TopupOrderStatusExpired)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected > 0 {
		common.SysLog(fmt.Sprintf("expired %d old topup orders", result.RowsAffected))
	}

	return nil
}

// UpdateTopupOrderPaymentMethod updates the payment method for a pending order
func UpdateTopupOrderPaymentMethod(orderId int, paymentMethod string) error {
	return DB.Model(&TopupOrder{}).
		Where("id = ? AND status = ?", orderId, TopupOrderStatusPending).
		Update("payment_method", paymentMethod).Error
}

// UpdateTopupOrderTradeNo updates the payment trade number
func UpdateTopupOrderTradeNo(orderId int, tradeNo string) error {
	return DB.Model(&TopupOrder{}).
		Where("id = ?", orderId).
		Update("payment_trade_no", tradeNo).Error
}
