# Design: Claude Masquerade Trace Analysis

## Goals

- 对 Claude 渠道每次请求，提供 **“下游原始 → 上游最终”** 的可回放对照数据。
- 仅保留最近 100 条，避免无界增长。
- Admin 控制台可视化差异（headers/body 标红）。

## Capture Point

最佳采集点位于 `relay/channel/api_request.go:DoApiRequest()`：

- 能获取下游请求头：`c.Request.Header`
- 能获取下游原始 body：`common.GetRequestBody(c)` 的缓存
- 能获取上游最终 headers：`req.Header`（已应用 header override + adaptor 固定伪装头）
- 能获取上游最终 body：`bodyBytes`（DoApiRequest 内已读取用于调试/重放）

在完成 `a.SetupRequestHeader(...)` 之后、`client.Do(req)` 之前写入记录，确保记录的是“实际发往上游的最终数据”。

## Storage

实现一个并发安全 ring-buffer：

- 容量固定 100
- 写入为 O(1)
- 读取返回按时间倒序（最近在前）
- 仅内存，不落库

## API

挂到现有 `/api/admin/analytics` 路由组：

- `GET /api/admin/analytics/masquerade_traces`：返回最近 100 条
- `DELETE /api/admin/analytics/masquerade_traces`：清空

## UI

在 `web/src/pages/Analytics/index.jsx` 新增 Tab：`防封分析`，包含：

- 左侧：最近记录表格（时间、渠道、模型、path、上游 URL）
- 右侧：选中记录详情
  - headers diff 表（key / before / after，变化行标红）
  - body diff 表（JSON path / before / after，变化行标红；解析失败则降级为原文对照）

## Security

根据需求选择：**不脱敏**展示 headers/body（风险高）。因此必须：

- 仅 Admin 可访问（沿用现有路由鉴权）
- 明确 UI 提示包含敏感信息

