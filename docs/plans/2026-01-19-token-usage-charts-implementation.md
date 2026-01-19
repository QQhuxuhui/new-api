# Token用量统计图表实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add two new chart tabs to display token usage statistics with trend and distribution views

**Architecture:** Extend existing dashboard charts by adding two new VChart specifications (line chart for trend, stacked bar chart for distribution) that visualize token_used data from the existing API

**Tech Stack:** React, VChart (@visactor/react-vchart), Semi Design UI, i18n

---

## Task 1: Add i18n translations for token usage charts

**Files:**
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/en.json`

**Step 1: Add Chinese translations**

In `web/src/i18n/locales/zh.json`, find the section with chart-related translations (around line 1256 where "消耗分布" is located) and add:

```json
"Token用量趋势": "Token用量趋势",
"Token用量分布": "Token用量分布",
"总Token用量": "总Token用量",
"对应额度": "对应额度",
"占比": "占比"
```

**Step 2: Add English translations**

In `web/src/i18n/locales/en.json`, find the corresponding section and add:

```json
"Token用量趋势": "Token Usage Trend",
"Token用量分布": "Token Usage Distribution",
"总Token用量": "Total Tokens",
"对应额度": "Quota Cost",
"占比": "Percentage"
```

**Step 3: Verify translations are in correct JSON format**

Run: `cat web/src/i18n/locales/zh.json | python3 -m json.tool > /dev/null && echo "Valid JSON"`

Expected: "Valid JSON"

**Step 4: Commit translations**

```bash
git add web/src/i18n/locales/zh.json web/src/i18n/locales/en.json
git commit -m "i18n: add translations for token usage charts"
```

---

## Task 2: Add token usage trend chart spec (Line Chart)

**Files:**
- Modify: `web/src/hooks/dashboard/useDashboardCharts.jsx:260-261` (after spec_rank_bar)

**Step 1: Add token usage trend chart state**

After line 260 (after `spec_rank_bar` state), add:

```javascript
// Token用量趋势折线图
const [spec_token_line, setSpecTokenLine] = useState({
  type: 'line',
  data: [
    {
      id: 'tokenLineData',
      values: [],
    },
  ],
  xField: 'Time',
  yField: 'TokenUsed',
  seriesField: 'Model',
  legends: {
    visible: true,
    selectMode: 'single',
  },
  title: {
    visible: true,
    text: t('Token用量趋势'),
    subtext: '',
  },
  tooltip: {
    mark: {
      content: [
        {
          key: (datum) => datum['Model'],
          value: (datum) => renderNumber(datum['TokenUsed']),
        },
      ],
    },
  },
  color: {
    specified: modelColorMap,
  },
});
```

**Step 2: Verify the code compiles**

Run: `cd web && npm run build 2>&1 | grep -i error || echo "No errors"`

Expected: "No errors" or successful build

**Step 3: Commit token trend chart spec**

```bash
git add web/src/hooks/dashboard/useDashboardCharts.jsx
git commit -m "feat: add token usage trend chart spec"
```

---

## Task 3: Add token usage distribution chart spec (Stacked Bar Chart)

**Files:**
- Modify: `web/src/hooks/dashboard/useDashboardCharts.jsx` (after spec_token_line)

**Step 1: Add token usage distribution chart state**

After the `spec_token_line` state, add:

```javascript
// Token用量分布堆叠柱状图
const [spec_token_bar, setSpecTokenBar] = useState({
  type: 'bar',
  data: [
    {
      id: 'tokenBarData',
      values: [],
    },
  ],
  xField: 'Time',
  yField: 'TokenUsed',
  seriesField: 'Model',
  stack: true,
  legends: {
    visible: true,
    selectMode: 'single',
  },
  title: {
    visible: true,
    text: t('Token用量分布'),
    subtext: '',
  },
  bar: {
    state: {
      hover: {
        stroke: '#000',
        lineWidth: 1,
      },
    },
  },
  tooltip: {
    mark: {
      content: [
        {
          key: (datum) => datum['Model'],
          value: (datum) => renderNumber(datum['TokenUsed']),
        },
      ],
    },
    dimension: {
      content: [
        {
          key: (datum) => datum['Model'],
          value: (datum) => datum['TokenUsed'] || 0,
        },
      ],
      updateContent: (array) => {
        array.sort((a, b) => b.value - a.value);
        let sum = 0;
        for (let i = 0; i < array.length; i++) {
          let value = parseFloat(array[i].value);
          if (isNaN(value)) {
            value = 0;
          }
          if (array[i].datum && array[i].datum.TimeSum) {
            sum = array[i].datum.TimeSum;
          }
          array[i].value = renderNumber(value);
        }
        array.unshift({
          key: t('总计'),
          value: renderNumber(sum),
        });
        return array;
      },
    },
  },
  color: {
    specified: modelColorMap,
  },
});
```

**Step 2: Verify the code compiles**

Run: `cd web && npm run build 2>&1 | grep -i error || echo "No errors"`

Expected: "No errors" or successful build

**Step 3: Commit token distribution chart spec**

```bash
git add web/src/hooks/dashboard/useDashboardCharts.jsx
git commit -m "feat: add token usage distribution chart spec"
```

---

## Task 4: Generate token chart data in updateChartData function

**Files:**
- Modify: `web/src/hooks/dashboard/useDashboardCharts.jsx:369-407` (in updateChartData function)

**Step 1: Add token line chart data generation**

After line 383 (after `modelLineData.sort`), add:

```javascript
// ===== Token用量趋势折线图 =====
let tokenLineData = [];
chartTimePoints.forEach((time) => {
  const timeData = Array.from(uniqueModels).map((model) => {
    const key = `${time}-${model}`;
    const aggregated = aggregatedData.get(key);
    return {
      Time: time,
      Model: model,
      TokenUsed: aggregated?.token_used || 0,
    };
  });
  tokenLineData.push(...timeData);
});
tokenLineData.sort((a, b) => a.Time.localeCompare(b.Time));
```

**Step 2: Add token bar chart data generation**

After the token line data generation, add:

```javascript
// ===== Token用量分布堆叠柱状图 =====
let tokenBarData = [];
chartTimePoints.forEach((time) => {
  let timeData = Array.from(uniqueModels).map((model) => {
    const key = `${time}-${model}`;
    const aggregated = aggregatedData.get(key);
    return {
      Time: time,
      Model: model,
      TokenUsed: aggregated?.token_used || 0,
    };
  });

  const timeSum = timeData.reduce((sum, item) => sum + item.TokenUsed, 0);
  timeData.sort((a, b) => b.TokenUsed - a.TokenUsed);
  timeData = timeData.map((item) => ({ ...item, TimeSum: timeSum }));
  tokenBarData.push(...timeData);
});
tokenBarData.sort((a, b) => a.Time.localeCompare(b.Time));
```

**Step 3: Add updateChartSpec calls for token charts**

After line 407 (after `setSpecRankBar` call), add:

```javascript
updateChartSpec(
  setSpecTokenLine,
  tokenLineData,
  `${t('总Token用量')}：${renderNumber(totalTokens)}`,
  newModelColors,
  'tokenLineData',
);

updateChartSpec(
  setSpecTokenBar,
  tokenBarData,
  `${t('总Token用量')}：${renderNumber(totalTokens)}`,
  newModelColors,
  'tokenBarData',
);
```

**Step 4: Verify the code compiles**

Run: `cd web && npm run build 2>&1 | grep -i error || echo "No errors"`

Expected: "No errors" or successful build

**Step 5: Commit token chart data generation**

```bash
git add web/src/hooks/dashboard/useDashboardCharts.jsx
git commit -m "feat: generate token usage chart data"
```

---

## Task 5: Export token chart specs from hook

**Files:**
- Modify: `web/src/hooks/dashboard/useDashboardCharts.jsx:436-447` (return statement)

**Step 1: Add token chart specs to return object**

In the return statement (around line 436), modify to include the new specs:

```javascript
return {
  // 图表规格
  spec_pie,
  spec_line,
  spec_model_line,
  spec_rank_bar,
  spec_token_line,
  spec_token_bar,

  // 函数
  updateChartData,
  generateModelColors,
};
```

**Step 2: Verify the code compiles**

Run: `cd web && npm run build 2>&1 | grep -i error || echo "No errors"`

Expected: "No errors" or successful build

**Step 3: Commit export changes**

```bash
git add web/src/hooks/dashboard/useDashboardCharts.jsx
git commit -m "feat: export token chart specs from hook"
```

---

## Task 6: Add token chart tabs to ChartsPanel component

**Files:**
- Modify: `web/src/components/dashboard/ChartsPanel.jsx:25-37` (props and component)

**Step 1: Add token chart specs to component props**

Modify the props destructuring (line 25-37) to include:

```javascript
const ChartsPanel = ({
  activeChartTab,
  setActiveChartTab,
  spec_line,
  spec_model_line,
  spec_pie,
  spec_rank_bar,
  spec_token_line,
  spec_token_bar,
  CARD_PROPS,
  CHART_CONFIG,
  FLEX_CENTER_GAP2,
  hasApiInfoPanel,
  t,
}) => {
```

**Step 2: Add token chart tab panes**

After line 56 (after the last TabPane), add:

```javascript
<TabPane tab={<span>{t('Token用量趋势')}</span>} itemKey='5' />
<TabPane tab={<span>{t('Token用量分布')}</span>} itemKey='6' />
```

**Step 3: Add token chart rendering**

After line 74 (after the last VChart conditional), add:

```javascript
{activeChartTab === '5' && (
  <VChart spec={spec_token_line} option={CHART_CONFIG} />
)}
{activeChartTab === '6' && (
  <VChart spec={spec_token_bar} option={CHART_CONFIG} />
)}
```

**Step 4: Verify the code compiles**

Run: `cd web && npm run build 2>&1 | grep -i error || echo "No errors"`

Expected: "No errors" or successful build

**Step 5: Commit ChartsPanel changes**

```bash
git add web/src/components/dashboard/ChartsPanel.jsx
git commit -m "feat: add token usage chart tabs to ChartsPanel"
```

---

## Task 7: Pass token chart specs to ChartsPanel in dashboard

**Files:**
- Modify: `web/src/components/dashboard/index.jsx` (ChartsPanel usage)

**Step 1: Find ChartsPanel usage in dashboard**

Run: `grep -n "ChartsPanel" web/src/components/dashboard/index.jsx`

Expected: Line number where ChartsPanel is used

**Step 2: Add token chart specs to ChartsPanel props**

Find the ChartsPanel component usage and add the new props:

```javascript
<ChartsPanel
  activeChartTab={activeChartTab}
  setActiveChartTab={setActiveChartTab}
  spec_line={spec_line}
  spec_model_line={spec_model_line}
  spec_pie={spec_pie}
  spec_rank_bar={spec_rank_bar}
  spec_token_line={spec_token_line}
  spec_token_bar={spec_token_bar}
  CARD_PROPS={CARD_PROPS}
  CHART_CONFIG={CHART_CONFIG}
  FLEX_CENTER_GAP2={FLEX_CENTER_GAP2}
  hasApiInfoPanel={hasApiInfoPanel}
  t={t}
/>
```

**Step 3: Verify the code compiles**

Run: `cd web && npm run build 2>&1 | grep -i error || echo "No errors"`

Expected: "No errors" or successful build

**Step 4: Commit dashboard integration**

```bash
git add web/src/components/dashboard/index.jsx
git commit -m "feat: integrate token charts into dashboard"
```

---

## Task 8: Manual testing

**Files:**
- Test: Dashboard page in browser

**Step 1: Start development server**

Run: `cd web && npm run dev`

Expected: Server starts on http://localhost:3000

**Step 2: Navigate to dashboard**

Open browser and go to the dashboard page

Expected: Dashboard loads without errors

**Step 3: Test Token用量趋势 tab**

Click on "Token用量趋势" tab (tab 5)

Expected:
- Line chart displays with token usage data
- Chart title shows "Token用量趋势"
- Subtitle shows "总Token用量: {number}"
- Multiple model lines visible with different colors
- Tooltip shows model name and token count on hover

**Step 4: Test Token用量分布 tab**

Click on "Token用量分布" tab (tab 6)

Expected:
- Stacked bar chart displays with token usage data
- Chart title shows "Token用量分布"
- Subtitle shows "总Token用量: {number}"
- Bars are stacked by model with different colors
- Tooltip shows model breakdown and total on hover

**Step 5: Test time granularity switching**

Switch between hour/day/week time granularities

Expected:
- Both token charts update correctly
- Data aggregates properly for each time period
- No console errors

**Step 6: Test with empty data**

If possible, test with account that has no token usage data

Expected:
- Charts display empty state gracefully
- No JavaScript errors in console

**Step 7: Document test results**

Create a simple test report noting any issues found

---

## Task 9: Final commit and cleanup

**Files:**
- All modified files

**Step 1: Run final build**

Run: `cd web && npm run build`

Expected: Build completes successfully with no errors

**Step 2: Check git status**

Run: `git status`

Expected: All changes committed, working directory clean

**Step 3: Review commit history**

Run: `git log --oneline -10`

Expected: See all commits from this implementation

**Step 4: Create summary commit if needed**

If there are any uncommitted changes:

```bash
git add .
git commit -m "feat: complete token usage charts implementation

- Add Token用量趋势 (line chart) and Token用量分布 (stacked bar chart)
- Display total token usage across all models
- Show per-model token consumption breakdown
- Add i18n translations for Chinese and English
- Integrate with existing dashboard chart infrastructure"
```

---

## Testing Checklist

After implementation, verify:

- [ ] Token用量趋势 tab displays line chart correctly
- [ ] Token用量分布 tab displays stacked bar chart correctly
- [ ] Total token count shown in subtitle is accurate
- [ ] Tooltip shows model name, token count, and percentage
- [ ] Charts update when time granularity changes
- [ ] Model colors are consistent across all charts
- [ ] Charts are responsive on mobile devices
- [ ] No console errors or warnings
- [ ] i18n translations work for both Chinese and English
- [ ] Empty data state handled gracefully

## Notes

- The implementation reuses existing data processing pipeline (`aggregateDataByTimeAndModel`)
- No backend changes required - `token_used` field already exists in API response
- Chart styling matches existing charts for consistency
- VChart configuration follows the same pattern as other charts
