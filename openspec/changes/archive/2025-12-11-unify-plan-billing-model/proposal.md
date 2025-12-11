# Proposal: Unify Plan Billing Model

## Summary

统一套餐计费模型，将 API 消费从"双重扣费"（用户余额 + 套餐额度）改为"套餐优先"模式。同时改造兑换码系统使其与套餐关联，并为新用户自动绑定试用套餐。

## Why

当前系统存在以下问题：

1. **双重扣费**：API 消费同时扣减用户余额和套餐额度，增加复杂性
2. **兑换码与套餐无关**：兑换码直接增加用户余额，而非套餐额度
3. **新用户无套餐**：新用户注册后没有套餐，无法享受套餐功能

## Proposed Changes

### 1. 扣费逻辑改造

```
有套餐且额度充足 → 只扣套餐额度（不扣用户余额）
有套餐但额度不足 → 回退扣用户余额（向后兼容）
无套餐           → 扣用户余额（向后兼容）
```

### 2. 兑换码与套餐关联

- 兑换码增加 `plan_id` 字段
- 兑换时增加套餐额度（而非用户余额）
- `plan_id=0` 时保持旧逻辑（向后兼容）

### 3. 新用户自动绑定试用套餐

- 注册成功后自动绑定试用套餐
- 试用套餐配置：$2 额度，7天有效期
- 试用套餐不存在时跳过绑定，不阻塞注册

## Backward Compatibility

- 现有用户余额保留，作为回退机制
- 旧兑换码（`plan_id=0`）仍可用，增加用户余额
- 无套餐用户仍可使用用户余额

## Scope

- Backend: 扣费逻辑、兑换逻辑、注册逻辑
- Database: Redemption 表增加 plan_id、validity_days 字段
- Frontend: 兑换码管理界面（支持选择套餐）

## Dependencies

- 依赖已有的 `add-user-plan-system` 变更中的套餐系统

## What Changes

- **plan-billing**: ADDED - New spec defining plan-aware billing logic
- **pre-consume-quota**: ADDED - New spec for consumption validation before deduction
- **redemption-plan-binding**: ADDED - New spec for plan-associated redemption codes
- **user-registration-plan**: ADDED - New spec for automatic trial plan binding on registration

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| 试用套餐不存在导致注册失败 | 跳过绑定，不阻塞注册 |
| 兑换码对应套餐不存在 | 兑换失败，返回明确错误 |
| 数据库迁移失败 | 新增字段使用默认值，不影响现有数据 |
