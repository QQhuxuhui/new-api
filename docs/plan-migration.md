# 套餐系统迁移指南

本文档描述如何将现有用户迁移到新的套餐系统。

## 概述

套餐系统默认关闭（`PlanSystemEnabled = false`），允许渐进式迁移。迁移过程包括：

1. 创建"遗留套餐"（legacy plan）
2. 为现有用户分配套餐，保留原有额度
3. 启用套餐系统

## 迁移步骤

### 步骤 1：检查迁移状态

```bash
# 调用 API 检查当前状态
curl -X GET "http://your-api/api/plan_migration/status" \
  -H "Authorization: Bearer YOUR_ROOT_TOKEN"
```

返回示例：
```json
{
  "success": true,
  "data": {
    "legacy_plan_exists": false,
    "legacy_plan_id": 0,
    "total_users": 1000,
    "users_with_plans": 0,
    "users_without_plans": 1000,
    "plan_system_enabled": false
  }
}
```

### 步骤 2：模拟运行迁移（Dry Run）

在执行实际迁移前，建议先进行模拟运行：

```bash
curl -X POST "http://your-api/api/plan_migration/run" \
  -H "Authorization: Bearer YOUR_ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"dry_run": true}'
```

### 步骤 3：执行迁移

确认模拟运行结果无误后，执行实际迁移：

```bash
curl -X POST "http://your-api/api/plan_migration/run" \
  -H "Authorization: Bearer YOUR_ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"dry_run": false}'
```

返回示例：
```json
{
  "success": true,
  "message": "迁移完成",
  "data": {
    "total_users": 1000,
    "migrated_users": 1000,
    "skipped_users": 0,
    "failed_users": 0,
    "plan_created": true,
    "plan_id": 1,
    "errors": []
  }
}
```

### 步骤 4：启用套餐系统

在管理后台的系统设置中，启用 `PlanSystemEnabled` 选项，或通过 API：

```bash
curl -X PUT "http://your-api/api/option/" \
  -H "Authorization: Bearer YOUR_ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"key": "PlanSystemEnabled", "value": "true"}'
```

### 步骤 5：验证

1. 检查迁移状态确认所有用户已迁移
2. 测试用户登录并查看"我的套餐"页面
3. 测试 API 请求是否正常消费套餐额度

## 回滚步骤

如需回滚迁移：

### 步骤 1：关闭套餐系统

```bash
curl -X PUT "http://your-api/api/option/" \
  -H "Authorization: Bearer YOUR_ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"key": "PlanSystemEnabled", "value": "false"}'
```

### 步骤 2：模拟回滚

```bash
curl -X POST "http://your-api/api/plan_migration/rollback" \
  -H "Authorization: Bearer YOUR_ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"dry_run": true}'
```

### 步骤 3：执行回滚

```bash
curl -X POST "http://your-api/api/plan_migration/rollback" \
  -H "Authorization: Bearer YOUR_ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"dry_run": false}'
```

**警告**：回滚将删除所有与遗留套餐相关的用户套餐数据。用户的原始额度（User.Quota）不受影响。

## 单用户迁移

如需迁移单个用户：

```bash
curl -X POST "http://your-api/api/plan_migration/user" \
  -H "Authorization: Bearer YOUR_ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id": 123}'
```

## API 端点参考

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/plan_migration/status` | 获取迁移状态 |
| POST | `/api/plan_migration/run` | 执行批量迁移 |
| POST | `/api/plan_migration/rollback` | 回滚迁移 |
| POST | `/api/plan_migration/user` | 迁移单个用户 |

## 注意事项

1. **权限要求**：所有迁移 API 仅限 Root 管理员访问
2. **备份建议**：执行迁移前建议备份数据库
3. **渐进式迁移**：可以先迁移少量用户测试，确认无误后再批量迁移
4. **额度保留**：迁移会将用户当前的 `Quota` 和 `UsedQuota` 转移到 `user_plans` 表
5. **功能开关**：在 `PlanSystemEnabled = false` 时，系统仍使用用户原有的额度机制

## 遗留套餐说明

迁移创建的"遗留套餐"具有以下特性：

- 名称：`legacy`
- 类型：`consumption`（按量付费）
- 优先级：`0`（最低）
- 有效期：永久
- 渠道组：空（使用默认渠道组）
- 用户权限：允许手动切换，允许控制自动切换

管理员可以在套餐管理页面修改遗留套餐的配置。
