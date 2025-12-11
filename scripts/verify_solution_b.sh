#!/bin/bash

# PlanOrder 方案 B 验证脚本
# 验证允许删除只有已完成订单的套餐的完整实现

echo "=== PlanOrder Solution B Verification ==="
echo

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

PASS=0
FAIL=0

# 1. 检查 PlanOrder.PlanId 是否为可空指针
echo "1. Checking PlanOrder.PlanId is nullable pointer..."
if grep -q 'PlanId.*\*int.*json:"plan_id".*gorm:"index"' model/plan_order.go; then
    echo -e "${GREEN}✓${NC} PlanId is *int (nullable)"
    PASS=$((PASS+1))
else
    echo -e "${RED}✗${NC} PlanId is not nullable pointer"
    FAIL=$((FAIL+1))
fi

# 2. 检查 CreatePlanOrder 使用指针
echo "2. Checking CreatePlanOrder uses pointer..."
if grep -q 'PlanId:.*&planIdPtr' model/plan_order.go; then
    echo -e "${GREEN}✓${NC} CreatePlanOrder assigns pointer correctly"
    PASS=$((PASS+1))
else
    echo -e "${RED}✗${NC} CreatePlanOrder doesn't use pointer"
    FAIL=$((FAIL+1))
fi

# 3. 检查 DeliverPlan 处理 NULL
echo "3. Checking DeliverPlan handles NULL..."
if grep -q 'if order.PlanId == nil' service/plan_delivery.go && \
   grep -q '\*order.PlanId' service/plan_delivery.go; then
    echo -e "${GREEN}✓${NC} DeliverPlan has NULL check"
    PASS=$((PASS+1))
else
    echo -e "${RED}✗${NC} DeliverPlan missing NULL handling"
    FAIL=$((FAIL+1))
fi

# 4. 检查控制器处理 NULL
echo "4. Checking controllers handle NULL..."
if grep -q 'order.PlanId != nil' controller/plan_purchase.go; then
    echo -e "${GREEN}✓${NC} Controllers check for NULL"
    PASS=$((PASS+1))
else
    echo -e "${RED}✗${NC} Controllers missing NULL checks"
    FAIL=$((FAIL+1))
fi

# 5. 检查外键迁移为 SET NULL
echo "5. Checking foreign key migration..."
if grep -q 'ON DELETE SET NULL' migrations/update_plan_order_fk.sql; then
    echo -e "${GREEN}✓${NC} Foreign key uses ON DELETE SET NULL"
    PASS=$((PASS+1))
else
    echo -e "${RED}✗${NC} Foreign key not SET NULL"
    FAIL=$((FAIL+1))
fi

# 6. 检查快照迁移允许 NULL
echo "6. Checking snapshot migration allows NULL..."
if grep -q 'ALTER COLUMN plan_id DROP NOT NULL' migrations/add_plan_order_snapshots.sql; then
    echo -e "${GREEN}✓${NC} Migration drops NOT NULL constraint"
    PASS=$((PASS+1))
else
    echo -e "${RED}✗${NC} Migration doesn't allow NULL"
    FAIL=$((FAIL+1))
fi

# 7. 检查 Plan.Delete() 逻辑
echo "7. Checking Plan.Delete() logic..."
if grep -q 'OrderStatusPending.*OrderStatusPaid' model/plan.go && \
   grep -q '该套餐有未完成订单' model/plan.go; then
    echo -e "${GREEN}✓${NC} Plan.Delete() checks unfinished orders"
    PASS=$((PASS+1))
else
    echo -e "${RED}✗${NC} Plan.Delete() logic incorrect"
    FAIL=$((FAIL+1))
fi

# 8. 检查快照显示逻辑
echo "8. Checking snapshot display logic..."
if grep -q 'order.PlanDisplayName.*!=' controller/plan_purchase.go && \
   grep -q 'order.PlanDisplayName.*!=' controller/admin_plan_order.go; then
    echo -e "${GREEN}✓${NC} Display logic prefers snapshots"
    PASS=$((PASS+1))
else
    echo -e "${RED}✗${NC} Display logic incorrect"
    FAIL=$((FAIL+1))
fi

# 9. 检查注释准确性
echo "9. Checking code comments..."
if grep -q 'ON DELETE SET NULL' model/plan.go; then
    echo -e "${GREEN}✓${NC} Comments mention SET NULL"
    PASS=$((PASS+1))
else
    echo -e "${YELLOW}!${NC} Comments may need updating"
fi

echo
echo "=== Results ==="
echo -e "${GREEN}Pass: $PASS${NC}"
echo -e "${RED}Fail: $FAIL${NC}"
echo

if [ $FAIL -eq 0 ]; then
    echo -e "${GREEN}=== All checks passed! ===${NC}"
    echo
    echo "Next steps:"
    echo "1. Backup database"
    echo "2. Run migrations in order:"
    echo "   a) migrations/add_plan_order_snapshots.sql"
    echo "   b) migrations/update_plan_order_fk.sql"
    echo "3. Restart application"
    echo "4. Test deleting plans with only completed orders"
    exit 0
else
    echo -e "${RED}=== Some checks failed ===${NC}"
    exit 1
fi
