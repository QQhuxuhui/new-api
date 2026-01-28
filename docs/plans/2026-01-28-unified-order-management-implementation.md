# 统一订单管理页面实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将管理员后台的套餐订单和充值订单合并到统一的"订单管理"页面

**Architecture:** 新增管理员充值订单 API，修改前端 AdminOrders 页面支持 Tab 切换和可展开行

**Tech Stack:** Go (Gin), React (Semi UI), GORM

---

## Task 1: 后端 - 新增 GetAllTopupOrders 模型函数

**Files:**
- Modify: `model/topup_order.go:286-300`

**Step 1: 在 topup_order.go 末尾添加 GetAllTopupOrders 函数**

```go
// GetAllTopupOrders retrieves all topup orders with filters (admin)
func GetAllTopupOrders(page int, pageSize int, status string, userId int, username string, email string, orderNo string, paymentMethod string, minAmount float64, maxAmount float64, startTime int64, endTime int64) ([]*TopupOrder, int64, error) {
	var orders []*TopupOrder
	var total int64

	query := DB.Model(&TopupOrder{})

	// Apply filters
	if status != "" {
		query = query.Where("topup_orders.status = ?", status)
	}
	if userId > 0 {
		query = query.Where("topup_orders.user_id = ?", userId)
	}
	if orderNo != "" {
		query = query.Where("topup_orders.order_no LIKE ?", "%"+orderNo+"%")
	}
	if paymentMethod != "" {
		query = query.Where("topup_orders.payment_method = ?", paymentMethod)
	}
	if minAmount > 0 {
		query = query.Where("topup_orders.amount >= ?", minAmount)
	}
	if maxAmount > 0 {
		query = query.Where("topup_orders.amount <= ?", maxAmount)
	}
	if startTime > 0 {
		query = query.Where("topup_orders.created_at >= ?", startTime)
	}
	if endTime > 0 {
		query = query.Where("topup_orders.created_at <= ?", endTime)
	}

	// Join users table for username/email search
	if username != "" || email != "" {
		query = query.Joins("LEFT JOIN users ON users.id = topup_orders.user_id")
		if username != "" {
			query = query.Where("users.username LIKE ?", "%"+username+"%")
		}
		if email != "" {
			query = query.Where("users.email LIKE ?", "%"+email+"%")
		}
	}

	// Get total count
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// Get paginated orders with user info
	offset := (page - 1) * pageSize
	err = query.Preload("User").
		Order("topup_orders.created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&orders).Error

	if err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}
```

**Step 2: 验证代码编译通过**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./...`
Expected: 编译成功，无错误

**Step 3: 提交**

```bash
git add model/topup_order.go
git commit -m "feat(model): add GetAllTopupOrders for admin order management"
```

---

## Task 2: 后端 - 新增管理员充值订单控制器

**Files:**
- Create: `controller/admin_topup_order.go`

**Step 1: 创建 admin_topup_order.go 文件**

```go
package controller

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetAllTopupOrders returns all topup orders (admin only)
func GetAllTopupOrders(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Parse filters
	status := c.Query("status")
	userId, _ := strconv.Atoi(c.Query("user_id"))
	username := c.Query("username")
	email := c.Query("email")
	orderNo := c.Query("order_no")
	paymentMethod := c.Query("payment_method")
	minAmount, _ := strconv.ParseFloat(c.Query("min_amount"), 64)
	maxAmount, _ := strconv.ParseFloat(c.Query("max_amount"), 64)
	startTime, _ := strconv.ParseInt(c.Query("start_time"), 10, 64)
	endTime, _ := strconv.ParseInt(c.Query("end_time"), 10, 64)

	orders, total, err := model.GetAllTopupOrders(
		page, pageSize, status, userId, username, email,
		orderNo, paymentMethod, minAmount, maxAmount, startTime, endTime,
	)
	if err != nil {
		common.ApiError(c, fmt.Errorf("获取充值订单列表失败: %w", err))
		return
	}

	// Build response
	orderList := make([]gin.H, 0, len(orders))
	for _, order := range orders {
		orderInfo := gin.H{
			"id":              order.Id,
			"order_no":        order.OrderNo,
			"user_id":         order.UserId,
			"amount":          order.Amount,
			"quota":           order.Quota,
			"original_price":  order.OriginalPrice,
			"final_price":     order.FinalPrice,
			"discount_rate":   order.DiscountRate,
			"payment_method":  order.PaymentMethod,
			"payment_trade_no": order.PaymentTradeNo,
			"status":          order.Status,
			"created_at":      order.CreatedAt,
			"expired_at":      order.ExpiredAt,
			"paid_at":         order.PaidAt,
			"cancelled_at":    order.CancelledAt,
		}

		// Add user info if available
		if order.User != nil {
			orderInfo["username"] = order.User.Username
			orderInfo["user_email"] = order.User.Email
		}

		orderList = append(orderList, orderInfo)
	}

	common.ApiSuccess(c, gin.H{
		"orders":    orderList,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetTopupOrderDetailAdmin returns detailed information of a topup order (admin only)
func GetTopupOrderDetailAdmin(c *gin.Context) {
	orderId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, errors.New("无效的订单ID"))
		return
	}

	order, err := model.GetTopupOrderById(orderId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Load user info
	user, _ := model.GetUserById(order.UserId, false)

	orderDetail := gin.H{
		"id":               order.Id,
		"order_no":         order.OrderNo,
		"user_id":          order.UserId,
		"amount":           order.Amount,
		"quota":            order.Quota,
		"original_price":   order.OriginalPrice,
		"final_price":      order.FinalPrice,
		"discount_rate":    order.DiscountRate,
		"payment_method":   order.PaymentMethod,
		"payment_trade_no": order.PaymentTradeNo,
		"status":           order.Status,
		"created_at":       order.CreatedAt,
		"expired_at":       order.ExpiredAt,
		"paid_at":          order.PaidAt,
		"cancelled_at":     order.CancelledAt,
	}

	if user != nil {
		orderDetail["username"] = user.Username
		orderDetail["user_email"] = user.Email
	}

	common.ApiSuccess(c, orderDetail)
}

// AdminCancelTopupOrder cancels a pending topup order (admin only)
func AdminCancelTopupOrder(c *gin.Context) {
	orderId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, errors.New("无效的订单ID"))
		return
	}

	order, err := model.GetTopupOrderById(orderId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if order.Status != model.TopupOrderStatusPending {
		common.ApiError(c, fmt.Errorf("只有待支付状态的订单才能取消，当前状态: %s", order.Status))
		return
	}

	err = model.DB.Model(order).Updates(map[string]interface{}{
		"status":       model.TopupOrderStatusCancelled,
		"cancelled_at": common.GetTimestamp(),
	}).Error
	if err != nil {
		common.ApiError(c, fmt.Errorf("取消订单失败: %w", err))
		return
	}

	adminId := c.GetInt("id")
	username := c.GetString("username")
	common.SysLog(fmt.Sprintf("Admin %s (ID: %d) cancelled topup order %d", username, adminId, orderId))

	common.ApiSuccess(c, gin.H{"message": "订单已取消"})
}

// DeleteTopupOrder deletes an expired or cancelled topup order (admin only)
func DeleteTopupOrder(c *gin.Context) {
	orderId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, errors.New("无效的订单ID"))
		return
	}

	order, err := model.GetTopupOrderById(orderId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if order.Status != model.TopupOrderStatusExpired && order.Status != model.TopupOrderStatusCancelled {
		common.ApiError(c, fmt.Errorf("只有已过期或已取消的订单才能删除，当前状态: %s", order.Status))
		return
	}

	err = model.DB.Delete(order).Error
	if err != nil {
		common.ApiError(c, fmt.Errorf("删除订单失败: %w", err))
		return
	}

	adminId := c.GetInt("id")
	username := c.GetString("username")
	common.SysLog(fmt.Sprintf("Admin %s (ID: %d) deleted topup order %d", username, adminId, orderId))

	common.ApiSuccess(c, gin.H{"message": "订单已删除"})
}
```

**Step 2: 验证代码编译通过**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./...`
Expected: 编译成功

**Step 3: 提交**

```bash
git add controller/admin_topup_order.go
git commit -m "feat(controller): add admin topup order controller"
```

---

## Task 3: 后端 - 注册管理员充值订单路由

**Files:**
- Modify: `router/api-router.go:138`

**Step 1: 在 adminRoute 组中添加充值订单路由**

在 `router/api-router.go` 的 adminRoute 组中（约第138行后），添加：

```go
				// Topup order management (admin)
				adminRoute.GET("/topup-orders", controller.GetAllTopupOrders)
				adminRoute.GET("/topup-orders/:id", controller.GetTopupOrderDetailAdmin)
				adminRoute.POST("/topup-orders/:id/cancel", controller.AdminCancelTopupOrder)
				adminRoute.DELETE("/topup-orders/:id", controller.DeleteTopupOrder)
```

**Step 2: 验证代码编译通过**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./...`
Expected: 编译成功

**Step 3: 提交**

```bash
git add router/api-router.go
git commit -m "feat(router): add admin topup order routes"
```

---

## Task 4: 前端 - 修改 AdminOrders 页面添加 Tab 切换和完整筛选

**Files:**
- Modify: `web/src/pages/AdminOrders/index.jsx`

**Step 1: 添加 Tab 状态、充值订单状态和完整筛选状态**

在组件顶部的 state 声明区域，替换原有的 filters 状态：

```jsx
// Order type tab
const [orderType, setOrderType] = useState('plan'); // 'plan' or 'topup'

// Topup orders state
const [topupOrders, setTopupOrders] = useState([]);
const [topupPagination, setTopupPagination] = useState({
  currentPage: 1,
  pageSize: 20,
  total: 0,
});
const [topupLoading, setTopupLoading] = useState(false);

// Expanded rows
const [expandedRowKeys, setExpandedRowKeys] = useState([]);

// Complete filters (replace original filters)
const [filters, setFilters] = useState({
  status: '',
  userId: '',
  username: '',
  email: '',
  orderNo: '',
  paymentMethod: '',
  minAmount: '',
  maxAmount: '',
  startTime: null,
  endTime: null,
});
```

**Step 2: 更新加载套餐订单的函数，支持完整筛选**

```jsx
// Load orders with complete filters
const loadOrders = async (page = 1) => {
  setLoading(true);
  try {
    const params = new URLSearchParams({
      page: page.toString(),
      page_size: pagination.pageSize.toString(),
    });

    if (filters.status) params.append('status', filters.status);
    if (filters.userId) params.append('user_id', filters.userId);
    if (filters.username) params.append('username', filters.username);
    if (filters.email) params.append('email', filters.email);
    if (filters.orderNo) params.append('order_no', filters.orderNo);
    if (filters.paymentMethod) params.append('payment_method', filters.paymentMethod);
    if (filters.minAmount) params.append('min_amount', filters.minAmount);
    if (filters.maxAmount) params.append('max_amount', filters.maxAmount);
    if (filters.startTime) params.append('start_time', filters.startTime.valueOf().toString());
    if (filters.endTime) params.append('end_time', filters.endTime.valueOf().toString());

    const res = await API.get(`/api/user/plan-orders?${params.toString()}`);
    const { success, message, data } = res.data;
    if (success && data) {
      setOrders(data.orders || []);
      setPagination({
        ...pagination,
        currentPage: data.page || page,
        total: data.total || 0,
      });
    } else {
      showError(message || t('加载失败'));
    }
  } catch (e) {
    showError(e.message || t('网络错误'));
  }
  setLoading(false);
};
```

**Step 3: 添加加载充值订单的函数，支持完整筛选**

```jsx
// Load topup orders with complete filters
const loadTopupOrders = async (page = 1) => {
  setTopupLoading(true);
  try {
    const params = new URLSearchParams({
      page: page.toString(),
      page_size: topupPagination.pageSize.toString(),
    });

    if (filters.status) params.append('status', filters.status);
    if (filters.userId) params.append('user_id', filters.userId);
    if (filters.username) params.append('username', filters.username);
    if (filters.email) params.append('email', filters.email);
    if (filters.orderNo) params.append('order_no', filters.orderNo);
    if (filters.paymentMethod) params.append('payment_method', filters.paymentMethod);
    if (filters.minAmount) params.append('min_amount', filters.minAmount);
    if (filters.maxAmount) params.append('max_amount', filters.maxAmount);
    if (filters.startTime) params.append('start_time', filters.startTime.valueOf().toString());
    if (filters.endTime) params.append('end_time', filters.endTime.valueOf().toString());

    const res = await API.get(`/api/user/topup-orders?${params.toString()}`);
    const { success, message, data } = res.data;
    if (success && data) {
      setTopupOrders(data.orders || []);
      setTopupPagination({
        ...topupPagination,
        currentPage: data.page || page,
        total: data.total || 0,
      });
    } else {
      showError(message || t('加载失败'));
    }
  } catch (e) {
    showError(e.message || t('网络错误'));
  }
  setTopupLoading(false);
};
```

**Step 4: 添加 Tab 切换处理和搜索处理**

```jsx
// Handle tab change
const handleTabChange = (key) => {
  setOrderType(key);
  setExpandedRowKeys([]);
  if (key === 'topup' && topupOrders.length === 0) {
    loadTopupOrders(1);
  }
};

// Handle search - search current tab
const handleSearch = () => {
  if (orderType === 'plan') {
    loadOrders(1);
  } else {
    loadTopupOrders(1);
  }
};

// Handle reset filters
const handleResetFilters = () => {
  setFilters({
    status: '',
    userId: '',
    username: '',
    email: '',
    orderNo: '',
    paymentMethod: '',
    minAmount: '',
    maxAmount: '',
    startTime: null,
    endTime: null,
  });
  setTimeout(() => {
    if (orderType === 'plan') {
      loadOrders(1);
    } else {
      loadTopupOrders(1);
    }
  }, 100);
};
```

**Step 5: 提交**

```bash
git add web/src/pages/AdminOrders/index.jsx
git commit -m "feat(frontend): add tab state and complete filter support"
```

---

## Task 5: 前端 - 添加完整筛选 UI 组件

**Files:**
- Modify: `web/src/pages/AdminOrders/index.jsx`

**Step 1: 导入 DatePicker 组件**

在文件顶部的 import 中添加 `DatePicker`：

```jsx
import {
  Card,
  Table,
  Tag,
  Button,
  Typography,
  Input,
  Select,
  Space,
  Modal,
  Toast,
  Spin,
  Empty,
  Descriptions,
  Popconfirm,
  Tabs,
  DatePicker,  // 添加这行
} from '@douyinfe/semi-ui';
```

**Step 2: 替换筛选区域 UI**

将原有的筛选区域替换为完整的筛选 UI：

```jsx
{/* Filters */}
<div className='mb-6'>
  <Space spacing='medium' wrap>
    {/* 第一行：基础筛选 */}
    <Select
      placeholder={t('全部状态')}
      style={{ width: 120 }}
      value={filters.status}
      onChange={(value) => setFilters({ ...filters, status: value })}
      showClear
    >
      <Select.Option value='pending'>{t('待支付')}</Select.Option>
      <Select.Option value='paid'>{t('已支付')}</Select.Option>
      {orderType === 'plan' && <Select.Option value='delivered'>{t('已完成')}</Select.Option>}
      <Select.Option value='expired'>{t('已过期')}</Select.Option>
      <Select.Option value='cancelled'>{t('已取消')}</Select.Option>
    </Select>

    <Select
      placeholder={t('支付方式')}
      style={{ width: 120 }}
      value={filters.paymentMethod}
      onChange={(value) => setFilters({ ...filters, paymentMethod: value })}
      showClear
    >
      <Select.Option value='epay'>{t('易支付')}</Select.Option>
      <Select.Option value='stripe'>{t('Stripe')}</Select.Option>
      <Select.Option value='creem'>{t('Creem')}</Select.Option>
    </Select>

    <DatePicker
      type='dateRange'
      placeholder={[t('开始日期'), t('结束日期')]}
      style={{ width: 260 }}
      value={filters.startTime && filters.endTime ? [filters.startTime, filters.endTime] : null}
      onChange={(dates) => {
        if (dates && dates.length === 2) {
          setFilters({ ...filters, startTime: dates[0], endTime: dates[1] });
        } else {
          setFilters({ ...filters, startTime: null, endTime: null });
        }
      }}
    />
  </Space>

  <Space spacing='medium' wrap style={{ marginTop: 12 }}>
    {/* 第二行：金额和用户搜索 */}
    <Input
      placeholder={t('最小金额')}
      style={{ width: 100 }}
      type='number'
      value={filters.minAmount}
      onChange={(value) => setFilters({ ...filters, minAmount: value })}
    />
    <span>-</span>
    <Input
      placeholder={t('最大金额')}
      style={{ width: 100 }}
      type='number'
      value={filters.maxAmount}
      onChange={(value) => setFilters({ ...filters, maxAmount: value })}
    />

    <Input
      placeholder={t('用户ID')}
      style={{ width: 100 }}
      value={filters.userId}
      onChange={(value) => setFilters({ ...filters, userId: value })}
    />

    <Input
      placeholder={t('用户名')}
      style={{ width: 120 }}
      value={filters.username}
      onChange={(value) => setFilters({ ...filters, username: value })}
    />

    <Input
      placeholder={t('邮箱')}
      style={{ width: 150 }}
      value={filters.email}
      onChange={(value) => setFilters({ ...filters, email: value })}
    />

    <Input
      placeholder={t('订单号')}
      style={{ width: 180 }}
      value={filters.orderNo}
      onChange={(value) => setFilters({ ...filters, orderNo: value })}
    />

    <Button
      theme='solid'
      type='primary'
      icon={<IconSearch />}
      onClick={handleSearch}
    >
      {t('搜索')}
    </Button>

    <Button onClick={handleResetFilters}>
      {t('重置')}
    </Button>
  </Space>
</div>
```

**Step 3: 提交**

```bash
git add web/src/pages/AdminOrders/index.jsx
git commit -m "feat(frontend): add complete filter UI components"
```

---

## Task 6: 前端 - 添加可展开行配置

**Files:**
- Modify: `web/src/pages/AdminOrders/index.jsx`

**Step 1: 添加展开行渲染函数**

```jsx
// Render expanded row content for plan orders
const renderPlanExpandedRow = (record) => (
  <div style={{ padding: '16px', background: '#fafafa' }}>
    <div style={{ display: 'flex', gap: '48px' }}>
      <div>
        <Text strong style={{ display: 'block', marginBottom: '8px' }}>{t('用户信息')}</Text>
        <div>用户ID: {record.user_id}</div>
        <div>邮箱: {record.user_email || '-'}</div>
      </div>
      <div>
        <Text strong style={{ display: 'block', marginBottom: '8px' }}>{t('订单信息')}</Text>
        <div>订单类型: {t('套餐订单')}</div>
        <div>套餐名称: {record.plan_name || '-'}</div>
        <div>套餐额度: {record.plan_quota ? `$${(record.plan_quota / 500000).toFixed(2)}` : '-'}</div>
        <div>有效期: {record.plan_validity_days ? `${record.plan_validity_days} 天` : '-'}</div>
        <div>原价: ¥{record.original_price?.toFixed(2) || '0.00'}</div>
        <div>支付单号: {record.payment_trade_no || '-'}</div>
        <div>支付时间: {record.paid_at ? timestamp2string(record.paid_at / 1000) : '-'}</div>
        <div>交付状态: {record.status === 'delivered' ? t('已交付') : (record.status === 'paid' ? t('待交付') : '-')}</div>
        {record.delivered_at && <div>发放时间: {timestamp2string(record.delivered_at / 1000)}</div>}
        <div>用户套餐ID: {record.user_plan_id || '-'}</div>
      </div>
    </div>
  </div>
);

// Render expanded row content for topup orders
const renderTopupExpandedRow = (record) => (
  <div style={{ padding: '16px', background: '#fafafa' }}>
    <div style={{ display: 'flex', gap: '48px' }}>
      <div>
        <Text strong style={{ display: 'block', marginBottom: '8px' }}>{t('用户信息')}</Text>
        <div>用户ID: {record.user_id}</div>
        <div>邮箱: {record.user_email || '-'}</div>
      </div>
      <div>
        <Text strong style={{ display: 'block', marginBottom: '8px' }}>{t('订单信息')}</Text>
        <div>订单类型: {t('充值订单')}</div>
        <div>充值金额: ${record.amount?.toFixed(2) || '0.00'}</div>
        <div>获得额度: {record.quota?.toLocaleString() || '0'}</div>
        <div>折扣率: {((record.discount_rate || 1) * 100).toFixed(0)}%</div>
        <div>支付单号: {record.payment_trade_no || '-'}</div>
        <div>支付时间: {record.paid_at ? timestamp2string(record.paid_at / 1000) : '-'}</div>
      </div>
    </div>
  </div>
);
```

**Step 2: 修改表格配置添加展开行**

在 Table 组件中添加 expandedRowRender 和相关配置：

```jsx
expandedRowRender={(record) => orderType === 'plan' ? renderPlanExpandedRow(record) : renderTopupExpandedRow(record)}
expandedRowKeys={expandedRowKeys}
onExpandedRowsChange={(keys) => setExpandedRowKeys(keys)}
```

**Step 3: 提交**

```bash
git add web/src/pages/AdminOrders/index.jsx
git commit -m "feat(frontend): add expandable row for order details"
```

---

## Task 7: 前端 - 添加充值订单表格列配置

**Files:**
- Modify: `web/src/pages/AdminOrders/index.jsx`

**Step 1: 添加充值订单表格列**

```jsx
// Topup order table columns
const topupColumns = [
  {
    title: t('订单号'),
    dataIndex: 'order_no',
    key: 'order_no',
    width: 180,
    render: (text) => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{text}</span>,
  },
  {
    title: t('用户'),
    key: 'user',
    width: 120,
    render: (_, record) => (
      <Text strong>{record.username || `ID: ${record.user_id}`}</Text>
    ),
  },
  {
    title: t('充值金额'),
    dataIndex: 'amount',
    key: 'amount',
    width: 100,
    render: (amount) => <span>${amount?.toFixed(2) || '0.00'}</span>,
  },
  {
    title: t('实付金额'),
    dataIndex: 'final_price',
    key: 'final_price',
    width: 100,
    render: (price) => (
      <span style={{ fontWeight: 600 }}>¥{price?.toFixed(2) || '0.00'}</span>
    ),
  },
  {
    title: t('支付方式'),
    dataIndex: 'payment_method',
    key: 'payment_method',
    width: 100,
    render: (method) => method || '-',
  },
  {
    title: t('状态'),
    dataIndex: 'status',
    key: 'status',
    width: 100,
    render: (status) => getStatusTag(status),
  },
  {
    title: t('创建时间'),
    dataIndex: 'created_at',
    key: 'created_at',
    width: 160,
    render: (time) => (time ? timestamp2string(time / 1000) : '-'),
  },
  {
    title: t('操作'),
    key: 'action',
    width: 120,
    fixed: 'right',
    render: (_, record) => {
      const actions = [];
      if (record.status === 'pending') {
        actions.push(
          <Popconfirm
            key='cancel'
            title={t('确认取消订单')}
            content={t('确认要取消该订单吗？')}
            onConfirm={() => handleCancelTopupOrder(record.id)}
          >
            <Button size='small' type='tertiary' icon={<IconClose />} />
          </Popconfirm>
        );
      }
      if (record.status === 'expired' || record.status === 'cancelled') {
        actions.push(
          <Popconfirm
            key='delete'
            title={t('确认删除订单')}
            content={t('确认要删除该订单吗？')}
            onConfirm={() => handleDeleteTopupOrder(record.id)}
          >
            <Button size='small' type='danger' icon={<IconDelete />} />
          </Popconfirm>
        );
      }
      return <Space>{actions}</Space>;
    },
  },
];
```

**Step 2: 添加充值订单操作函数**

```jsx
// Handle cancel topup order
const handleCancelTopupOrder = async (orderId) => {
  try {
    const res = await API.post(`/api/user/topup-orders/${orderId}/cancel`);
    const { success, message } = res.data;
    if (success) {
      showSuccess(t('订单已取消'));
      loadTopupOrders(topupPagination.currentPage);
    } else {
      showError(message || t('操作失败'));
    }
  } catch (e) {
    showError(e.message || t('网络错误'));
  }
};

// Handle delete topup order
const handleDeleteTopupOrder = async (orderId) => {
  try {
    const res = await API.delete(`/api/user/topup-orders/${orderId}`);
    const { success, message } = res.data;
    if (success) {
      showSuccess(t('订单已删除'));
      loadTopupOrders(topupPagination.currentPage);
    } else {
      showError(message || t('操作失败'));
    }
  } catch (e) {
    showError(e.message || t('网络错误'));
  }
};
```

**Step 3: 提交**

```bash
git add web/src/pages/AdminOrders/index.jsx
git commit -m "feat(frontend): add topup order columns and actions"
```

---

## Task 8: 前端 - 更新页面布局添加 Tabs

**Files:**
- Modify: `web/src/pages/AdminOrders/index.jsx`

**Step 1: 导入 Tabs 组件**

在文件顶部的 import 中添加 `Tabs`：

```jsx
import {
  Card,
  Table,
  Tag,
  Button,
  Typography,
  Input,
  Select,
  Space,
  Modal,
  Toast,
  Spin,
  Empty,
  Descriptions,
  Popconfirm,
  Tabs,  // 添加这行
} from '@douyinfe/semi-ui';
```

**Step 2: 修改页面布局添加 Tabs**

将表格区域替换为带 Tabs 的布局：

```jsx
{/* Tabs */}
<Tabs type='line' activeKey={orderType} onChange={handleTabChange}>
  <Tabs.TabPane tab={t('套餐订单')} itemKey='plan'>
    {/* Plan orders table */}
    {loading && orders.length === 0 ? (
      <div className='flex items-center justify-center py-20'>
        <Spin size='large' />
      </div>
    ) : orders.length > 0 ? (
      <Table
        columns={planColumns}
        dataSource={orders}
        pagination={{
          currentPage: pagination.currentPage,
          pageSize: pagination.pageSize,
          total: pagination.total,
          onPageChange: handlePageChange,
          showSizeChanger: false,
        }}
        loading={loading}
        rowKey='order_id'
        scroll={{ x: 1400 }}
        expandedRowRender={renderPlanExpandedRow}
        expandedRowKeys={expandedRowKeys}
        onExpandedRowsChange={(keys) => setExpandedRowKeys(keys)}
      />
    ) : (
      <Empty
        image={<IconShoppingBag size='extra-large' style={{ fontSize: 64 }} />}
        title={t('暂无订单')}
        description={t('暂时没有符合条件的订单')}
      />
    )}
  </Tabs.TabPane>
  <Tabs.TabPane tab={t('充值订单')} itemKey='topup'>
    {/* Topup orders table */}
    {topupLoading && topupOrders.length === 0 ? (
      <div className='flex items-center justify-center py-20'>
        <Spin size='large' />
      </div>
    ) : topupOrders.length > 0 ? (
      <Table
        columns={topupColumns}
        dataSource={topupOrders}
        pagination={{
          currentPage: topupPagination.currentPage,
          pageSize: topupPagination.pageSize,
          total: topupPagination.total,
          onPageChange: (page) => loadTopupOrders(page),
          showSizeChanger: false,
        }}
        loading={topupLoading}
        rowKey='id'
        scroll={{ x: 1000 }}
        expandedRowRender={renderTopupExpandedRow}
        expandedRowKeys={expandedRowKeys}
        onExpandedRowsChange={(keys) => setExpandedRowKeys(keys)}
      />
    ) : (
      <Empty
        image={<IconShoppingBag size='extra-large' style={{ fontSize: 64 }} />}
        title={t('暂无订单')}
        description={t('暂时没有符合条件的充值订单')}
      />
    )}
  </Tabs.TabPane>
</Tabs>
```

**Step 3: 提交**

```bash
git add web/src/pages/AdminOrders/index.jsx
git commit -m "feat(frontend): add Tabs layout for order types"
```

---

## Task 9: 前端 - 简化套餐订单表格列（移除详情按钮）

**Files:**
- Modify: `web/src/pages/AdminOrders/index.jsx`

**Step 1: 重命名 columns 为 planColumns 并移除详情按钮**

将原有的 `columns` 重命名为 `planColumns`，并从操作列中移除详情按钮，同时移除不必要的列（发放时间、重试次数等移到展开行）：

```jsx
// Plan order table columns
const planColumns = [
  {
    title: t('订单号'),
    dataIndex: 'order_no',
    key: 'order_no',
    width: 180,
    render: (text) => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{text}</span>,
  },
  {
    title: t('用户'),
    key: 'user',
    width: 120,
    render: (_, record) => (
      <Text strong>{record.username || `ID: ${record.user_id}`}</Text>
    ),
  },
  {
    title: t('套餐'),
    dataIndex: 'plan_name',
    key: 'plan_name',
    width: 120,
  },
  {
    title: t('金额'),
    dataIndex: 'final_price',
    key: 'final_price',
    width: 100,
    render: (price) => (
      <span style={{ fontWeight: 600 }}>¥{price?.toFixed(2) || '0.00'}</span>
    ),
  },
  {
    title: t('支付方式'),
    dataIndex: 'payment_method',
    key: 'payment_method',
    width: 100,
    render: (method) => method || '-',
  },
  {
    title: t('状态'),
    dataIndex: 'status',
    key: 'status',
    width: 100,
    render: (status) => getStatusTag(status),
  },
  {
    title: t('创建时间'),
    dataIndex: 'created_at',
    key: 'created_at',
    width: 160,
    render: (time) => (time ? timestamp2string(time / 1000) : '-'),
  },
  {
    title: t('操作'),
    key: 'action',
    width: 150,
    fixed: 'right',
    render: (_, record) => {
      const actions = [];
      // Manual complete button - for paid orders without user_plan (not yet delivered)
      if (record.status === 'paid' && !record.user_plan_id) {
        actions.push(
          <Button
            key='complete'
            size='small'
            type='warning'
            icon={<IconTickCircle />}
            onClick={() => handleManualComplete(record.order_id)}
          />
        );
      }
      // Cancel button - for pending or paid (not delivered) orders
      if (record.status === 'pending' || (record.status === 'paid' && !record.user_plan_id)) {
        actions.push(
          <Popconfirm
            key='cancel'
            title={t('确认取消订单')}
            content={t('确认要取消该订单吗？')}
            onConfirm={() => handleCancelOrder(record.order_id)}
          >
            <Button size='small' type='tertiary' icon={<IconClose />} />
          </Popconfirm>
        );
      }
      // Delete button - for delivered, expired or cancelled orders
      if (record.status === 'delivered' || record.status === 'expired' || record.status === 'cancelled') {
        actions.push(
          <Popconfirm
            key='delete'
            title={t('确认删除订单')}
            content={t('确认要删除该订单吗？此操作不可撤销。')}
            onConfirm={() => handleDeleteOrder(record.order_id)}
          >
            <Button size='small' type='danger' icon={<IconDelete />} />
          </Popconfirm>
        );
      }
      return <Space>{actions}</Space>;
    },
  },
];
```

**Step 2: 移除详情弹窗相关代码（可选保留）**

可以保留详情弹窗代码以备后用，或者完全移除。

**Step 3: 移除不再使用的 import**

移除 `IconEyeOpened` 的导入（如果不再使用）。

**Step 4: 提交**

```bash
git add web/src/pages/AdminOrders/index.jsx
git commit -m "refactor(frontend): simplify plan order columns, remove detail button"
```

---

## Task 10: 前端 - 更新菜单名称

**Files:**
- Modify: `web/src/components/layout/SiderBar.jsx:190`

**Step 1: 将"套餐订单"改为"订单管理"**

```jsx
{
  text: t('订单管理'),  // 从 '套餐订单' 改为 '订单管理'
  itemKey: 'plan-orders',
  to: '/console/admin/plan-orders',
  className: isAdmin() ? '' : 'tableHiddle',
},
```

**Step 2: 提交**

```bash
git add web/src/components/layout/SiderBar.jsx
git commit -m "feat(frontend): rename menu from '套餐订单' to '订单管理'"
```

---

## Task 11: 测试验证

**Step 1: 启动后端服务**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go run main.go`
Expected: 服务启动成功

**Step 2: 启动前端开发服务**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api/web && npm run dev`
Expected: 前端服务启动成功

**Step 3: 手动测试**

1. 登录管理员账号
2. 访问"订单管理"页面
3. 验证 Tab 切换功能
4. 验证套餐订单列表和展开行
5. 验证充值订单列表和展开行
6. 验证筛选功能
7. 验证操作按钮（取消、删除等）

**Step 4: 最终提交**

```bash
git add .
git commit -m "feat: unified order management page complete"
```

---

## 文件改动清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `model/topup_order.go` | 修改 | 添加 GetAllTopupOrders 函数 |
| `controller/admin_topup_order.go` | 新建 | 管理员充值订单控制器 |
| `router/api-router.go` | 修改 | 注册充值订单管理路由 `/api/user/topup-orders` |
| `web/src/pages/AdminOrders/index.jsx` | 修改 | 添加 Tab、可展开行、完整筛选功能（状态、支付方式、时间范围、金额范围、用户搜索） |
| `web/src/components/layout/SiderBar.jsx` | 修改 | 菜单名称改为"订单管理" |

## 筛选功能清单

| 筛选项 | 套餐订单 | 充值订单 |
|--------|----------|----------|
| 订单状态 | ✅ | ✅ |
| 支付方式 | ✅ | ✅ |
| 时间范围 | ✅ | ✅ |
| 金额范围 | ✅ | ✅ |
| 用户ID | ✅ | ✅ |
| 用户名 | ✅ | ✅ |
| 邮箱 | ✅ | ✅ |
| 订单号 | ✅ | ✅ |
