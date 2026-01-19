# Token用量统计图表设计方案

**日期**: 2026-01-19
**状态**: 已确认
**优先级**: 高

## 背景

当前模型数据分析卡片包含多个统计图表（消耗分布、消耗趋势、调用次数分布、调用次数排行），但缺少Token用量的专门图表。由于Token用量与资金直接挂钩，这是用户最关心的数据之一。

## 设计目标

1. 添加Token用量相关的统计图表
2. 按模型分组展示Token消耗情况
3. 显示Token用量总量
4. 提供趋势和分布两种视角
5. 关联显示对应的额度消耗（资金）

## 数据结构

### 数据来源

使用现有API端点，无需后端改动：
- **用户端点**: `GET /api/data/self/`
- **管理员端点**: `GET /api/data/`

### 核心字段

```javascript
{
  model_name: "gpt-4",      // 模型名称
  quota: 1000,              // 额度消耗（用于关联资金）
  count: 50,                // 调用次数
  token_used: 5000,         // Token用量（核心指标）
  created_at: 1704067200    // 时间戳
}
```

### 展示指标

1. **总Token用量** - 所有模型的token总和
2. **按模型的Token用量** - 每个模型的token消耗明细
3. **Token用量趋势** - 随时间变化的token消耗曲线
4. **Token用量分布** - 每个时间段内各模型的token占比
5. **关联额度** - 在tooltip中显示对应的资金消耗

## 图表设计

### Tab 5: Token用量趋势

**图表类型**: 折线图（Line Chart）

**配置**:
- **X轴**: 时间（支持小时/天/周粒度）
- **Y轴**: Token数量
- **系列**: 每个模型一条线
- **标题**: "Token用量趋势"
- **副标题**: "总Token用量: {totalTokens}"
- **图例**: 左侧显示

**用途**:
- 查看Token用量的增长/下降趋势
- 对比不同模型的使用模式
- 识别异常峰值或低谷

**Tooltip内容**:
```
时间: 2026-01-19 14:00
模型: gpt-4
Token用量: 5,000
对应额度: ¥10.00
占比: 35%
```

### Tab 6: Token用量分布

**图表类型**: 堆叠柱状图（Stacked Bar Chart）

**配置**:
- **X轴**: 时间（支持小时/天/周粒度）
- **Y轴**: Token数量
- **系列**: 每个模型一个颜色块
- **堆叠**: 启用（stack: true）
- **标题**: "Token用量分布"
- **副标题**: "总Token用量: {totalTokens}"
- **图例**: 左侧显示

**用途**:
- 查看每个时间段的总Token消耗
- 查看每个模型在总量中的占比
- 识别哪个模型消耗最多

**Tooltip内容**:
```
时间: 2026-01-19 14:00
模型: gpt-4
Token用量: 5,000
对应额度: ¥10.00
占比: 35%
总计: 14,285
```

## 视觉设计

### 颜色方案

复用现有的模型颜色映射，保持与其他图表的一致性。

### 布局

- 与现有4个tab保持相同的布局结构
- 使用相同的卡片样式和间距
- 图例位置统一在左侧

### 响应式

- 支持桌面和移动端显示
- 图表自适应容器宽度
- 移动端图例可折叠

## 实现方案

### 文件改动

#### 1. `useDashboardCharts.jsx`

添加两个新的chart spec：

```javascript
// Token用量趋势图（折线图）
const spec_token_line = useMemo(() => {
  const totalTokens = chartData.reduce((sum, item) => sum + (item.token_used || 0), 0);

  return {
    type: 'line',
    data: [{ id: 'tokenTrend', values: chartData }],
    xField: 'time',
    yField: 'token_used',
    seriesField: 'model_name',
    title: {
      visible: true,
      text: t('Token用量趋势')
    },
    subtitle: {
      visible: true,
      text: `${t('总Token用量')}: ${totalTokens.toLocaleString()}`
    },
    legends: {
      visible: true,
      orient: 'left'
    },
    tooltip: {
      visible: true,
      renderMode: 'canvas',
      mark: {
        content: [
          { key: datum => datum.model_name, value: datum => datum.token_used.toLocaleString() },
          { key: t('对应额度'), value: datum => renderQuota(datum.quota) },
          { key: t('占比'), value: datum => `${((datum.token_used / totalTokens) * 100).toFixed(1)}%` }
        ]
      }
    },
    axes: [
      { orient: 'bottom', type: 'band' },
      { orient: 'left', type: 'linear', label: { formatMethod: val => val.toLocaleString() } }
    ]
  };
}, [chartData, t]);

// Token用量分布图（堆叠柱状图）
const spec_token_bar = useMemo(() => {
  const totalTokens = chartData.reduce((sum, item) => sum + (item.token_used || 0), 0);

  return {
    type: 'bar',
    data: [{ id: 'tokenDist', values: chartData }],
    xField: 'time',
    yField: 'token_used',
    seriesField: 'model_name',
    stack: true,
    title: {
      visible: true,
      text: t('Token用量分布')
    },
    subtitle: {
      visible: true,
      text: `${t('总Token用量')}: ${totalTokens.toLocaleString()}`
    },
    legends: {
      visible: true,
      orient: 'left'
    },
    tooltip: {
      visible: true,
      renderMode: 'canvas',
      mark: {
        content: [
          { key: datum => datum.model_name, value: datum => datum.token_used.toLocaleString() },
          { key: t('对应额度'), value: datum => renderQuota(datum.quota) },
          { key: t('占比'), value: datum => `${((datum.token_used / totalTokens) * 100).toFixed(1)}%` }
        ]
      }
    },
    axes: [
      { orient: 'bottom', type: 'band' },
      { orient: 'left', type: 'linear', label: { formatMethod: val => val.toLocaleString() } }
    ]
  };
}, [chartData, t]);
```

返回值中添加：
```javascript
return {
  // ... 现有的specs
  spec_token_line,
  spec_token_bar,
};
```

#### 2. `ChartsPanel.jsx`

在tabs数组中添加两个新tab：

```javascript
const tabs = [
  { key: '1', tab: t('消耗分布'), spec: spec_line },
  { key: '2', tab: t('消耗趋势'), spec: spec_model_line },
  { key: '3', tab: t('调用次数分布'), spec: spec_pie },
  { key: '4', tab: t('调用次数排行'), spec: spec_rank_bar },
  { key: '5', tab: t('Token用量趋势'), spec: spec_token_line },
  { key: '6', tab: t('Token用量分布'), spec: spec_token_bar },
];
```

#### 3. 国际化文件

需要添加的翻译key：
- `Token用量趋势`
- `Token用量分布`
- `总Token用量`
- `对应额度`
- `占比`

### 数据处理

复用现有的数据处理管道：
- `aggregateDataByTimeAndModel()` - 已包含token_used字段的聚合
- `processRawData()` - 无需修改
- `calculateTrendData()` - 无需修改

### 优势

1. **最小改动**: 复用现有架构，只需添加新的chart spec
2. **无需后端改动**: API已返回所需数据
3. **一致性**: 与现有图表风格完全一致
4. **可维护性**: 遵循现有代码模式

## 测试要点

1. **数据准确性**: 验证token_used字段的聚合计算正确
2. **总量计算**: 验证副标题中的总Token用量准确
3. **Tooltip显示**: 验证tooltip中的额度和占比计算正确
4. **时间粒度**: 测试小时/天/周三种粒度下的显示
5. **模型过滤**: 测试多个模型时的颜色区分和图例显示
6. **响应式**: 测试不同屏幕尺寸下的显示效果
7. **空数据**: 测试无数据时的显示状态

## 后续优化（可选）

1. **Input/Output Token分离**: 如果API支持，可以进一步区分输入和输出token
2. **成本预测**: 基于历史token用量预测未来成本
3. **异常告警**: Token用量异常增长时的提醒
4. **导出功能**: 支持导出token用量报表

## 总结

本设计方案通过添加两个新的图表tab，为用户提供了Token用量的趋势和分布视图。方案充分复用现有架构，实现成本低，同时通过关联显示额度信息，帮助用户更好地理解资金消耗情况。
