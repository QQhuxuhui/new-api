# 令牌用量统计弹窗设计

## 背景

企业用户购买平台后会创建多个令牌分发给不同员工，令牌以员工名字命名。用户需要在令牌管理页面查看每个令牌（员工）的使用情况。

## 方案

基于现有 `logs` 表直接聚合查询，无需新建表。logs 表已有 `token_id` 索引，查询效率足够。

## 后端 API

### `GET /api/user/token/stats`

普通用户接口，查询当前用户所有令牌的用量统计。

**请求参数**：
- `start_timestamp` (int64) - 开始时间戳
- `end_timestamp` (int64) - 结束时间戳

**响应**：
```json
{
  "success": true,
  "data": {
    "tokens": [
      {
        "token_id": 1,
        "token_name": "张三",
        "request_count": 1520,
        "quota": 50000,
        "prompt_tokens": 120000,
        "completion_tokens": 80000,
        "models": {
          "gpt-4o": {"request_count": 800, "quota": 30000},
          "claude-3-sonnet": {"request_count": 720, "quota": 20000}
        }
      }
    ],
    "summary": {
      "total_requests": 5000,
      "total_quota": 200000,
      "active_tokens": 8
    }
  }
}
```

**数据库查询**：
```sql
-- 按令牌聚合
SELECT token_id, token_name,
       COUNT(*) as request_count,
       SUM(quota) as total_quota,
       SUM(prompt_tokens) as total_prompt_tokens,
       SUM(completion_tokens) as total_completion_tokens
FROM logs
WHERE user_id = ? AND type = 2
  AND created_at BETWEEN ? AND ?
GROUP BY token_id, token_name

-- 按令牌 + 模型聚合
SELECT token_id, model_name,
       COUNT(*) as request_count,
       SUM(quota) as total_quota
FROM logs
WHERE user_id = ? AND type = 2
  AND created_at BETWEEN ? AND ?
GROUP BY token_id, model_name
```

时间范围限制：最大 90 天。

## 前端弹窗

### 入口

令牌管理页面（`/token`）顶部增加「用量统计」按钮，点击打开大尺寸弹窗。

### 弹窗布局

**1. 时间筛选栏**
- 快捷按钮：今天 | 本周 | 本月 | 最近 7 天 | 最近 30 天
- 自定义日期范围选择器
- 默认选中「最近 7 天」

**2. 汇总卡片区**（一行 3-4 个统计卡片）
- 总调用次数
- 总消耗额度
- 活跃令牌数
- 总 Token 数（prompt + completion）

**3. 令牌对比排名**（柱状图）
- 横轴为令牌名称，纵轴为消耗额度
- 可切换：调用次数 / 消耗额度
- 前 10 个令牌，其余合并为「其他」

**4. 令牌概览表格**
- 列：令牌名称 | 调用次数 | 消耗额度 | Prompt Tokens | Completion Tokens | 最常用模型
- 支持按各列排序
- 点击某行可展开查看该令牌的模型分布详情

**5. 模型分布**（饼图/环形图）
- 全局模型使用占比（所有令牌合计）
- 可切换：调用次数 / 消耗额度

### 技术实现

- 单次 API 调用返回所有数据，前端本地渲染
- 新增 React Hook：`useTokenStats`
- 使用项目现有图表组件
- 无数据时显示空状态提示
