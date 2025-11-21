# Spec: Enhanced Channel Health Display

## ADDED Requirements

### Requirement: Channel health modal SHALL display health status with visual hierarchy

The channel health modal SHALL present information in a visually hierarchical layout that emphasizes critical metrics and uses color coding to indicate severity levels.

#### Scenario: Healthy channel shows green status with minimal warnings

**Given:**
- Channel has health data:
  ```json
  {
    "is_suspended": false,
    "consecutive_failures": 0,
    "current_failure_rate": 0.05,
    "total_successes": 950,
    "total_failures": 50,
    "window_total_requests": 100,
    "window_failure_count": 5
  }
  ```

**When:**
- User clicks on health status badge in channel list

**Then:**
- Modal opens with layout:
  - **Top section** (Status Card):
    - Large green badge: "✅ 正常"
    - Failure rate: "5.00%" in normal gray text
    - Consecutive failures: Progress bar showing 0/10 in green
  - **No alert banner** (only shown when suspended)
  - **Metrics cards**:
    - Success: 950 with green check icon
    - Failures: 50 with gray warning icon (not red, below threshold)
    - Success rate: 95.00% in green text
  - **Collapsed details sections**:
    - Window statistics (collapsed by default)
    - Historical statistics (collapsed by default)

#### Scenario: Warning channel shows yellow status with visible alerts

**Given:**
- Channel has health data:
  ```json
  {
    "is_suspended": false,
    "consecutive_failures": 5,
    "current_failure_rate": 0.25,
    "window_failure_count": 25
  }
  ```

**When:**
- User clicks on health status badge

**Then:**
- Modal shows:
  - **Status Card**:
    - Orange badge: "⚠️ 警告"
    - Failure rate: "25.00%" in orange text (warning threshold 10-30%)
    - Consecutive failures: Progress bar showing 5/10 in orange (50%)
  - **No suspension banner** (not suspended yet)
  - **Metrics cards** with orange accents
  - User can see channel is approaching suspension (5/10 threshold)

#### Scenario: Suspended channel shows critical alert banner

**Given:**
- Channel is suspended:
  ```json
  {
    "is_suspended": true,
    "suspended_until": "2025-11-20T15:30:00Z", // 3 minutes from now
    "suspension_count": 2,
    "consecutive_failures": 10,
    "current_failure_rate": 0.45
  }
  ```

**When:**
- User clicks on health status badge
- Current time is 15:27:00

**Then:**
- Modal shows:
  - **Status Card**:
    - Red badge: "🔴 已暂停"
    - Failure rate: "45.00%" in red text (critical threshold >30%)
    - Consecutive failures: Progress bar showing 10/10 in red (100%)
    - "重置健康状态" button visible in status card
  - **Alert Banner** (prominent orange banner):
    - Icon: ⏱️ clock icon
    - Text: "冷却中 - 还剩 3 分钟"
    - Countdown timer updates in real-time
    - Progress bar showing cooldown progress (e.g., 60% complete if 3 min into 5 min cooldown)
    - Estimated recovery time: "预计恢复时间: 15:30"
  - **Suspension info**:
    - "第 2 次暂停" displayed in banner or status card
    - Cooldown duration: "5分钟" (calculated from suspension_count)

---

### Requirement: Metrics SHALL be color-coded based on thresholds

Metrics SHALL be color-coded based on thresholds to provide instant visual feedback about channel health severity.

#### Scenario: Failure rate color changes based on threshold

**Given:**
- User views three different channels

**When:**
- Channel A has `current_failure_rate: 0.05` (5%)
- Channel B has `current_failure_rate: 0.20` (20%)
- Channel C has `current_failure_rate: 0.40` (40%)

**Then:**
- Channel A modal shows failure rate in **green** text (`--semi-color-success`)
- Channel B modal shows failure rate in **orange** text (`--semi-color-warning`)
- Channel C modal shows failure rate in **red** text (`--semi-color-danger`)

**Color Thresholds:**
| Metric            | Green       | Orange      | Red         |
|-------------------|-------------|-------------|-------------|
| Failure Rate      | < 10%       | 10% - 30%   | > 30%       |
| Consecutive Fails | 0           | 1-5         | 6-10        |

#### Scenario: Consecutive failure progress bar uses color coding

**Given:**
- Channel has `consecutive_failures: 7`

**When:**
- Modal renders progress bar for consecutive failures

**Then:**
- Progress bar displays:
  - Label: "连续失败: 7 / 10"
  - Bar filled to 70%
  - Bar color: red (`--semi-color-danger`)
  - Tooltip on hover: "达到 10 次将自动暂停渠道"

**Color Mapping:**
- 0 failures: Green bar
- 1-5 failures: Orange bar
- 6-10 failures: Red bar

---

### Requirement: System SHALL display real-time countdown timer for suspended channels

When a channel is suspended, the system SHALL display a real-time countdown timer showing remaining cooldown time with a visual progress bar.

#### Scenario: Countdown timer updates in real-time

**Given:**
- Channel is suspended until `suspended_until: "2025-11-20T15:30:00Z"`
- Current time is 15:27:00 (3 minutes remaining)
- User has modal open

**When:**
- Modal is displayed
- Time passes (10 seconds)

**Then:**
- Initial display: "冷却中 - 还剩 3 分钟"
- After 10 seconds: "冷却中 - 还剩 2 分钟 50 秒"
- After 60 seconds: "冷却中 - 还剩 2 分钟"
- Timer updates every 1 second
- Progress bar animates smoothly from 60% → 65% → ... as time passes

**Implementation Note:**
- Use `setInterval` to update countdown every 1000ms
- Clean up interval on modal close to prevent memory leak
- Use `date-fns` `formatDistanceToNow` for human-readable time

#### Scenario: Countdown completes and channel auto-recovers

**Given:**
- Channel suspended until 15:30:00
- User has modal open
- Current time reaches 15:30:00

**When:**
- Countdown reaches zero

**Then:**
- Timer shows: "冷却完成"
- Alert banner changes to success state (green): "✅ 渠道已恢复，可以重新使用"
- Status badge updates to "正常" (may require manual refresh)
- User can close modal and channel should be active

**Note:** Auto-refresh of channel list not in scope for this change. User may need to manually refresh channel list to see updated status.

---

### Requirement: Detailed metrics SHALL be placed in collapsible sections

Detailed metrics (window statistics, historical data) SHALL be placed in collapsible sections to reduce visual clutter and allow progressive disclosure.

#### Scenario: Modal opens with details collapsed by default

**Given:**
- User clicks on channel health status

**When:**
- Modal opens

**Then:**
- Collapsible sections are rendered:
  - "📈 窗口统计 (最近 100 次请求)" - Collapsed with ▼ icon
  - "📊 历史统计" - Collapsed with ▼ icon
- User can see summary metrics without scrolling
- Modal height is compact (~400px instead of ~600px)

#### Scenario: User expands window statistics section

**Given:**
- Modal is open with details collapsed

**When:**
- User clicks on "📈 窗口统计 (最近 100 次请求)" header

**Then:**
- Section expands with smooth animation
- Displays:
  - 窗口请求数: 100
  - 窗口失败数: 45
  - 窗口失败率: 45.00%
  - 最后成功时间: 5 分钟前
  - 最后失败时间: 1 分钟前
- Icon changes from ▼ to ▲
- Other sections remain collapsed
- Expanded state is NOT persisted (resets on modal reopen)

---

### Requirement: Primary metrics SHALL be displayed in card-based layout with icons

Primary metrics (success count, failure count, success rate) SHALL be displayed using Semi Design's `Statistic` component in a responsive grid layout with meaningful icons.

#### Scenario: Metrics grid renders responsively

**Given:**
- Modal width is 700px (desktop)

**When:**
- Metrics section renders

**Then:**
- Three cards displayed in a row:
  ```
  ┌─────────────┬─────────────┬─────────────┐
  │ ✅ 成功次数 │ ❌ 失败次数 │ 📊 成功率   │
  │    1,234    │     567     │   68.52%    │
  └─────────────┴─────────────┴─────────────┘
  ```
- Each card uses `<Statistic>` component
- Icons use Semi Icons: `IconTickCircle`, `IconAlertTriangle`, `IconActivity`
- Numbers formatted with thousand separators: `1,234` not `1234`
- Success rate shows 2 decimal places: `68.52%` not `68.5%`

**Mobile Responsive (width < 600px):**
- Cards stack vertically
- Each card full width
- Font sizes slightly smaller

---

## MODIFIED Requirements

### Requirement: Modal SHALL use hierarchical layout instead of flat Descriptions

The modal SHALL replace the flat `<Descriptions>` list with a hierarchical layout consisting of status card, alert banner, metric cards, and collapsible details sections.

#### Scenario: Code migration from Descriptions to new layout

**Given:**
- Current implementation in `ChannelHealthModal.jsx` lines 144-235:
  ```jsx
  <Descriptions row size="medium">
    <Descriptions.Item itemKey="状态">...</Descriptions.Item>
    <Descriptions.Item itemKey="连续高失败率周期">...</Descriptions.Item>
    // ... 12 more items
  </Descriptions>
  ```

**When:**
- Code is refactored to new layout

**Then:**
- New implementation structure:
  ```jsx
  <Modal>
    {/* Top: Status Summary Card */}
    <Card>
      <StatusBadge />
      <LargeMetric label="当前失败率" value={failureRate} />
      <Progress label="连续失败" value={consecutive_failures} max={10} />
    </Card>

    {/* Middle: Suspension Alert (conditional) */}
    {is_suspended && (
      <Banner type="warning">
        <CountdownTimer target={suspended_until} />
      </Banner>
    )}

    {/* Bottom: Metrics Grid */}
    <Row gutter={16}>
      <Col span={8}><Statistic title="成功次数" value={total_successes} /></Col>
      <Col span={8}><Statistic title="失败次数" value={total_failures} /></Col>
      <Col span={8}><Statistic title="成功率" value={successRate} /></Col>
    </Row>

    {/* Collapsible Details */}
    <Collapse>
      <Panel header="窗口统计">...</Panel>
      <Panel header="历史统计">...</Panel>
    </Collapse>
  </Modal>
  ```
- `Descriptions` component completely removed
- New components: `Card`, `Banner`, `Statistic`, `Collapse`

---

## REMOVED Requirements

### Requirement: Display all metrics in single flat list
**Status:** REMOVED
**Reason:** Replaced by hierarchical layout with visual emphasis

The previous requirement of displaying all metrics in a single `<Descriptions>` list is removed in favor of the new hierarchical design that prioritizes critical information.

---

## Cross-Capability Dependencies

This capability is independent of the "Onboarding Restart" capability. Both can be implemented in parallel.

**External Dependencies:**
- None. Uses existing backend API `/api/channel/:id` (no changes needed)
- All data already provided in health object passed to modal

---

## Validation Criteria

### Functional Validation
- [ ] Healthy channel (0 failures) → Green status, no banner
- [ ] Warning channel (5 failures) → Orange status, no banner
- [ ] Suspended channel → Red status, orange banner with countdown
- [ ] Countdown timer updates every second
- [ ] Countdown completes → Success message appears
- [ ] Collapsible sections expand/collapse smoothly
- [ ] Metrics cards show correct numbers with thousand separators

### Visual Validation
- [ ] Failure rate <10% → Green text
- [ ] Failure rate 10-30% → Orange text
- [ ] Failure rate >30% → Red text
- [ ] Progress bar 0 failures → Green
- [ ] Progress bar 1-5 failures → Orange
- [ ] Progress bar 6-10 failures → Red
- [ ] Icons render correctly (✅, ❌, ⏱️, etc.)
- [ ] Layout responsive on mobile (cards stack)

### Non-Functional Validation
- [ ] Modal renders in <100ms
- [ ] No memory leaks (countdown interval cleaned up)
- [ ] No console errors or warnings
- [ ] Works in Chrome, Firefox, Safari

### Regression Prevention
- [ ] "重置健康状态" button still works correctly
- [ ] Manual health reset API call succeeds
- [ ] Modal closes correctly with Esc or X button
- [ ] Health status badge in channel list still clickable
