## 1. 数据模型层

- [x] 1.1 在 `model/user.go` 的 User 结构体中添加 `MaxConcurrency` 字段
- [ ] 1.2 运行数据库迁移确保字段生效
- [x] 1.3 更新 `User.Edit()` 方法支持更新 `MaxConcurrency` 字段

## 2. Redis 并发计数器

- [x] 2.1 在 `model/user_cache.go` 中添加用户并发计数相关函数
  - `IncrUserConcurrency(userId int) (int64, error)` - 增加并发计数
  - `DecrUserConcurrency(userId int) error` - 减少并发计数
  - `GetUserConcurrency(userId int) (int64, error)` - 获取当前并发数
- [x] 2.2 设置计数器 TTL（5分钟）防止孤立计数器

## 3. 并发限制中间件

- [x] 3.1 在 `relay/controller/relay.go` 的请求入口处添加用户并发检查逻辑
- [x] 3.2 实现并发限制检查函数 `checkUserConcurrencyLimit(userId, maxConcurrency int) error`
- [x] 3.3 在请求完成时（成功或失败）确保减少并发计数（使用 defer）
- [x] 3.4 超出限制时返回 HTTP 429 错误，包含清晰的错误信息

## 4. 用户 API 更新

- [x] 4.1 更新 `controller/user.go` 的用户更新接口支持 `max_concurrency` 字段
- [x] 4.2 确保只有管理员可以修改用户的并发限制

## 5. 前端用户编辑界面

- [x] 5.1 在 `web/src/components/table/users/modals/EditUserModal.jsx` 添加"最大并发数"输入字段
- [x] 5.2 字段放置在"权限设置"卡片中，与额度字段同级
- [x] 5.3 添加字段说明提示（0 表示不限制）
- [x] 5.4 确保表单提交时包含 `max_concurrency` 字段

## 6. 测试验证

- [ ] 6.1 手动测试：设置用户并发限制为 2，发起 3 个并发请求，验证第 3 个被拒绝
- [ ] 6.2 验证并发计数器在请求完成后正确递减
- [ ] 6.3 验证 Redis 计数器 TTL 正常工作
- [ ] 6.4 验证前端编辑界面正常显示和保存
