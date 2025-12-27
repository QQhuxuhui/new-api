# Claude Header Masquerading - Development Plan

## Overview
在Claude适配器中实现HTTP请求头伪装，添加15个固定的Stainless SDK特征头，模拟真实Claude Code客户端，避免上游API服务检测到中转模式。

## Task Breakdown

### Task 1: 实现固定请求头伪装
- **ID**: task-1
- **type**: default
- **Description**: 在 `relay/channel/claude/adaptor.go` 的 `SetupRequestHeader()` 函数中添加15个固定请求头，包括9个 X-Stainless-* SDK特征头、3个标准HTTP头、3个Claude/Anthropic特定头。使用固定值（而非透传真实客户端值）以避免暴露多用户设备指纹差异。
- **File Scope**: `relay/channel/claude/adaptor.go`（主要修改 `SetupRequestHeader` 函数）
- **Dependencies**: None
- **Test Command**: `go test ./relay/channel/claude/... -v -cover -coverprofile=coverage.out && go tool cover -func=coverage.out | grep adaptor.go`
- **Test Focus**:
  - 验证所有15个请求头均被正确设置
  - 验证请求头的值为预期固定值
  - 验证原有的anthropic-version、x-api-key、anthropic-beta等头仍正常工作
  - 验证与现有 `SetupApiRequestHeader()` 和 `CommonClaudeHeadersOperation()` 调用无冲突

### Task 2: 编写单元测试
- **ID**: task-2
- **type**: default
- **Description**: 创建 `relay/channel/claude/adaptor_test.go` 文件，编写全面的单元测试验证请求头设置逻辑。包括正常场景、边界场景、已有请求头覆盖场景、与其他头共存场景等测试用例。
- **File Scope**: `relay/channel/claude/adaptor_test.go`（新建文件）
- **Dependencies**: depends on task-1
- **Test Command**: `go test ./relay/channel/claude/... -v -cover -coverprofile=coverage.out && go tool cover -func=coverage.out`
- **Test Focus**:
  - **正常场景**: 所有15个伪装头正确设置
  - **请求头验证**: 验证每个头的具体值与固定配置一致
  - **已有头兼容性**: 验证 anthropic-version、x-api-key、anthropic-beta 等原有头仍正常设置
  - **RequestMode场景**: 验证不同 RequestMode（Message vs Completion）下头设置一致
  - **覆盖率**: 确保 adaptor.go 中的 SetupRequestHeader 函数覆盖率 ≥90%
  - **边界条件**: 空 gin.Context、空 RelayInfo 等场景的健壮性

### Task 3: 添加测试辅助工具（如需要）
- **ID**: task-3
- **type**: quick-fix
- **Description**: 如果任务2中发现需要mock或辅助函数（如创建测试用的 gin.Context、http.Header、RelayInfo 等），则创建测试辅助文件。仅在确实需要时创建，优先使用标准库和已有测试模式。
- **File Scope**: `relay/channel/claude/test_helpers.go`（仅在需要时创建）
- **Dependencies**: depends on task-2
- **Test Command**: `go test ./relay/channel/claude/... -v -cover`
- **Test Focus**:
  - 辅助函数可正确创建测试夹具
  - 辅助函数不引入额外复杂性
  - 所有测试使用辅助函数后仍能通过

## Acceptance Criteria
- [x] 在 `SetupRequestHeader()` 中添加15个固定请求头（9个X-Stainless-*、3个标准HTTP、3个Claude特定）
- [x] 请求头使用固定值（参考 docs/bypass-detection-plan.md 3.6节的配置）
- [x] 原有的 anthropic-version、x-api-key、anthropic-beta 等头设置逻辑保持不变
- [x] 所有单元测试通过（包括新增和已有测试）
- [x] 代码覆盖率 ≥90%（针对修改的 adaptor.go 和新增的测试文件）
- [x] 测试覆盖所有关键场景（正常、边界、兼容性）
- [x] 代码符合项目现有风格（参考现有 adaptor.go 的代码风格）

## Technical Notes
- **固定值策略**: 使用固定的设备指纹（Linux/x64/Node v22.18.0）而非透传真实客户端值，避免暴露多用户模式（详见 docs/bypass-detection-plan.md 3.5节的策略分析）
- **请求头优先级**: 新增的固定头应在调用 `CommonClaudeHeadersOperation()` 之前设置，避免被覆盖
- **测试策略**: 参考 `common/currency_test.go` 的表驱动测试模式，使用标准库 `testing` 包，无需额外测试框架
- **覆盖率计算**: 使用 `go test -cover -coverprofile=coverage.out` 和 `go tool cover -func=coverage.out` 验证覆盖率
- **局限性**: 此方案仅解决HTTP头层面的检测，TLS指纹、HTTP/2指纹、请求行为模式等检测需要后续阶段处理（参考 docs/bypass-detection-plan.md 第二至第四阶段）

## 参考文档
- **主要参考**: `/usr/src/workspace/github/QQhuxuhui/new-api/docs/bypass-detection-plan.md`（第3.5-3.6节）
- **代码位置**: `/usr/src/workspace/github/QQhuxuhui/new-api/relay/channel/claude/adaptor.go`（第74-84行 SetupRequestHeader 函数）
- **测试模式参考**: `/usr/src/workspace/github/QQhuxuhui/new-api/common/currency_test.go`
