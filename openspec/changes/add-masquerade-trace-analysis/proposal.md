# Change: Add Claude Masquerade Trace Analysis

## Why

目前 Claude 渠道的“伪装”行为（请求头与请求体）只能依赖日志或抓包验证，缺少：

- 单次请求维度的“下游原始 → 上游伪装后”对照
- 最近请求的快速回放与差异定位
- 控制台内可视化对比（差异标红）

这导致排查“UA 与 body 中版本字段不一致”等问题效率较低。

## What Changes

- 对 **Claude 类型渠道** 的请求，记录：
  - 下游请求：完整 headers + body
  - 上游请求：完整 headers + body（伪装/覆盖/转换后的最终发出内容）
- 使用 **进程内内存 ring-buffer** 保存最近 **100 条**记录（先入先出覆盖）。
- 提供 **Admin** 可访问的 API：
  - 查询最近记录
  - 清空记录
- 在控制台「数据分析」页面新增 Tab：**防封分析**，展示列表与 before/after 差异标红对比。

## Impact

- Affected code:
  - `relay/channel/api_request.go`（采集点）
  - `service/*`（内存存储与查询接口）
  - `controller/*` + `router/api-router.go`（Admin API）
  - `web/src/pages/Analytics/*`（新增 Tab 与对比 UI）
- Security:
  - **不脱敏**展示 headers/body（包含 `Authorization` / `x-api-key` / prompt 等敏感信息）
  - 仅 Admin 可访问（沿用现有 `/api/admin/analytics/*` 鉴权）
- Data lifecycle:
  - 仅内存保存，重启即清空，不落库

