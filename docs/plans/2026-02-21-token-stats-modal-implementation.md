# Token Stats Modal Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a "用量统计" (Usage Statistics) button to the token management page that opens a modal showing per-token usage analytics (summary cards, ranking chart, overview table, model distribution chart).

**Architecture:** Backend adds one new API endpoint (`GET /api/token/stats`) that queries the existing `logs` table with GROUP BY aggregation. Frontend adds a modal component triggered from TokensActions, using VChart for visualizations and Semi Design for UI.

**Tech Stack:** Go/Gin/GORM (backend), React/Semi Design/VChart (frontend), react-i18next (i18n)

---

### Task 1: Backend - Add aggregation query functions to model layer

**Files:**
- Modify: `model/log.go` (append new functions at end of file)

**Step 1: Add TokenStatsSummary and TokenStatsModelBreakdown structs and query functions**

Add the following to the end of `model/log.go`:

```go
// TokenStatsSummary holds aggregated stats for a single token
type TokenStatsSummary struct {
	TokenId          int    `json:"token_id"`
	TokenName        string `json:"token_name"`
	RequestCount     int64  `json:"request_count"`
	Quota            int64  `json:"quota"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
}

// TokenStatsModelBreakdown holds per-model stats for a token
type TokenStatsModelBreakdown struct {
	TokenId      int    `json:"token_id"`
	ModelName    string `json:"model_name"`
	RequestCount int64  `json:"request_count"`
	Quota        int64  `json:"quota"`
}

// GetTokenStatsByUserId aggregates log data by token for a given user and time range.
func GetTokenStatsByUserId(userId int, startTimestamp, endTimestamp int64) ([]*TokenStatsSummary, error) {
	var stats []*TokenStatsSummary
	err := LOG_DB.Model(&Log{}).
		Select("token_id, token_name, COUNT(*) as request_count, SUM(quota) as quota, SUM(prompt_tokens) as prompt_tokens, SUM(completion_tokens) as completion_tokens").
		Where("user_id = ? AND type = ? AND created_at BETWEEN ? AND ?", userId, LogTypeConsume, startTimestamp, endTimestamp).
		Group("token_id, token_name").
		Order("quota DESC").
		Find(&stats).Error
	return stats, err
}

// GetTokenStatsModelBreakdown aggregates log data by token+model for a given user and time range.
func GetTokenStatsModelBreakdown(userId int, startTimestamp, endTimestamp int64) ([]*TokenStatsModelBreakdown, error) {
	var stats []*TokenStatsModelBreakdown
	err := LOG_DB.Model(&Log{}).
		Select("token_id, model_name, COUNT(*) as request_count, SUM(quota) as quota").
		Where("user_id = ? AND type = ? AND created_at BETWEEN ? AND ?", userId, LogTypeConsume, startTimestamp, endTimestamp).
		Group("token_id, model_name").
		Find(&stats).Error
	return stats, err
}
```

**Step 2: Verify the code compiles**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./model/...`
Expected: No errors

**Step 3: Commit**

```bash
git add model/log.go
git commit -m "feat: add token stats aggregation query functions"
```

---

### Task 2: Backend - Add token stats controller

**Files:**
- Create: `controller/token_analytics.go`

**Step 1: Create the controller file**

Create `controller/token_analytics.go`:

```go
package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const maxTokenStatsRangeDays = 90

func GetTokenStats(c *gin.Context) {
	userId := c.GetInt("id")

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	now := time.Now().Unix()
	if endTimestamp <= 0 {
		endTimestamp = now
	}
	if startTimestamp <= 0 {
		startTimestamp = endTimestamp - 7*24*3600
	}

	// Enforce max range
	maxRange := int64(maxTokenStatsRangeDays * 24 * 3600)
	if endTimestamp-startTimestamp > maxRange {
		startTimestamp = endTimestamp - maxRange
	}

	// Query 1: per-token aggregation
	tokenStats, err := model.GetTokenStatsByUserId(userId, startTimestamp, endTimestamp)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Query 2: per-token per-model breakdown
	modelBreakdown, err := model.GetTokenStatsModelBreakdown(userId, startTimestamp, endTimestamp)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Build model map: tokenId -> { modelName -> { request_count, quota } }
	modelMap := make(map[int]map[string]map[string]int64)
	for _, mb := range modelBreakdown {
		if _, ok := modelMap[mb.TokenId]; !ok {
			modelMap[mb.TokenId] = make(map[string]map[string]int64)
		}
		modelMap[mb.TokenId][mb.ModelName] = map[string]int64{
			"request_count": mb.RequestCount,
			"quota":         mb.Quota,
		}
	}

	// Build response tokens array
	var totalRequests int64
	var totalQuota int64
	activeTokens := 0

	type tokenResponse struct {
		TokenId          int                        `json:"token_id"`
		TokenName        string                     `json:"token_name"`
		RequestCount     int64                      `json:"request_count"`
		Quota            int64                      `json:"quota"`
		PromptTokens     int64                      `json:"prompt_tokens"`
		CompletionTokens int64                      `json:"completion_tokens"`
		Models           map[string]map[string]int64 `json:"models"`
	}

	tokens := make([]tokenResponse, 0, len(tokenStats))
	for _, ts := range tokenStats {
		totalRequests += ts.RequestCount
		totalQuota += ts.Quota
		if ts.RequestCount > 0 {
			activeTokens++
		}
		tokens = append(tokens, tokenResponse{
			TokenId:          ts.TokenId,
			TokenName:        ts.TokenName,
			RequestCount:     ts.RequestCount,
			Quota:            ts.Quota,
			PromptTokens:     ts.PromptTokens,
			CompletionTokens: ts.CompletionTokens,
			Models:           modelMap[ts.TokenId],
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"tokens": tokens,
			"summary": gin.H{
				"total_requests": totalRequests,
				"total_quota":    totalQuota,
				"active_tokens":  activeTokens,
			},
		},
	})
}
```

**Step 2: Verify the code compiles**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./controller/...`
Expected: No errors

**Step 3: Commit**

```bash
git add controller/token_analytics.go
git commit -m "feat: add token stats controller endpoint"
```

---

### Task 3: Backend - Register route

**Files:**
- Modify: `router/api-router.go:220-230` (token route group)

**Step 1: Add the stats route to the token route group**

In `router/api-router.go`, inside the `tokenRoute` group block (after line 229, before the closing `}`), add:

```go
tokenRoute.GET("/stats", controller.GetTokenStats)
```

The route must be registered BEFORE the `/:id` route to avoid Gin treating "stats" as an `:id` parameter. Move it right after the `/search` route:

```go
tokenRoute := apiRouter.Group("/token")
tokenRoute.Use(middleware.UserAuth())
{
    tokenRoute.GET("/", controller.GetAllTokens)
    tokenRoute.GET("/search", controller.SearchTokens)
    tokenRoute.GET("/stats", controller.GetTokenStats)
    tokenRoute.GET("/:id", controller.GetToken)
    tokenRoute.POST("/", controller.AddToken)
    tokenRoute.PUT("/", controller.UpdateToken)
    tokenRoute.DELETE("/:id", controller.DeleteToken)
    tokenRoute.POST("/batch", controller.DeleteTokenBatch)
}
```

**Step 2: Verify the code compiles**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./...`
Expected: No errors

**Step 3: Commit**

```bash
git add router/api-router.go
git commit -m "feat: register token stats route"
```

---

### Task 4: Frontend - Create useTokenAnalytics hook

**Files:**
- Create: `web/src/hooks/tokens/useTokenAnalytics.jsx`

**Step 1: Create the hook**

Create `web/src/hooks/tokens/useTokenAnalytics.jsx`:

```jsx
import { useState, useCallback } from 'react';
import { API, showError } from '../../helpers';

const PRESET_RANGES = {
  today: () => {
    const now = new Date();
    const start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    return [Math.floor(start.getTime() / 1000), Math.floor(now.getTime() / 1000)];
  },
  thisWeek: () => {
    const now = new Date();
    const day = now.getDay() || 7;
    const start = new Date(now.getFullYear(), now.getMonth(), now.getDate() - day + 1);
    return [Math.floor(start.getTime() / 1000), Math.floor(now.getTime() / 1000)];
  },
  thisMonth: () => {
    const now = new Date();
    const start = new Date(now.getFullYear(), now.getMonth(), 1);
    return [Math.floor(start.getTime() / 1000), Math.floor(now.getTime() / 1000)];
  },
  last7Days: () => {
    const now = new Date();
    const start = new Date(now.getTime() - 7 * 24 * 3600 * 1000);
    return [Math.floor(start.getTime() / 1000), Math.floor(now.getTime() / 1000)];
  },
  last30Days: () => {
    const now = new Date();
    const start = new Date(now.getTime() - 30 * 24 * 3600 * 1000);
    return [Math.floor(start.getTime() / 1000), Math.floor(now.getTime() / 1000)];
  },
};

export function useTokenAnalytics() {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [activeRange, setActiveRange] = useState('last7Days');
  const [customRange, setCustomRange] = useState(null);

  const fetchStats = useCallback(async (startTimestamp, endTimestamp) => {
    setLoading(true);
    try {
      const res = await API.get(
        `/api/token/stats?start_timestamp=${startTimestamp}&end_timestamp=${endTimestamp}`
      );
      const { success, message, data: resData } = res.data || {};
      if (success) {
        setData(resData);
      } else {
        showError(message || 'Failed to fetch token stats');
      }
    } catch (e) {
      showError(e.message || 'Failed to fetch token stats');
    } finally {
      setLoading(false);
    }
  }, []);

  const selectPresetRange = useCallback(
    (rangeKey) => {
      setActiveRange(rangeKey);
      setCustomRange(null);
      const rangeFn = PRESET_RANGES[rangeKey];
      if (rangeFn) {
        const [start, end] = rangeFn();
        fetchStats(start, end);
      }
    },
    [fetchStats]
  );

  const selectCustomRange = useCallback(
    (dates) => {
      if (!dates || dates.length < 2) return;
      setActiveRange(null);
      const start = Math.floor(new Date(dates[0]).getTime() / 1000);
      const end = Math.floor(new Date(dates[1]).getTime() / 1000) + 86399; // end of day
      setCustomRange(dates);
      fetchStats(start, end);
    },
    [fetchStats]
  );

  const initLoad = useCallback(() => {
    selectPresetRange('last7Days');
  }, [selectPresetRange]);

  return {
    data,
    loading,
    activeRange,
    customRange,
    selectPresetRange,
    selectCustomRange,
    initLoad,
    PRESET_RANGES,
  };
}
```

**Step 2: Commit**

```bash
git add web/src/hooks/tokens/useTokenAnalytics.jsx
git commit -m "feat: add useTokenAnalytics hook"
```

---

### Task 5: Frontend - Create TokenAnalyticsModal component

**Files:**
- Create: `web/src/components/table/tokens/modals/TokenAnalyticsModal.jsx`

**Step 1: Create the modal component**

Create `web/src/components/table/tokens/modals/TokenAnalyticsModal.jsx`.

This is the largest frontend file. It contains:
1. Time range selector (preset buttons + DatePicker)
2. Summary cards (total requests, total quota, active tokens, total tokens)
3. Bar chart (token ranking by quota, switchable to request count)
4. Overview table with expandable rows (model breakdown)
5. Pie chart (global model distribution)

```jsx
import React, { useState, useEffect, useMemo } from 'react';
import {
  Modal,
  Card,
  Table,
  Tag,
  Button,
  Space,
  DatePicker,
  Spin,
  Empty,
  Typography,
  RadioGroup,
  Radio,
} from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';
import { useTokenAnalytics } from '../../../../hooks/tokens/useTokenAnalytics';
import { renderQuota, renderNumber } from '../../../../helpers';

const CHART_CONFIG = { mode: 'desktop-browser' };

const TokenAnalyticsModal = ({ visible, onClose, t }) => {
  const {
    data,
    loading,
    activeRange,
    customRange,
    selectPresetRange,
    selectCustomRange,
    initLoad,
  } = useTokenAnalytics();

  const [barMetric, setBarMetric] = useState('quota');
  const [pieMetric, setPieMetric] = useState('quota');

  useEffect(() => {
    if (visible) {
      initLoad();
    }
  }, [visible, initLoad]);

  const presetButtons = [
    { key: 'today', label: t('今天') },
    { key: 'thisWeek', label: t('本周') },
    { key: 'thisMonth', label: t('本月') },
    { key: 'last7Days', label: t('最近 7 天') },
    { key: 'last30Days', label: t('最近 30 天') },
  ];

  // Build bar chart spec
  const barChartSpec = useMemo(() => {
    if (!data?.tokens?.length) return null;
    const sorted = [...data.tokens]
      .sort((a, b) => b[barMetric] - a[barMetric])
      .slice(0, 10);

    const values = sorted.map((tk) => ({
      name: tk.token_name || `Token #${tk.token_id}`,
      value: barMetric === 'quota' ? tk.quota : tk.request_count,
    }));

    return {
      type: 'bar',
      data: [{ id: 'bar', values }],
      xField: 'name',
      yField: 'value',
      title: {
        visible: true,
        text:
          barMetric === 'quota'
            ? t('令牌消耗排名（额度）')
            : t('令牌调用排名（次数）'),
      },
      label: { visible: true },
      tooltip: {
        mark: {
          content: [
            {
              key: (datum) => datum['name'],
              value: (datum) =>
                barMetric === 'quota'
                  ? renderQuota(datum['value'], 2)
                  : renderNumber(datum['value']),
            },
          ],
        },
      },
    };
  }, [data, barMetric, t]);

  // Build pie chart spec
  const pieChartSpec = useMemo(() => {
    if (!data?.tokens?.length) return null;

    const modelTotals = {};
    data.tokens.forEach((tk) => {
      if (tk.models) {
        Object.entries(tk.models).forEach(([modelName, stats]) => {
          if (!modelTotals[modelName]) {
            modelTotals[modelName] = { request_count: 0, quota: 0 };
          }
          modelTotals[modelName].request_count += stats.request_count;
          modelTotals[modelName].quota += stats.quota;
        });
      }
    });

    const values = Object.entries(modelTotals).map(([name, stats]) => ({
      type: name,
      value: pieMetric === 'quota' ? stats.quota : stats.request_count,
    }));

    return {
      type: 'pie',
      data: [{ id: 'pie', values }],
      outerRadius: 0.8,
      innerRadius: 0.5,
      padAngle: 0.6,
      valueField: 'value',
      categoryField: 'type',
      pie: {
        style: { cornerRadius: 10 },
        state: {
          hover: { outerRadius: 0.85, stroke: '#000', lineWidth: 1 },
        },
      },
      title: {
        visible: true,
        text:
          pieMetric === 'quota'
            ? t('模型消耗分布（额度）')
            : t('模型调用分布（次数）'),
      },
      legends: { visible: true, orient: 'left' },
      label: { visible: true },
      tooltip: {
        mark: {
          content: [
            {
              key: (datum) => datum['type'],
              value: (datum) =>
                pieMetric === 'quota'
                  ? renderQuota(datum['value'], 2)
                  : renderNumber(datum['value']),
            },
          ],
        },
      },
    };
  }, [data, pieMetric, t]);

  // Table columns
  const columns = useMemo(
    () => [
      {
        title: t('令牌名称'),
        dataIndex: 'token_name',
        render: (text, record) => text || `Token #${record.token_id}`,
      },
      {
        title: t('调用次数'),
        dataIndex: 'request_count',
        sorter: (a, b) => a.request_count - b.request_count,
        render: (val) => renderNumber(val),
      },
      {
        title: t('消耗额度'),
        dataIndex: 'quota',
        sorter: (a, b) => a.quota - b.quota,
        render: (val) => renderQuota(val, 2),
      },
      {
        title: 'Prompt Tokens',
        dataIndex: 'prompt_tokens',
        sorter: (a, b) => a.prompt_tokens - b.prompt_tokens,
        render: (val) => renderNumber(val),
      },
      {
        title: 'Completion Tokens',
        dataIndex: 'completion_tokens',
        sorter: (a, b) => a.completion_tokens - b.completion_tokens,
        render: (val) => renderNumber(val),
      },
      {
        title: t('最常用模型'),
        dataIndex: 'models',
        render: (models) => {
          if (!models) return '-';
          const sorted = Object.entries(models).sort(
            (a, b) => b[1].quota - a[1].quota
          );
          return sorted.length > 0 ? (
            <Tag size='small'>{sorted[0][0]}</Tag>
          ) : (
            '-'
          );
        },
      },
    ],
    [t]
  );

  // Expandable row for model breakdown
  const expandedRowRender = (record) => {
    if (!record.models || Object.keys(record.models).length === 0) {
      return <Empty description={t('暂无模型数据')} />;
    }
    const modelData = Object.entries(record.models).map(
      ([name, stats]) => ({
        model_name: name,
        request_count: stats.request_count,
        quota: stats.quota,
      })
    );
    const modelColumns = [
      { title: t('模型名称'), dataIndex: 'model_name' },
      {
        title: t('调用次数'),
        dataIndex: 'request_count',
        render: (val) => renderNumber(val),
      },
      {
        title: t('消耗额度'),
        dataIndex: 'quota',
        render: (val) => renderQuota(val, 2),
      },
    ];
    return (
      <Table
        columns={modelColumns}
        dataSource={modelData}
        rowKey='model_name'
        pagination={false}
        size='small'
      />
    );
  };

  const summary = data?.summary;

  return (
    <Modal
      title={t('令牌用量统计')}
      visible={visible}
      onCancel={onClose}
      footer={null}
      width={960}
      style={{ maxWidth: '95vw' }}
      bodyStyle={{ maxHeight: '80vh', overflow: 'auto' }}
    >
      {/* Time Range Selector */}
      <div className='flex flex-wrap items-center gap-2 mb-4'>
        {presetButtons.map((btn) => (
          <Button
            key={btn.key}
            type={activeRange === btn.key ? 'primary' : 'tertiary'}
            size='small'
            onClick={() => selectPresetRange(btn.key)}
          >
            {btn.label}
          </Button>
        ))}
        <DatePicker
          type='dateRange'
          density='compact'
          value={customRange}
          onChange={selectCustomRange}
          style={{ width: 260 }}
        />
      </div>

      <Spin spinning={loading}>
        {!data || !data.tokens?.length ? (
          <Empty
            description={t('暂无数据')}
            style={{ padding: '60px 0' }}
          />
        ) : (
          <div className='flex flex-col gap-4'>
            {/* Summary Cards */}
            <div className='grid grid-cols-2 md:grid-cols-4 gap-3'>
              <Card bodyStyle={{ padding: '12px 16px' }}>
                <Typography.Text type='tertiary' size='small'>
                  {t('总调用次数')}
                </Typography.Text>
                <Typography.Title heading={4} style={{ margin: 0 }}>
                  {renderNumber(summary?.total_requests || 0)}
                </Typography.Title>
              </Card>
              <Card bodyStyle={{ padding: '12px 16px' }}>
                <Typography.Text type='tertiary' size='small'>
                  {t('总消耗额度')}
                </Typography.Text>
                <Typography.Title heading={4} style={{ margin: 0 }}>
                  {renderQuota(summary?.total_quota || 0, 2)}
                </Typography.Title>
              </Card>
              <Card bodyStyle={{ padding: '12px 16px' }}>
                <Typography.Text type='tertiary' size='small'>
                  {t('活跃令牌数')}
                </Typography.Text>
                <Typography.Title heading={4} style={{ margin: 0 }}>
                  {summary?.active_tokens || 0}
                </Typography.Title>
              </Card>
              <Card bodyStyle={{ padding: '12px 16px' }}>
                <Typography.Text type='tertiary' size='small'>
                  {t('总 Token 数')}
                </Typography.Text>
                <Typography.Title heading={4} style={{ margin: 0 }}>
                  {renderNumber(
                    data.tokens.reduce(
                      (sum, tk) =>
                        sum + (tk.prompt_tokens || 0) + (tk.completion_tokens || 0),
                      0
                    )
                  )}
                </Typography.Title>
              </Card>
            </div>

            {/* Bar Chart - Token Ranking */}
            {barChartSpec && (
              <Card
                title={
                  <div className='flex items-center justify-between w-full'>
                    <span>{t('令牌对比排名')}</span>
                    <RadioGroup
                      type='button'
                      size='small'
                      value={barMetric}
                      onChange={(e) => setBarMetric(e.target.value)}
                    >
                      <Radio value='quota'>{t('消耗额度')}</Radio>
                      <Radio value='request_count'>{t('调用次数')}</Radio>
                    </RadioGroup>
                  </div>
                }
              >
                <div style={{ height: 300 }}>
                  <VChart spec={barChartSpec} option={CHART_CONFIG} />
                </div>
              </Card>
            )}

            {/* Token Overview Table */}
            <Card title={t('令牌概览')}>
              <Table
                columns={columns}
                dataSource={data.tokens}
                rowKey='token_id'
                expandedRowRender={expandedRowRender}
                pagination={
                  data.tokens.length > 10
                    ? { pageSize: 10, showSizeChanger: true }
                    : false
                }
                size='small'
              />
            </Card>

            {/* Pie Chart - Model Distribution */}
            {pieChartSpec && (
              <Card
                title={
                  <div className='flex items-center justify-between w-full'>
                    <span>{t('模型分布')}</span>
                    <RadioGroup
                      type='button'
                      size='small'
                      value={pieMetric}
                      onChange={(e) => setPieMetric(e.target.value)}
                    >
                      <Radio value='quota'>{t('消耗额度')}</Radio>
                      <Radio value='request_count'>{t('调用次数')}</Radio>
                    </RadioGroup>
                  </div>
                }
              >
                <div style={{ height: 350 }}>
                  <VChart spec={pieChartSpec} option={CHART_CONFIG} />
                </div>
              </Card>
            )}
          </div>
        )}
      </Spin>
    </Modal>
  );
};

export default TokenAnalyticsModal;
```

**Step 2: Commit**

```bash
git add web/src/components/table/tokens/modals/TokenAnalyticsModal.jsx
git commit -m "feat: add TokenAnalyticsModal component"
```

---

### Task 6: Frontend - Integrate modal into TokensActions

**Files:**
- Modify: `web/src/components/table/tokens/TokensActions.jsx`

**Step 1: Add import and state for the analytics modal**

At the top of `TokensActions.jsx`, add the import (after existing modal imports around line 27):

```jsx
import TokenAnalyticsModal from './modals/TokenAnalyticsModal';
```

Inside the component, add state (after line 45):

```jsx
const [showAnalytics, setShowAnalytics] = useState(false);
```

**Step 2: Add the analytics button**

In the button area (inside the `<div className='flex flex-wrap gap-2 ...'>` block), add a new button after the "添加令牌" button (after line 120):

```jsx
<Button
  type='secondary'
  className='flex-1 md:flex-initial'
  onClick={() => setShowAnalytics(true)}
  size='small'
>
  {t('用量统计')}
</Button>
```

**Step 3: Add the modal component**

In the JSX return, before the closing `</>` (after `TokenCreatedSuccess` around line 177), add:

```jsx
<TokenAnalyticsModal
  visible={showAnalytics}
  onClose={() => setShowAnalytics(false)}
  t={t}
/>
```

**Step 4: Commit**

```bash
git add web/src/components/table/tokens/TokensActions.jsx
git commit -m "feat: integrate token analytics modal into token page"
```

---

### Task 7: I18n - Add translation keys

**Files:**
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/en.json`

**Step 1: Add Chinese keys**

Add the following keys to `zh.json` (inside the `"translation"` object):

```json
"用量统计": "用量统计",
"令牌用量统计": "令牌用量统计",
"今天": "今天",
"本周": "本周",
"本月": "本月",
"最近 7 天": "最近 7 天",
"最近 30 天": "最近 30 天",
"总调用次数": "总调用次数",
"总消耗额度": "总消耗额度",
"活跃令牌数": "活跃令牌数",
"总 Token 数": "总 Token 数",
"令牌对比排名": "令牌对比排名",
"令牌消耗排名（额度）": "令牌消耗排名（额度）",
"令牌调用排名（次数）": "令牌调用排名（次数）",
"消耗额度": "消耗额度",
"调用次数": "调用次数",
"令牌概览": "令牌概览",
"令牌名称": "令牌名称",
"最常用模型": "最常用模型",
"模型分布": "模型分布",
"模型消耗分布（额度）": "模型消耗分布（额度）",
"模型调用分布（次数）": "模型调用分布（次数）",
"模型名称": "模型名称",
"暂无模型数据": "暂无模型数据"
```

**Step 2: Add English keys**

Add the following keys to `en.json`:

```json
"用量统计": "Usage Statistics",
"令牌用量统计": "Token Usage Statistics",
"今天": "Today",
"本周": "This Week",
"本月": "This Month",
"最近 7 天": "Last 7 Days",
"最近 30 天": "Last 30 Days",
"总调用次数": "Total Requests",
"总消耗额度": "Total Quota Used",
"活跃令牌数": "Active Tokens",
"总 Token 数": "Total Tokens",
"令牌对比排名": "Token Ranking",
"令牌消耗排名（额度）": "Token Ranking (Quota)",
"令牌调用排名（次数）": "Token Ranking (Requests)",
"消耗额度": "Quota",
"调用次数": "Requests",
"令牌概览": "Token Overview",
"令牌名称": "Token Name",
"最常用模型": "Most Used Model",
"模型分布": "Model Distribution",
"模型消耗分布（额度）": "Model Distribution (Quota)",
"模型调用分布（次数）": "Model Distribution (Requests)",
"模型名称": "Model Name",
"暂无模型数据": "No model data"
```

**Step 3: Run i18n sync for other languages**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api/web && npm run i18n:sync`

Note: Some keys like "今天", "本周", "暂无数据", "调用次数", "消耗额度" may already exist. Check before adding to avoid duplicates. Only add keys that don't already exist.

**Step 4: Commit**

```bash
git add web/src/i18n/locales/
git commit -m "feat: add i18n keys for token analytics"
```

---

### Task 8: Build verification and manual testing

**Step 1: Build backend**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./...`
Expected: No errors

**Step 2: Build frontend**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api/web && npm run build`
Expected: No errors

**Step 3: Commit if any lint/build fixes were needed**

```bash
git add -A
git commit -m "fix: build fixes for token analytics"
```

---

## Summary of All Files

**Created:**
- `controller/token_analytics.go` - Backend controller
- `web/src/hooks/tokens/useTokenAnalytics.jsx` - Frontend hook
- `web/src/components/table/tokens/modals/TokenAnalyticsModal.jsx` - Modal component

**Modified:**
- `model/log.go` - Added 2 aggregation query functions + 2 structs
- `router/api-router.go` - Added 1 route line
- `web/src/components/table/tokens/TokensActions.jsx` - Added button + modal integration
- `web/src/i18n/locales/zh.json` - Added Chinese translation keys
- `web/src/i18n/locales/en.json` - Added English translation keys
