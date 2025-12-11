## 1. 数据库迁移
- [x] 1.1 为 `channels` 表添加 `ratio` 字段 (`DECIMAL(10,4) DEFAULT 1.0000`) - 通过 GORM AutoMigrate 自动完成
- [x] 1.2 添加字段约束检查 - 通过 GetRatio() 方法处理 nil 值，前端 min=0

## 2. 后端模型层
- [x] 2.1 更新 `model/channel.go`：Channel 结构体添加 Ratio 字段
- [x] 2.2 添加 `GetRatio()` 方法，处理 nil 值返回默认 1.0
- [x] 2.3 更新渠道 CRUD 操作支持 Ratio 字段 - GORM 自动处理

## 3. 计费逻辑
- [x] 3.1 修改 `relay/helper/price.go`：ModelPriceHelper 函数增加渠道倍率计算
- [x] 3.2 更新 `types/price_data.go`：PriceData 结构体添加 ChannelRatio 字段
- [x] 3.3 确保倍率模式和价格模式都正确应用渠道倍率

## 4. 重试费用处理
- [x] 4.1 修改 `middleware/distributor.go`：SetupContextForSelectedChannel 设置渠道倍率到 Context
- [x] 4.2 所有 post-consume 函数从 Context 获取最新渠道倍率（支持重试场景）
- [x] 4.3 实现按新渠道重新计算并扣费逻辑 - 通过 Context 获取当前渠道倍率
- [x] 4.4 添加重试费用变化的日志记录 - 渠道倍率记录到 admin_info

## 5. 账单展示优化
- [x] 5.1 检查账单 API 响应，确保不包含渠道倍率字段 - 渠道倍率仅在 admin_info 中
- [x] 5.2 确保日志记录表只记录总扣费，渠道倍率仅管理员可见

## 6. 前端实现
- [x] 6.1 渠道编辑页面添加"倍率"输入字段
- [x] 6.2 添加倍率输入验证（min=0, step=0.1）
- [x] 6.3 添加倍率字段的帮助提示文案
- [x] 6.4 渠道列表页面添加倍率列显示
- [x] 6.5 确保定价页面不展示渠道倍率信息 - 无需修改，渠道倍率不在定价 API 中

## 7. 测试验证
- [ ] 7.1 单元测试：计费公式正确性
- [ ] 7.2 单元测试：重试费用处理
- [ ] 7.3 集成测试：不同倍率渠道的实际扣费
- [ ] 7.4 手动测试：前端渠道倍率配置
- [ ] 7.5 手动测试：用户端定价和账单页面不显示渠道倍率

## 8. 文档更新
- [x] 8.1 更新 API 文档（如有）- 无需单独文档
- [x] 8.2 更新管理员操作手册（如有）- 无需单独文档
