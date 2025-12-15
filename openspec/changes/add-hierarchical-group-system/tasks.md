## 1. 后端 - GroupTree 配置管理

- [x] 1.1 在 `setting/ratio_setting/group_ratio.go` 新增 GroupTree 数据结构和读写锁
- [x] 1.2 实现 GroupTree 辅助函数
  - [x] `IsParentGroup(group string) bool`
  - [x] `GetChildGroups(parentGroup string) []string`
  - [x] `GetParentGroup(childGroup string) string`
  - [x] `ExpandGroup(group string) []string`
  - [x] `GetAllParentGroups() []string`
  - [x] `GetAllChildGroups() []string`
- [x] 1.3 实现 GroupTree 的 JSON 序列化/反序列化
- [x] 1.4 在 `model/option.go` 添加 GroupTree 配置的读写支持
- [x] 1.5 添加 GroupTree 配置校验逻辑（检查循环引用、重复定义等）

## 2. 后端 - 交集逻辑修改

- [x] 2.1 修改 `middleware/distributor.go` 的交集验证逻辑
  - [x] 检测令牌分组是否为父级
  - [x] 父级时展开为子级列表再取交集
  - [x] 子级时保持原有逻辑
- [x] 2.2 修改 `service/plan_failover.go` 同步交集逻辑
- [x] 2.3 修复钱包充值用户的父级分组展开支持
  - [x] 在 `middleware/distributor.go:85-97` 添加父级展开逻辑
  - [x] 钱包用户令牌使用父级分组时自动展开为子级分组
  - [x] 与套餐用户使用相同的多分组选择逻辑
- [ ] 2.4 添加单元测试验证各种交集场景

## 3. 后端 - 渠道分组处理

- [x] 3.1 修改 `model/channel.go` 的 `GetGroups()` 方法
  - [x] 支持父级分组自动展开为所有子级
  - [x] 保持向后兼容（非父级分组不变）
- [x] 3.2 修改 `model/channel_cache.go` 的 `InitChannelCache()` 方法
  - [x] 渠道分组展开后再构建缓存
- [ ] 3.3 添加渠道分组展开的单元测试

## 3.5. 后端 - 倍率回退逻辑

- [x] 3.5.1 修改 `setting/ratio_setting/group_ratio.go` 的 `GetGroupRatio()` 方法
  - [x] 先查子级倍率
  - [x] 没有则回退到父级倍率
  - [x] 最后返回默认值 1.0
- [x] 3.5.2 修改 `setting/ratio_setting/group_ratio.go` 的 `GetGroupGroupRatio()` 方法
  - [x] 先查子级的模型倍率
  - [x] 没有则回退到父级的模型倍率
  - [x] 最后调用 GetGroupRatio() 获取分组整体倍率
- [ ] 3.5.3 添加倍率回退的单元测试（覆盖 GroupRatio 和 GroupGroupRatio）

## 3.6. 后端 - 缓存自动刷新

- [x] 3.6.1 在 `UpdateGroupTreeByJSONString()` 成功后自动调用 `InitChannelCache()`
- [x] 3.6.2 添加日志记录缓存刷新事件

## 4. 后端 - API 接口

- [x] 4.1 在 `controller/group.go` 新增 GroupTree 相关 API
  - [x] `GET /api/group/tree` - 获取 GroupTree 配置
  - [x] `GET /api/group/selectable` - 获取可选分组（用于令牌创建）
  - [x] `GET /api/group/all` - 获取所有分组及层级信息
- [x] 4.2 修改 `GetUserGroups()` 接口，自动过滤只返回父级分组和独立分组
- [x] 4.3 在 `router/api_router.go` 注册新路由
- [x] 4.4 添加 API 权限控制（仅管理员可修改）

## 5. 前端 - 系统设置

- [x] 5.1 创建 GroupTree 配置组件
  - [x] JSON 文本区编辑（支持树形结构配置）
  - [x] 配置校验
- [x] 5.2 在系统设置页面添加 GroupTree 配置入口
- [x] 5.3 实现配置保存和校验提示

## 6. 前端 - 令牌创建

- [x] 6.1 修改令牌创建页面的分组选择组件
- [x] 6.2 分组下拉框只显示父级分组和独立分组（过滤子级分组）
- [x] 6.3 添加分组用途说明（父级分组标签和子级分组列表）
- [x] 6.4 保持向后兼容（无 GroupTree 时显示所有分组）

## 7. 前端 - 渠道配置

- [x] 7.1 修改渠道配置页面的分组选择组件
- [x] 7.2 显示分组层级信息（父级分组显示包含的子级）
- [x] 7.3 选择父级时显示"将归属所有子级"提示
- [x] 7.4 保持向后兼容

## 8. 前端 - 套餐配置

- [x] 8.1 修改套餐配置页面的分组选择组件
- [x] 8.2 分组选择显示层级关系
- [x] 8.3 按父级分组归类显示
- [x] 8.4 保持向后兼容

## 9. 国际化

- [x] 9.1 添加中文翻译
- [x] 9.2 添加英文翻译
- [ ] 9.3 添加其他语言翻译（如有需要）

## 10. 测试和文档

- [ ] 10.1 编写后端单元测试
- [ ] 10.2 编写 API 集成测试
- [ ] 10.3 手动测试各种场景
- [ ] 10.4 更新用户文档（如有）

## 11. 数据迁移

- [ ] 11.1 编写迁移检查脚本
  - [ ] 检查现有套餐是否配置了即将成为父级的分组
  - [ ] 输出需要迁移的套餐列表
- [ ] 11.2 上线前手动迁移数据
  - [ ] 将套餐中的父级分组名称改为对应的子级分组
  - [ ] 验证迁移后的套餐权限正确
- [ ] 11.3 文档记录迁移步骤（供后续参考）
