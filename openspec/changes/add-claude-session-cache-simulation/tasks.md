## 1. 文档与规范

- [x] 1.1 新增 `docs/plans/2026-03-18-claude-session-cache-simulation-design.md`
- [x] 1.2 新增 `docs/plans/2026-03-18-claude-session-cache-simulation-implementation.md`
- [x] 1.3 新增 OpenSpec change：`add-claude-session-cache-simulation`
- [x] 1.4 运行 `openspec validate add-claude-session-cache-simulation --strict`

## 2. 独立算法模块

- [x] 2.1 新增 `internal/cachesim/types.go` 定义 snapshot / segment / result / store 接口
- [x] 2.2 新增 `internal/cachesim/memory_store.go` 实现 TTL + LRU 的内存状态存储
- [x] 2.3 新增 `internal/cachesim/engine.go` 实现 session-prefix 前缀命中算法
- [x] 2.4 为 `internal/cachesim` 添加首轮写入、5m 过期、1h 保留、作用域隔离测试

## 3. Claude 接入

- [x] 3.1 新增 `internal/cachesim/claude_adapter.go` 构建 Claude `PromptSnapshot`
- [x] 3.2 新增 `internal/cachesim/usage_projector.go` 将结果投影为 Claude usage 字段
- [x] 3.3 扩展 `dto/channel_settings.go`，加入缓存模拟 `mode` 和新模式字段
- [x] 3.4 修改 `relay/channel/claude/relay-claude.go`，按 mode 选择 ratio 或 session_prefix
- [x] 3.5 更新 Claude relay 测试，覆盖新模式与旧模式兼容

## 4. 计费与日志

- [x] 4.1 通过 `internal/cachesim/usage_projector.go` 接入现有 usage / quota 口径，确保 session_prefix 结果不重复扣费
- [x] 4.2 修改 `service/log_info_generate.go`，记录 5m/1h 分层字段且不新增日志展示字段
- [x] 4.3 新增服务层测试，验证 quota/log 口径正确

## 5. 前端配置与展示

- [x] 5.1 修改 `web/src/components/table/channels/modals/EditChannelModal.jsx`，支持 mode 感知配置
- [x] 5.2 保持使用日志现有展示结构不变
- [x] 5.3 确认 `web/src/hooks/usage-logs/useUsageLogsData.jsx` 不新增详情字段
- [x] 5.4 确认 `web/src/components/table/usage-logs/UsageLogsColumnDefs.jsx` 保持 Claude “输入”列语义不变
- [x] 5.5 运行 `cd web && npm run build`

## 6. 收尾验证

- [x] 6.1 运行 `go test ./internal/cachesim ./relay/channel/claude ./service -v`
- [x] 6.2 运行 `cd web && npm run build`
- [x] 6.3 再次运行 `openspec validate add-claude-session-cache-simulation --strict`
