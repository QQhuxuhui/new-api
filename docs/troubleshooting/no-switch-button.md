# 套餐切换按钮不显示问题排查指南

## 问题描述
用户在"我的套餐"页面看不到切换按钮。

## 切换按钮显示的必要条件

前端代码要求**同时满足**以下所有条件才会显示切换按钮：

```javascript
{!isCurrent && canSwitch && !isLocked && !isQueued && (
  <Button>切换到此套餐</Button>
)}
```

| 条件 | 前端字段 | 后端字段 | 说明 |
|------|---------|---------|------|
| ✅ 不是当前套餐 | `is_current !== 1` | `is_current` | 当前正在使用的套餐不显示切换按钮 |
| ✅ 允许用户切换 | `can_switch === 1` | `allow_user_switch` | **最常见原因** |
| ✅ 未被锁定 | `locked !== 1` | `locked` | 被锁定的套餐不能切换 |
| ✅ 不在队列中 | `queue_position === 0` | `queue_position` | 队列中的套餐不能手动切换 |

## 排查步骤

### 步骤1：检查数据库

执行以下SQL查询（替换 `YOUR_USER_ID` 为你的用户ID）：

```sql
SELECT
    id,
    plan_display_name,
    is_current,
    allow_user_switch,  -- 重点检查这个字段
    locked,
    queue_position,
    status,
    quota
FROM user_plans
WHERE user_id = YOUR_USER_ID
ORDER BY is_current DESC, plan_priority DESC;
```

### 步骤2：分析结果

#### 情况A：`allow_user_switch = 0`（最常见）

**原因：** 套餐创建时继承了模板的 `default_allow_switch = 0`，或管理员手动设置为不允许切换。

**解决方案：**

1. **通过管理后台修改**（推荐）：
   - 登录管理后台
   - 进入"用户管理" → 找到对应用户
   - 点击"套餐管理"
   - 找到对应套餐，开启"允许用户切换"开关

2. **通过SQL修改**（需要数据库权限）：
   ```sql
   -- 为特定用户的特定套餐开启切换权限
   UPDATE user_plans
   SET allow_user_switch = 1
   WHERE id = <套餐ID>;

   -- 或批量为某用户的所有非当前套餐开启
   UPDATE user_plans
   SET allow_user_switch = 1
   WHERE user_id = <用户ID>
     AND is_current = 0
     AND locked = 0
     AND queue_position = 0;
   ```

3. **通过API修改**（管理员权限）：
   ```bash
   curl -X PUT 'http://your-domain/api/user_plan/<user_plan_id>/permissions' \
     -H 'Authorization: Bearer <admin_token>' \
     -H 'Content-Type: application/json' \
     -d '{"can_switch": 1}'
   ```

#### 情况B：`locked = 1`

**原因：** 套餐被管理员锁定。

**解决方案：**
- 联系管理员解锁
- 或通过管理后台解锁

#### 情况C：`queue_position > 0`

**原因：** 套餐在队列中，等待自动激活。

**说明：** 这是正常行为，队列中的套餐不能手动切换，会在当前套餐耗尽后自动激活。

#### 情况D：`is_current = 1`

**原因：** 这是当前正在使用的套餐。

**说明：** 这是正常行为，当前套餐不显示切换按钮。

### 步骤3：检查套餐模板默认设置

如果新分配的套餐总是不能切换，检查套餐模板：

```sql
SELECT
    id,
    name,
    display_name,
    default_allow_switch  -- 应该为 1
FROM plans
WHERE id = <套餐模板ID>;
```

如果 `default_allow_switch = 0`，修改模板：

```sql
UPDATE plans
SET default_allow_switch = 1
WHERE id = <套餐模板ID>;
```

**注意：** 修改模板只影响新分配的套餐，已存在的套餐需要单独修改。

## 前端显示逻辑

### 可以切换时
```
┌─────────────────────────────────┐
│  套餐名称                        │
│  [切换到此套餐] 按钮（蓝色）     │
└─────────────────────────────────┘
```

### 不能切换时的提示

| 条件 | 显示内容 |
|------|---------|
| `!canSwitch` | 灰色标签："暂不可手动切换" |
| `isQueued` | 蓝色标签："排队中 #N" |
| `isLocked` | 红色标签："套餐锁定中" |
| `isCurrent` | 蓝色标签："当前使用" |

## 常见问题

### Q1: 为什么我的套餐默认不允许切换？

**A:** 这取决于套餐模板的 `default_allow_switch` 设置。管理员可能出于以下原因禁用切换：
- 防止用户频繁切换影响系统
- 某些特殊套餐需要管理员控制
- 试用套餐限制

### Q2: 我修改了数据库，但前端还是不显示？

**A:** 需要清除缓存：
1. 后端缓存会在套餐修改时自动清除
2. 前端需要刷新页面（F5）
3. 如果还不行，清除浏览器缓存

### Q3: 管理员可以强制切换吗？

**A:** 可以。管理员有专门的强制切换API：
```
POST /api/user_plan/force_switch
{
  "user_id": 123,
  "user_plan_id": 456
}
```

### Q4: 如何批量开启所有用户的切换权限？

**A:** 谨慎执行以下SQL：
```sql
-- 为所有非当前、未锁定、不在队列的套餐开启切换权限
UPDATE user_plans
SET allow_user_switch = 1
WHERE is_current = 0
  AND locked = 0
  AND queue_position = 0
  AND status = 1;
```

## 相关代码位置

- 前端切换按钮逻辑：`web/src/pages/MyPlans/index.jsx` 第 653-673 行
- 后端字段映射：`controller/user_plan.go` 第 659 行
- 后端权限检查：`service/plan_selector.go` `UserSwitchPlanByUserPlanId` 函数
- 数据库表：`user_plans` 表的 `allow_user_switch` 字段

## 快速修复脚本

如果确认是权限问题，可以使用以下脚本快速修复：

```bash
#!/bin/bash
# fix_switch_permission.sh

USER_ID=$1
if [ -z "$USER_ID" ]; then
    echo "用法: ./fix_switch_permission.sh <用户ID>"
    exit 1
fi

mysql -u root -p your_database << EOF
UPDATE user_plans
SET allow_user_switch = 1
WHERE user_id = $USER_ID
  AND is_current = 0
  AND locked = 0
  AND queue_position = 0
  AND status = 1;

SELECT
    id,
    plan_display_name,
    allow_user_switch,
    '✅ 已修复' as 状态
FROM user_plans
WHERE user_id = $USER_ID;
EOF

echo "修复完成！请刷新页面查看。"
```

## 总结

**最常见原因：** `allow_user_switch = 0`

**最快解决方案：** 通过管理后台或SQL将该字段改为 `1`

**预防措施：** 在套餐模板中设置 `default_allow_switch = 1`
