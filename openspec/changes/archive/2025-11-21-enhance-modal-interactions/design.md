# Design: Enhance Modal Interactions

## Architecture Overview

This change involves two independent UI improvements with no backend dependencies:

```
┌─────────────────────────────────────────────────────────┐
│                     Frontend Only                       │
├─────────────────────────────────────────────────────────┤
│  Capability 1: Onboarding Restart                       │
│  ┌──────────────┐      ┌────────────────┐              │
│  │ Help Menu    │─────▶│ OnboardingWiz  │              │
│  │ Click        │      │ (Step 1 Reset) │              │
│  └──────────────┘      └────────────────┘              │
│         │                      │                        │
│         └──────────────────────┘                        │
│                │                                         │
│                ▼                                         │
│         useOnboarding Hook                              │
│         (localStorage: clear currentStep)               │
│                                                          │
│  Capability 2: Channel Health Modal Redesign            │
│  ┌──────────────┐      ┌────────────────┐              │
│  │ Health Badge │─────▶│ Enhanced Modal │              │
│  │ Click        │      │ (New Layout)   │              │
│  └──────────────┘      └────────────────┘              │
│                               │                         │
│                        Uses existing API                │
│                        (no backend changes)             │
└─────────────────────────────────────────────────────────┘
```

## Capability 1: Onboarding Restart Design

### Current Flow (Problem)

```
User Opens Wizard → Check localStorage → Resume at Step N
                          │
                          ├─ currentStep: 2
                          ├─ completedSteps: [1]
                          └─ createdToken: {...}

Result: User sees Step 2, confused why not Step 1
```

### New Flow (Solution)

```
User Opens Wizard → Check if fresh session → Always start Step 1
                          │
                          ├─ Clear currentStep on close
                          ├─ Keep completedSteps (analytics only)
                          └─ Show welcome screen

Result: User sees Step 1, clear fresh start
```

### State Management Changes

**Before (OnboardingWizard.jsx lines 58-78):**
```javascript
useEffect(() => {
  if (visible && progress.startTime && !hasRestoredProgress.current) {
    hasRestoredProgress.current = true;
    const restoredStep = progress.currentStep || 1;  // ❌ Restores step
    setCurrentStep(restoredStep);
    // ... restore other state
  }
}, [visible, progress.startTime]);
```

**After:**
```javascript
useEffect(() => {
  if (visible) {
    // Always reset to step 1 on open
    setCurrentStep(1);
    // Clear localStorage currentStep but keep analytics data
    updateProgress({ currentStep: 1 });
    startStep(1);
  }
}, [visible]);
```

**useOnboarding Hook Changes:**
```javascript
// Add method to reset currentStep only
const resetCurrentStep = () => {
  const updatedProgress = { ...progress, currentStep: 1 };
  setProgress(updatedProgress);
  localStorage.setItem(STORAGE_KEYS.PROGRESS, JSON.stringify(updatedProgress));
};
```

### Trade-offs

**Option A: Always Reset (Chosen)**
- ✅ Simple, predictable behavior
- ✅ Matches user mental model
- ❌ Loses resume capability (acceptable trade-off)

**Option B: Smart Resume (Rejected)**
- Resume only if closed <5 minutes ago
- ❌ Complex logic, hard to explain to users
- ❌ Still confusing edge cases

## Capability 2: Channel Health Modal Design

### Current Layout Analysis (Problems)

```
┌─────────────────────────────────────────┐
│  通道健康状态详情                        │
├─────────────────────────────────────────┤
│  状态: [已暂停]                          │  ← Buried in flat list
│  连续高失败率周期: 5 / 10                │
│  当前窗口失败率: 45.23%                  │  ← Critical data, no emphasis
│  窗口请求数: 100 请求 (45 失败)          │
│  暂停次数: 第 2 次暂停                   │
│  冷却时间: 还剩 3 分钟 (5分钟)           │  ← Important, needs visibility
│  最后成功时间: 5 分钟前                  │
│  最后失败时间: 1 分钟前                  │
│  总请求数: 1,234                         │
│  成功次数: 678                           │
│  失败次数: 556                           │
│  成功率: 54.94%                          │
└─────────────────────────────────────────┘
```

Problems:
- All data has equal visual weight
- Critical alerts (suspension) blend in
- No color coding for thresholds
- Hard to scan quickly

### New Layout Design (Solution)

```
┌─────────────────────────────────────────────────────────┐
│  通道健康状态详情                                        │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────┐   │
│  │  ⚠️  已暂停                      [重置健康状态]  │   │ ← Status Header
│  │  当前窗口失败率: 45.23%                          │   │
│  │  连续失败: ███████░░░ 5/10                      │   │
│  └─────────────────────────────────────────────────┘   │
│                                                          │
│  ┌─────────────────────────────────────────────────┐   │
│  │  ⏱️  冷却中 - 还剩 3 分钟                        │   │ ← Alert Banner
│  │  ████████████████░░░░░░░░░░ 60%                 │   │   (if suspended)
│  │  预计恢复时间: 14:23                             │   │
│  └─────────────────────────────────────────────────┘   │
│                                                          │
│  ┌─────────────┬─────────────┬─────────────┐          │
│  │ ✅ 成功次数 │ ❌ 失败次数 │ 📊 成功率   │          │ ← Metrics Cards
│  │    678      │    556      │   54.94%    │          │
│  └─────────────┴─────────────┴─────────────┘          │
│                                                          │
│  📈 窗口统计 (最近 100 次请求)              ▼          │ ← Collapsible
│  ├─ 窗口请求数: 100                                     │   Details
│  ├─ 窗口失败数: 45                                      │
│  ├─ 最后成功: 5 分钟前                                  │
│  └─ 最后失败: 1 分钟前                                  │
│                                                          │
│  📊 历史统计                                 ▼          │
│  ├─ 总请求数: 1,234                                     │
│  ├─ 暂停次数: 2                                         │
│  └─ ...                                                  │
└─────────────────────────────────────────────────────────┘
```

Benefits:
- ✅ Visual hierarchy: Critical info at top
- ✅ Color coding: Red for danger, orange for warning, green for healthy
- ✅ Progressive disclosure: Details collapsed by default
- ✅ Scannable: Key metrics visible at glance

### Component Structure

```javascript
<Modal>
  {/* Top: Status Summary */}
  <StatusCard status={status} failureRate={current_failure_rate}>
    <LargeMetric value={failureRate} threshold={30} />
    <ProgressBar value={consecutive_failures} max={10} />
  </StatusCard>

  {/* Middle: Suspension Alert (conditional) */}
  {is_suspended && (
    <Banner type="warning" fullMode icon={<IconClock />}>
      <CountdownTimer target={suspended_until} />
      <Progress percent={cooldownProgress} />
    </Banner>
  )}

  {/* Bottom: Metrics Grid */}
  <Row gutter={16}>
    <Col span={8}>
      <Statistic title="成功次数" value={total_successes} prefix={<IconTickCircle />} />
    </Col>
    <Col span={8}>
      <Statistic title="失败次数" value={total_failures} prefix={<IconAlertTriangle />} />
    </Col>
    <Col span={8}>
      <Statistic title="成功率" value={successRate} suffix="%" />
    </Col>
  </Row>

  {/* Collapsible Details */}
  <Collapse defaultActiveKey={[]}>
    <Panel header="窗口统计" itemKey="window">
      <WindowMetrics {...windowStats} />
    </Panel>
    <Panel header="历史统计" itemKey="history">
      <HistoricalMetrics {...historicalStats} />
    </Panel>
  </Collapse>
</Modal>
```

### Color Coding System

| Metric              | Green (Healthy) | Yellow (Warning) | Red (Critical) |
|---------------------|-----------------|------------------|----------------|
| Failure Rate        | < 10%           | 10-30%           | > 30%          |
| Consecutive Failures| 0               | 1-5              | 6-10           |
| Status              | 正常            | 警告             | 已暂停         |

### Semi Design Components Used

- `Card`: Status summary container
- `Banner`: Suspension alert with countdown
- `Statistic`: Large metric numbers with icons
- `Progress`: Cooldown progress, failure count bar
- `Collapse`: Progressive disclosure for detailed stats
- `Row/Col`: Responsive grid layout
- `Tag`: Status badges
- `Space`: Consistent spacing

## Data Flow

Both capabilities operate on existing data:

**Onboarding:**
```
localStorage (onboarding_progress)
  ↓
useOnboarding hook
  ↓
OnboardingWizard component
  ↓
UI (always step 1 on open)
```

**Channel Health:**
```
Backend API (unchanged)
  ↓
ChannelHealthModal props
  ↓
New layout components
  ↓
UI (enhanced visuals)
```

No new API calls, no new data structures.

## Testing Strategy

### Unit Tests (Optional - Add if project adopts testing)
- `useOnboarding.test.js`: Verify `currentStep` resets on modal close
- `ChannelHealthModal.test.js`: Verify correct status colors for thresholds

### Manual Testing Checklist

**Onboarding Restart:**
- [ ] Open wizard from Help menu → Verify starts at step 1
- [ ] Complete step 1, close, reopen → Verify back to step 1
- [ ] Complete all steps → Verify completion state persists
- [ ] Open wizard, skip step 1 with "don't show again" → Verify dismissed state persists

**Channel Health Modal:**
- [ ] Healthy channel (0 failures) → Green status, no banner
- [ ] Warning channel (3 consecutive failures) → Yellow status, no banner
- [ ] Suspended channel → Red status, orange banner with countdown
- [ ] Failure rate >30% → Red text on failure rate metric
- [ ] Collapse/expand detail sections → Verify smooth animation

### Browser Compatibility
- Chrome/Edge (latest)
- Firefox (latest)
- Safari (latest)
- Mobile browsers (responsive design check)

## Performance Considerations

### Onboarding Restart
- **Cost**: 1 localStorage write on modal close
- **Benefit**: Simpler logic (remove restore logic) may actually improve performance

### Channel Health Modal
- **Before**: 12+ `<Descriptions.Item>` components rendered as table cells
- **After**:
  - 1 Card
  - 1 Banner (conditional)
  - 3 Statistic components
  - 2 Collapse panels (lazy rendered)
- **Result**: Comparable or better rendering performance

No performance regressions expected.

## Accessibility

**Onboarding:**
- Modal still supports `Esc` key to close
- Focus management unchanged

**Channel Health:**
- Color coding supplemented with icons (not color-only)
- `Statistic` components have proper ARIA labels
- `Collapse` panels keyboard-navigable
- Screen reader announces status changes

## Internationalization (i18n)

All UI text should support existing i18n infrastructure:

**New translation keys needed:**
```javascript
{
  "channel.health.status_summary": "状态摘要",
  "channel.health.cooling_down": "冷却中 - 还剩 {time}",
  "channel.health.window_stats": "窗口统计",
  "channel.health.historical_stats": "历史统计",
  "channel.health.estimated_recovery": "预计恢复时间",
  // ... etc
}
```

## Rollback Plan

Both changes are frontend-only:

1. Revert Git commit
2. Rebuild frontend bundle
3. Deploy previous version

No database migrations, no backend changes, no data loss risk.

## Open Questions

1. **Should we add a manual "Reset Progress" button in onboarding?**
   - Decision: Not in initial version. Auto-reset on reopen is sufficient.

2. **Should channel health modal show historical trends (chart)?**
   - Decision: Not in initial version. Current point-in-time data is sufficient. Can add later if users request it.

3. **Should we preserve scroll position in health modal if user collapses/expands sections?**
   - Decision: Yes, use Semi Design's default Collapse behavior (preserves scroll).
