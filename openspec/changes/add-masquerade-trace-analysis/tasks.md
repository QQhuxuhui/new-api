## 1. 后端采集与存储

- [ ] 1.1 新增进程内 ring-buffer（最多 100 条）用于保存 Claude 伪装对照数据
- [ ] 1.2 在 `relay/channel/api_request.go` 的上游请求构造完成后写入记录（仅 Claude 渠道）
- [ ] 1.3 记录字段包含：时间、request_id、用户/令牌/渠道信息、上游 URL、before/after headers、before/after body

## 2. Admin API

- [ ] 2.1 在 `/api/admin/analytics` 下新增查询接口：获取最近 100 条（按时间倒序）
- [ ] 2.2 新增清空接口：清空 ring-buffer

## 3. 前端（数据分析页面 Tab）

- [ ] 3.1 在 `web/src/pages/Analytics/index.jsx` 增加 Tab：**防封分析**
- [ ] 3.2 新增组件实现：列表 + 详情对比（headers/body）差异标红
- [ ] 3.3 支持刷新与清空

## 4. 测试与验证

- [ ] 4.1 单元测试：ring-buffer 并发安全、容量覆盖行为
- [ ] 4.2 手动验证：发送 Claude 请求后，控制台可看到记录并准确标红差异

