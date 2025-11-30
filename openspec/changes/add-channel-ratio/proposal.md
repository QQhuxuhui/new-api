# Change: 添加渠道倍率（Channel Ratio）

## Why

当前系统的计费公式为：`费用 = Token × 模型倍率 × 用户分组倍率`。

运营需要对不同渠道设置不同的价格倍率以反映实际成本差异：
- 官方 OpenAI 渠道成本高，倍率 1.0
- 官方 Claude 渠道成本更高，倍率 1.2
- 第三方代理渠道成本低，倍率 0.8
- 高级专属渠道成本高，倍率 1.5

目前只能通过修改模型倍率来实现，但模型倍率由上游统一提供，修改会增加运营成本且难以统计。

## What Changes

### 核心变更
- **Channel 表新增 `ratio` 字段**：渠道倍率，默认 1.0
- **计费公式扩展**：`费用 = Token × 模型倍率 × 用户分组倍率 × 渠道倍率`
- **渠道倍率对用户隐藏**：定价页面和账单不展示渠道倍率细节

### 配套变更
- **账单展示优化**：不显示单价，只显示总扣费金额
- **重试计费修正**：渠道切换时重新计算费用，退回差额后按新渠道扣费
- **管理后台增强**：渠道编辑页面新增倍率配置字段

## Impact

- **Affected specs**:
  - `channel-management` (新增渠道倍率配置)
  - `billing` (新增计费规格)

- **Affected code**:
  - `model/channel.go` - Channel 模型增加 Ratio 字段
  - `relay/helper/price.go` - 计费逻辑修改
  - `controller/relay.go` - 重试时费用重算逻辑
  - `service/quota.go` - 扣费逻辑调整
  - `web/src/pages/Channel/` - 前端渠道管理页面

- **Database migration**: 需要为 `channels` 表添加 `ratio` 列

- **Breaking changes**: 无，新字段默认值为 1.0，保持向后兼容
