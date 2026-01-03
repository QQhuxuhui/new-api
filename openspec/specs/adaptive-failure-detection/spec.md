# adaptive-failure-detection Specification

## Purpose
TBD - created by archiving change improve-low-traffic-error-handling. Update Purpose after archive.
## Requirements
### Requirement: Low-traffic scenarios MUST use reduced sample threshold with elevated failure rate

When a channel's sliding window contains fewer than `MinSampleSize` (5) samples but at least 2 samples, the system MUST evaluate failure rate using an 80% threshold instead of requiring full sample accumulation.

#### Scenario: Two consecutive failures trigger high failure rate detection

**Given**:
- Channel health configured with `MinSampleSize=5`
- 60-second sliding window currently empty
- Low-traffic deployment (~5 requests/hour)

**When**:
1. T+0:00 - Request 1 routed to Channel A → 504 timeout
   - `RecordChannelRequest(A, false)` → `totalCount=1, failureCount=1`
   - `IsHighFailureRate(A)` → `(false, 0, "样本数不足: 1 < 5")`
2. T+0:15 - Request 2 routed to Channel A → 504 timeout
   - `RecordChannelRequest(A, false)` → `totalCount=2, failureCount=2`
   - `IsHighFailureRate(A)` → Evaluates new low-traffic logic

**Then**:
- Condition `totalCount >= 2 && failureCount >= 2` satisfied
- Calculated rate: `2/2 = 100%` which exceeds 80% threshold
- Function returns `(true, 1.0, "低流量高失败率: 2/2=100% (快速识别)")`
- Failure counted toward consecutive failure threshold
- After 3 such periods, channel suspended

**Observable Behavior**:
```
[SYS] 2025/11/21 - 14:00:00 | Channel 4 failure NOT counted: 样本数不足: 1 < 5 (rate=0.00%)
[SYS] 2025/11/21 - 14:15:00 | Channel 4 high failure rate: 低流量高失败率: 2/2=100% (快速识别), counting consecutive period
[INFO] 2025/11/21 - 14:15:00 | record error log: channelId=4
```

#### Scenario: Mixed failure/success below 80% threshold does NOT trigger

**Given**:
- Channel with 2 samples in window: 1 failure, 1 success

**When**:
- `GetWindowStats(channelID)` returns `totalCount=2, failureCount=1`
- `IsHighFailureRate()` evaluates low-traffic logic

**Then**:
- Calculated rate: `1/2 = 50%` which is below 80% threshold
- Function returns `(false, 0, "样本数不足: 2 < 5")`
- Channel NOT marked as high failure rate
- No impact on consecutive failure counter

**Observable Behavior**:
```
[SYS] Channel 4 failure NOT counted: 样本数不足: 2 < 5 (rate=50.00%)
```

#### Scenario: Three samples with 67% failure rate does NOT trigger

**Given**:
- Channel with 3 samples: 2 failures, 1 success

**When**:
- `GetWindowStats(channelID)` returns `totalCount=3, failureCount=2`
- `IsHighFailureRate()` evaluates low-traffic logic

**Then**:
- Calculated rate: `2/3 = 66.67%` which is below 80% threshold
- Function returns `(false, 0, "样本数不足: 3 < 5")`
- System waits for more samples before decision

**Observable Behavior**:
```
[SYS] Channel 4 failure NOT counted: 样本数不足: 3 < 5 (rate=66.67%)
```

### Requirement: Transition from low-traffic to standard threshold MUST be seamless

When sample count reaches `MinSampleSize` (5), the system MUST transition from low-traffic detection (2+ samples, 80% rate) to standard detection (5+ samples, 30%/50% rate) without discontinuity in behavior.

#### Scenario: Sample count reaches threshold during evaluation

**Given**:
- Channel accumulating samples: 4 samples (3 failures, 1 success)
- Next request fails, bringing total to 5 samples (4 failures)

**When**:
1. Before 5th request: `totalCount=4, failureCount=3`
   - `IsHighFailureRate()` → `(false, 0, "样本数不足: 4 < 5")`
   - Rate 75% < 80%, not triggered
2. After 5th failure: `totalCount=5, failureCount=4`
   - `IsHighFailureRate()` → Skips low-traffic branch (`totalCount >= MinSampleSize`)
   - Evaluates standard threshold: `4/5 = 80% > 30%`
   - Returns `(true, 0.8, "失败率80.00%超过阈值30.00% (窗口: 5请求)")`

**Then**:
- Transition occurs naturally via conditional logic
- No gap in detection coverage
- Standard threshold (30%) applies immediately at 5 samples

**Observable Behavior**:
```
[SYS] Channel 4 failure NOT counted: 样本数不足: 4 < 5 (rate=75.00%)
[SYS] Channel 4 high failure rate: 失败率80.00%超过阈值30.00% (窗口: 5请求), counting consecutive period
```

#### Scenario: Low-traffic detection triggers before reaching standard threshold

**Given**:
- Channel with persistent failures in low-traffic environment

**When**:
1. Sample 1: Failure → "样本数不足: 1 < 5"
2. Sample 2: Failure → "低流量高失败率: 2/2=100%" → Counted
3. Window resets (60s expired)
4. Sample 1 (new window): Failure → "样本数不足: 1 < 5"
5. Sample 2 (new window): Failure → "低流量高失败率: 2/2=100%" → Counted
6. Repeat until 3 consecutive high-failure periods → Channel suspended

**Then**:
- Channel suspended after 6 total failures (3 windows × 2 failures each)
- Standard 5-sample threshold never reached
- Low-traffic detection provided early protection

**Observable Behavior**:
```
Window 1: [FAIL, FAIL] → Consecutive failures: 1
Window 2: [FAIL, FAIL] → Consecutive failures: 2
Window 3: [FAIL, FAIL] → Consecutive failures: 3
[SYS] Channel 4 suspended for 5m0s (suspension #1, 3 consecutive high-failure-rate periods)
```

### Requirement: Standard threshold logic MUST remain unchanged for normal traffic

When sample count equals or exceeds `MinSampleSize` (5), the existing 30%/50% failure rate thresholds MUST apply without modification.

#### Scenario: Normal traffic with 10 samples uses 30% threshold

**Given**:
- High-traffic channel: 10 requests in 60-second window
- 4 failures, 6 successes

**When**:
- `GetWindowStats(channelID)` returns `totalCount=10, failureCount=4`
- `IsHighFailureRate()` skips low-traffic logic (10 >= 5)
- Evaluates standard threshold

**Then**:
- Calculated rate: `4/10 = 40%`
- Threshold: `30%` (standard traffic, > LowTrafficThreshold)
- Rate exceeds threshold: `40% > 30%` → `true`
- Returns `(true, 0.4, "失败率40.00%超过阈值30.00% (窗口: 10请求)")`

**Observable Behavior**:
```
[SYS] Channel 4 high failure rate: 失败率40.00%超过阈值30.00% (窗口: 10请求), counting consecutive period
```

#### Scenario: Low traffic (6 samples) with 25% failure rate does NOT trigger

**Given**:
- Channel with 6 samples (borderline low-traffic): 2 failures, 4 successes

**When**:
- `GetWindowStats()` returns `totalCount=6, failureCount=2`
- Condition `totalCount >= MinSampleSize` satisfied → Standard logic applies
- Check: `totalCount < LowTrafficThreshold` (6 < 30) → Use 50% threshold
- Calculated rate: `2/6 = 33.33%`
- Compare: `33.33% > 50%` → `false`

**Then**:
- Returns `(false, 0.333, "失败率33.33%正常 (窗口: 6请求)")`
- Channel NOT marked as high failure

**Observable Behavior**:
```
[SYS] Channel 4 failure NOT counted: 失败率33.33%正常 (窗口: 6请求) (rate=33.33%)
```

### Requirement: Low-traffic detection MUST preserve existing special handling

The existing special handling for low-traffic channels (5+ failures with 80%+ rate) MUST continue to function as fallback when new 2-sample logic does not trigger.

#### Scenario: Four failures out of four samples triggers via existing logic

**Given**:
- Channel with 4 samples: 4 failures, 0 successes
- `LowTrafficMinFailures = 5` in constants

**When**:
- `GetWindowStats()` returns `totalCount=4, failureCount=4`
- New logic evaluates: `totalCount >= 2 && failureCount >= 2` → `true`
- Calculated rate: `4/4 = 100%` > 80% → Triggers immediately

**Then**:
- New low-traffic logic triggers before reaching existing logic
- Returns `(true, 1.0, "低流量高失败率: 4/4=100% (快速识别)")`
- Existing logic (5+ failures) not reached in this case

**Observable Behavior**:
```
[SYS] Channel 4 high failure rate: 低流量高失败率: 4/4=100% (快速识别), counting consecutive period
```

#### Scenario: Five failures with mixed successes triggers existing logic

**Given**:
- Channel with 8 samples: 5 failures, 3 successes (62.5% rate)

**When**:
- `GetWindowStats()` returns `totalCount=8, failureCount=5`
- New logic evaluates: `5/8 = 62.5%` < 80% → Does NOT trigger
- Existing special handling: `failureCount >= LowTrafficMinFailures` (5 >= 5) → `true`
- Check rate: `5/8 = 62.5%` < `LowTrafficFailureRate` (80%) → `false`

**Then**:
- Neither new nor existing special handling triggers
- Returns `(false, 0, "样本数不足: 8 < 5")` (contradiction in sample size, likely code path issue)
- **Note**: This exposes edge case where `totalCount >= MinSampleSize` should apply standard threshold

**Correction**: When `totalCount` between 5-9, should use standard threshold logic, not special handling.

**Expected Behavior**:
```
[SYS] Channel 4 failure NOT counted: 失败率62.50%正常 (窗口: 8请求)
```

