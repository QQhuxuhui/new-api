# PlanOrder 数据库迁移指南（方案 B - 最终版）

## ✅ 当前方案：允许删除只有已完成订单的套餐

**方案选择**：方案 B - 平衡灵活性与安全性

---

## 概述

本目录包含两个数据库迁移脚本，实现了允许删除只有已完成订单的套餐功能：

1. `add_plan_order_snapshots.sql` - 添加快照字段并将 plan_id 改为可空
2. `update_plan_order_fk.sql` - 更新外键约束为 ON DELETE SET NULL

## 业务逻辑

### 套餐删除规则

| 订单状态 | 是否允许删除套餐 | 原因 |
|---------|----------------|------|
| ❌ pending（待支付） | **禁止** | 需要套餐配置，可能需要发货 |
| ❌ paid（已支付待发货） | **禁止** | 需要套餐配置完成发货 |
| ✅ delivered（已完成） | **允许** | 使用快照显示，plan_id 变 NULL |
| ✅ expired（已过期） | **允许** | 使用快照显示，plan_id 变 NULL |
| ✅ cancelled（已取消） | **允许** | 使用快照显示，plan_id 变 NULL |

### 保护机制

**双重保护**：
1. **应用层**：Plan.Delete() 检查是否有 pending/paid 订单
2. **数据库层**：外键 ON DELETE SET NULL（已完成订单 plan_id 变 NULL）

### 快照机制

在订单创建时保存套餐信息：
- **价格快照**（已有）：`plan_price`, `plan_original_price`, `final_price`
- **名称快照**（新增）：`plan_name`, `plan_display_name`

**作用**：
- 已完成订单显示使用快照，不依赖 Plan 表
- 即使套餐被删除（plan_id 为 NULL），仍能看到完整信息

---

## 执行顺序

**⚠️ 必须按以下顺序执行迁移：**

### 1. 首先执行：添加快照字段并允许 plan_id 为 NULL

```bash
# Docker 环境
docker exec -i postgres psql -U root -d new-api < migrations/add_plan_order_snapshots.sql

# 或远程数据库
PGPASSWORD=your_password psql -h host -U root -d new-api -f migrations/add_plan_order_snapshots.sql
```

**此步骤会**：
- 将 plan_id 列改为允许 NULL
- 添加 plan_name 和 plan_display_name 列
- 回填现有订单的套餐名称

### 2. 然后执行：更新外键约束为 SET NULL

```bash
# Docker 环境
docker exec -i postgres psql -U root -d new-api < migrations/update_plan_order_fk.sql

# 或远程数据库
PGPASSWORD=your_password psql -h host -U root -d new-api -f migrations/update_plan_order_fk.sql
```

**此步骤会**：
- 更新外键约束为 ON DELETE SET NULL
- 允许删除有已完成订单的套餐

---

## 验证迁移结果

### 验证快照字段和 plan_id 可空

```sql
-- 检查字段是否正确
SELECT
    COUNT(*) as total_orders,
    COUNT(plan_name) as orders_with_name,
    COUNT(plan_display_name) as orders_with_display_name,
    COUNT(plan_id) as orders_with_plan_id
FROM plan_orders;
```

### 验证外键约束

```sql
-- 检查外键约束
SELECT constraint_name, delete_rule, update_rule
FROM information_schema.referential_constraints
WHERE constraint_name = 'fk_plan_orders_plan';

-- 期望结果：
-- delete_rule = 'SET NULL'
-- update_rule = 'CASCADE'
```

### 测试删除功能

```sql
-- 场景 1：尝试删除有未完成订单的套餐（应该被应用层阻止）
-- 1. 查看套餐的订单状态分布
SELECT status, COUNT(*)
FROM plan_orders
WHERE plan_id = 1
GROUP BY status;

-- 2. 如果有 pending 或 paid，尝试通过应用删除（会被阻止）
-- 应用层会返回错误："该套餐有未完成订单，无法删除"

-- 场景 2：删除只有已完成订单的套餐（应该成功）
-- 1. 确认所有订单都已完成
SELECT status, COUNT(*)
FROM plan_orders
WHERE plan_id = 2
GROUP BY status;
-- 应该只有 'delivered', 'expired', 或 'cancelled'

-- 2. 删除套餐（应该成功）
DELETE FROM plans WHERE id = 2;

-- 3. 验证订单 plan_id 变为 NULL 但快照仍存在
SELECT id, order_no, plan_id, plan_name, plan_display_name, status
FROM plan_orders
WHERE plan_name = '被删除套餐的名称';
-- plan_id 应该为 NULL
-- plan_name 和 plan_display_name 应该仍有值
```

---

## 回滚方案

### 回滚到不允许删除任何有订单的套餐

```sql
-- 1. 恢复 plan_id 为非空（需要先填充所有 NULL）
-- 注意：如果已经有 plan_id 为 NULL 的订单，需要先处理

-- 2. 修改外键为 RESTRICT
ALTER TABLE plan_orders DROP CONSTRAINT IF EXISTS fk_plan_orders_plan;
ALTER TABLE plan_orders
ADD CONSTRAINT fk_plan_orders_plan
FOREIGN KEY (plan_id) REFERENCES plans(id)
ON DELETE RESTRICT;

-- 3. 可选：删除快照字段（不推荐，因为快照有价值）
-- ALTER TABLE plan_orders
-- DROP COLUMN IF EXISTS plan_name,
-- DROP COLUMN IF EXISTS plan_display_name;
```

---

## 注意事项

1. **备份数据库**：执行迁移前务必备份
2. **按顺序执行**：必须先允许 NULL，再修改外键
3. **应用重启**：迁移后需要重启应用（Go 结构体已更新为 *int）
4. **GORM 兼容**：模型定义与迁移脚本一致，AutoMigrate 安全
5. **测试删除**：在测试环境先验证删除逻辑

---

## 常见问题

### Q1: 为什么选择方案 B？

**A**: 方案 B 在灵活性和安全性之间取得平衡：
- ✅ 可以清理只有历史订单的旧套餐
- ✅ 保护未完成订单不受影响（应用层检查）
- ✅ 已完成订单仍能显示完整信息（快照）
- ✅ 不影响发货流程（DeliverPlan 有 NULL 检查）

### Q2: plan_id 为 NULL 会不会影响发货？

**A**: 不会，因为：
1. DeliverPlan 只在订单状态为 `paid` 时被调用
2. Plan.Delete() 禁止删除有 `paid` 订单的套餐
3. 所以 DeliverPlan 执行时，plan_id 必然有效
4. 即使 DeliverPlan 被错误调用，也有 NULL 检查保护

### Q3: 删除套餐后订单显示什么？

**A**: 显示快照信息：
```json
{
  "order_id": 123,
  "plan_id": null,                    // ← 变为 NULL
  "plan_name": "月度会员",            // ← 快照保留
  "plan_display_name": "月度会员套餐", // ← 快照保留
  "status": "delivered",
  "final_price": 99.00
}
```

### Q4: 如何手动处理未完成订单？

**A**: 两种方法：
1. **完成订单**：管理员手动完成已支付订单（触发发货）
2. **取消订单**：取消待支付订单

完成后即可删除套餐。

### Q5: GORM AutoMigrate 会不会冲突？

**A**: 不会，因为：
- 模型定义：`PlanId *int` + `gorm:"constraint:OnDelete:SET NULL"`
- 迁移脚本：`ON DELETE SET NULL`
- 完全一致 ✅

---

## 架构优势

### 数据完整性

| 方面 | 实现 |
|-----|------|
| 未完成订单保护 | ✅ 应用层检查 |
| 数据库一致性 | ✅ 外键约束 |
| 历史数据保留 | ✅ 快照机制 |
| 发货流程安全 | ✅ NULL 检查 |

### 与其他方案对比

| 特性 | 方案 A（禁止删除） | 方案 B（当前） |
|-----|------------------|--------------|
| 清理历史套餐 | ❌ 不允许 | ✅ 允许 |
| 未完成订单保护 | ✅ 数据库阻止 | ✅ 应用层阻止 |
| 实现复杂度 | 🟢 简单 | 🟡 中等 |
| 灵活性 | 🟡 低 | ✅ 高 |

---

## 业务价值

1. **运营灵活性** ✅
   - 可以清理不再使用的套餐模板
   - 减少套餐表的冗余数据

2. **数据完整性** ✅
   - 未完成订单得到保护
   - 历史订单完整可查

3. **发货可靠性** ✅
   - 发货流程不受影响
   - 双重保护机制

4. **用户体验** ✅
   - 历史订单始终可见
   - 显示完整的购买信息

---

## 验证清单

运行自动化验证：
```bash
bash scripts/verify_solution_b.sh
```

预期输出：
```
=== PlanOrder Solution B Verification ===
✓ PlanId is *int (nullable)
✓ CreatePlanOrder assigns pointer correctly
✓ DeliverPlan has NULL check
✓ Controllers check for NULL
✓ Foreign key uses ON DELETE SET NULL
✓ Migration drops NOT NULL constraint
✓ Plan.Delete() checks unfinished orders
✓ Display logic prefers snapshots
✓ Comments mention SET NULL

=== All checks passed! ===
```

---

## 支持

如有问题，请联系开发团队或查看 `docs/plan_order_solution_b.md` 了解详细设计。
