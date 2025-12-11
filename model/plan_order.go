package model

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// PlanOrder represents a plan purchase order
type PlanOrder struct {
	Id            int     `json:"id" gorm:"primaryKey;autoIncrement"`
	OrderNo       string  `json:"order_no" gorm:"type:varchar(64);uniqueIndex;not null"` // Unique order number
	UserId        int     `json:"user_id" gorm:"not null;index"`
	PlanId        int     `json:"plan_id" gorm:"not null;index"` // Reference to Plan.Id (not Name)

	// Price snapshot (preserve at purchase time)
	PlanPrice         float64 `json:"plan_price" gorm:"type:decimal(10,2);not null"`          // Actual sale price
	PlanOriginalPrice float64 `json:"plan_original_price" gorm:"type:decimal(10,2);default:0"` // Original price before discount
	FinalPrice        float64 `json:"final_price" gorm:"type:decimal(10,2);not null"`         // Final payment amount

	// Payment information
	PaymentMethod  string `json:"payment_method" gorm:"type:varchar(50)"`   // alipay, wechat, stripe, creem
	PaymentTradeNo string `json:"payment_trade_no" gorm:"type:varchar(255);index"` // Payment gateway transaction ID

	// Status management
	Status string `json:"status" gorm:"type:varchar(20);default:'pending';index"` // pending, paid, delivered, expired, cancelled

	// Timestamps (milliseconds)
	CreatedAt  int64 `json:"created_at" gorm:"index;not null"`  // Order creation time
	ExpiredAt  int64 `json:"expired_at"`                        // Expiration time for pending orders
	PaidAt     int64 `json:"paid_at"`                           // Payment completion time
	DeliveredAt int64 `json:"delivered_at"`                     // Plan delivery time
	CancelledAt int64 `json:"cancelled_at"`                     // Cancellation time

	// Relationships
	UserPlanId *int `json:"user_plan_id"` // Created UserPlan instance ID (nil = not yet delivered)

	// Delivery retry tracking
	DeliveryRetryCount int `json:"delivery_retry_count" gorm:"default:0"` // Number of delivery retry attempts

	// Associations
	User     *User     `json:"user,omitempty" gorm:"foreignKey:UserId"`
	Plan     *Plan     `json:"plan,omitempty" gorm:"foreignKey:PlanId"`
	UserPlan *UserPlan `json:"user_plan,omitempty" gorm:"foreignKey:UserPlanId"`
}

// Order status constants
const (
	OrderStatusPending   = "pending"
	OrderStatusPaid      = "paid"
	OrderStatusDelivered = "delivered"
	OrderStatusExpired   = "expired"
	OrderStatusCancelled = "cancelled"
)

// Payment method constants
const (
	PaymentMethodAlipay = "alipay"
	PaymentMethodWechat = "wechat"
	PaymentMethodStripe = "stripe"
	PaymentMethodCreem  = "creem"
)

// Order expiration time (30 minutes)
const OrderExpirationMinutes = 30

func (po *PlanOrder) TableName() string {
	return "plan_orders"
}

// orderLocks stores in-memory locks for each order to prevent concurrent processing
var orderLocks sync.Map

// getOrderLock returns a lock for the given order ID or trade number
func getOrderLock(key string) *sync.Mutex {
	lock, _ := orderLocks.LoadOrStore(key, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// GenerateOrderNo generates a unique order number
// Format: PO{userId}NO{timestamp}{4-digit-random}
func GenerateOrderNo(userId int) string {
	timestamp := time.Now().UnixMilli()

	// Generate 4-digit random number
	randomBytes := make([]byte, 2)
	rand.Read(randomBytes)
	randomNum := int(randomBytes[0])<<8 | int(randomBytes[1])
	randomNum = randomNum % 10000 // Ensure 4 digits

	return fmt.Sprintf("PO%dNO%d%04d", userId, timestamp, randomNum)
}

// CreatePlanOrder creates a new plan order with validation
func CreatePlanOrder(userId int, planId int) (*PlanOrder, error) {
	// Start transaction
	var order *PlanOrder

	err := DB.Transaction(func(tx *gorm.DB) error {
		// 1. Load and validate plan
		var plan Plan
		if err := tx.Where("id = ?", planId).First(&plan).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("套餐不存在")
			}
			return err
		}

		// 2. Check if plan is purchasable
		if plan.Status != PlanStatusEnabled {
			return errors.New("该套餐已禁用")
		}

		// Check purchasable field
		if plan.Purchasable != 1 {
			return errors.New("该套餐不支持在线购买")
		}

		// 3. Validate queue capacity (max 10 active plans)
		if err := ValidateQueueCapacity(userId); err != nil {
			return err
		}

		// 4. Calculate price (use plan's current price)
		planPrice := plan.Price
		originalPrice := plan.OriginalPrice
		if originalPrice == 0 {
			originalPrice = planPrice // No discount
		}
		finalPrice := planPrice

		// 5. Generate unique order number
		orderNo := GenerateOrderNo(userId)

		// 6. Create order
		now := time.Now().UnixMilli()
		expiredAt := now + (OrderExpirationMinutes * 60 * 1000) // 30 minutes from now

		order = &PlanOrder{
			OrderNo:            orderNo,
			UserId:             userId,
			PlanId:             plan.Id,
			PlanPrice:          planPrice,
			PlanOriginalPrice:  originalPrice,
			FinalPrice:         finalPrice,
			Status:             OrderStatusPending,
			CreatedAt:          now,
			ExpiredAt:          expiredAt,
			DeliveryRetryCount: 0,
		}

		if err := tx.Create(order).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return order, nil
}

// ValidateQueueCapacity checks if user has < 10 active plans
func ValidateQueueCapacity(userId int) error {
	var count int64
	err := DB.Model(&UserPlan{}).
		Where("user_id = ? AND status = ?", userId, UserPlanStatusActive).
		Count(&count).Error

	if err != nil {
		return err
	}

	if count >= 10 {
		return errors.New("您已拥有10个套餐,无法继续购买")
	}

	return nil
}

// GetOrderById retrieves an order by ID
func GetOrderById(orderId int) (*PlanOrder, error) {
	var order PlanOrder
	err := DB.Where("id = ?", orderId).First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("订单不存在")
		}
		return nil, err
	}
	return &order, nil
}

// GetOrderByOrderNo retrieves an order by order number
func GetOrderByOrderNo(orderNo string) (*PlanOrder, error) {
	var order PlanOrder
	err := DB.Where("order_no = ?", orderNo).First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("订单不存在")
		}
		return nil, err
	}
	return &order, nil
}

// GetOrderByTradeNo retrieves an order by payment trade number
func GetOrderByTradeNo(tradeNo string) (*PlanOrder, error) {
	var order PlanOrder
	err := DB.Where("payment_trade_no = ?", tradeNo).First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("订单不存在")
		}
		return nil, err
	}
	return &order, nil
}

// UpdateOrderStatus updates order status
func UpdateOrderStatus(orderId int, status string, tx *gorm.DB) error {
	db := DB
	if tx != nil {
		db = tx
	}

	return db.Model(&PlanOrder{}).
		Where("id = ?", orderId).
		Update("status", status).Error
}

// GetUserOrders retrieves user's orders with pagination
func GetUserOrders(userId int, page int, pageSize int) ([]*PlanOrder, int64, error) {
	var orders []*PlanOrder
	var total int64

	// Get total count
	err := DB.Model(&PlanOrder{}).
		Where("user_id = ?", userId).
		Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// Get paginated orders with plan info
	offset := (page - 1) * pageSize
	err = DB.Preload("Plan").
		Where("user_id = ?", userId).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&orders).Error

	if err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// GetAllOrders retrieves all orders with filters (admin)
func GetAllOrders(page int, pageSize int, status string, userId int, orderNo string) ([]*PlanOrder, int64, error) {
	var orders []*PlanOrder
	var total int64

	query := DB.Model(&PlanOrder{})

	// Apply filters
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if orderNo != "" {
		query = query.Where("order_no LIKE ?", "%"+orderNo+"%")
	}

	// Get total count
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// Get paginated orders with associations
	offset := (page - 1) * pageSize
	err = query.Preload("User").
		Preload("Plan").
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&orders).Error

	if err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// ExpireOldOrders marks old pending orders as expired
func ExpireOldOrders() error {
	cutoff := time.Now().UnixMilli() - (OrderExpirationMinutes * 60 * 1000)

	result := DB.Model(&PlanOrder{}).
		Where("status = ? AND created_at < ?", OrderStatusPending, cutoff).
		Update("status", OrderStatusExpired)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected > 0 {
		common.SysLog(fmt.Sprintf("expired %d old orders", result.RowsAffected))
	}

	return nil
}

// GetFailedDeliveryOrders retrieves orders that need delivery retry
func GetFailedDeliveryOrders(maxRetries int) ([]*PlanOrder, error) {
	cutoff := time.Now().UnixMilli() - (5 * 60 * 1000) // 5 minutes ago

	var orders []*PlanOrder
	err := DB.Where(
		"status = ? AND paid_at < ? AND (user_plan_id IS NULL OR user_plan_id = 0) AND delivery_retry_count < ?",
		OrderStatusPaid,
		cutoff,
		maxRetries,
	).Find(&orders).Error

	return orders, err
}

// IncrementDeliveryRetryCount increments the delivery retry counter
func IncrementDeliveryRetryCount(orderId int) error {
	return DB.Model(&PlanOrder{}).
		Where("id = ?", orderId).
		Update("delivery_retry_count", gorm.Expr("delivery_retry_count + 1")).Error
}
