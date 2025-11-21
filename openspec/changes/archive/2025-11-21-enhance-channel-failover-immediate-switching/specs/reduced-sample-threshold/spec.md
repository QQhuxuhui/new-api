# Reduced Sample Threshold

## MODIFIED Requirements

### Requirement: Lower minimum sample size for faster failure detection

The system SHALL reduce `MinSampleSize` from 10 to 5, enabling statistical failure rate evaluation with fewer samples and reducing the number of user-impacting failures before protection mechanisms activate.

**Previous Behavior**: `MinSampleSize = 10` (requires 10 requests before evaluation)

**New Behavior**: `MinSampleSize = 5` (faster detection, fewer user-impacting failures)

#### Scenario: Statistical evaluation triggers at 5 samples

**Given**:
- Channel has 4 failed requests and 1 successful request (5 total)
- Failure rate = 80% (4/5)
- `FailureRateThreshold = 30%`

**When**:
- `IsHighFailureRate(channelID)` is called

**Then**:
- `totalCount >= MinSampleSize` check passes (5 >= 5)
- Failure rate calculated: 80%
- Function returns `(true, 0.80, "失败率80.00%超过阈值30.00%...")`
- Channel failure counter incremented

**Validation**:
- No "样本数不足" message
- Statistical analysis executes
- Failure tracking active

---

#### Scenario: Sample threshold reduction limits user-impacting failures

**Given**:
- Channel experiences degraded performance
- Previous threshold: 10 samples → 9 failures impact users
- New threshold: 5 samples → 4 failures impact users

**When**:
- Channel fails 5 times in sliding window

**Then**:
- 5th failure triggers statistical evaluation
- Channel suspended if failure rate > 30%
- Subsequent requests use backup channels
- User exposure reduced by ~55% (4 vs 9 failures)

**Validation**:
- Failures 1-4 returned to users (unavoidable without immediate detection)
- Failure 5 triggers protection
- Failures 6+ use backup channels

---

### Requirement: Preserve low-traffic special handling

The existing low-traffic special case MUST continue working with new threshold.

#### Scenario: Low-traffic handling still applies below 5 samples

**Given**:
- Channel has 3 total requests in window
- All 3 are failures (100% failure rate)
- `LowTrafficMinFailures = 5` (unchanged)

**When**:
- `IsHighFailureRate(channelID)` is called

**Then**:
- `totalCount < MinSampleSize` check fails (3 < 5)
- Low-traffic handling evaluates: `failureCount >= 5` → false (3 < 5)
- Function returns `(false, 0, "样本数不足: 3 < 5")`
- No suspension triggered yet

**Validation**:
- Low-traffic threshold still 5 failures
- Prevents false positives for very low traffic
- 5-sample threshold doesn't create new edge cases

---

#### Scenario: Low-traffic special case triggers at 5 failures

**Given**:
- Channel has 5 failures, 0 successes (5 total)
- Failure rate = 100%
- `LowTrafficFailureRate = 80%`

**When**:
- `IsHighFailureRate(channelID)` is called

**Then**:
- Standard evaluation: `totalCount >= MinSampleSize` → true (5 >= 5)
- Failure rate 100% > 30% threshold
- Returns `(true, 1.0, "失败率100.00%超过阈值30.00%...")`
- Channel suspended

**Validation**:
- Both paths work: standard evaluation OR low-traffic special case
- No logic conflicts
- Conservative protection maintained

---

### Requirement: Maintain 30% failure rate threshold

The `FailureRateThreshold` of 30% MUST remain unchanged to prevent false positives.

#### Scenario: 5 samples with 1 failure does not trigger suspension

**Given**:
- Channel: 4 successes, 1 failure (5 total)
- Failure rate = 20% (1/5)

**When**:
- `IsHighFailureRate(channelID)` is called

**Then**:
- `failureRate <= FailureRateThreshold` check: 0.20 <= 0.30
- Returns `(false, 0.20, "失败率20.00%正常 (窗口: 5请求)")`
- No suspension

**Validation**:
- Threshold unchanged at 30%
- Lower sample size doesn't increase false positives
- Requires 2+ failures in 5 samples to trigger (40%+)

---

#### Scenario: 5 samples with 2 failures triggers suspension

**Given**:
- Channel: 3 successes, 2 failures (5 total)
- Failure rate = 40% (2/5)

**When**:
- `IsHighFailureRate(channelID)` is called

**Then**:
- `failureRate > FailureRateThreshold` check: 0.40 > 0.30
- Returns `(true, 0.40, "失败率40.00%超过阈值30.00% (窗口: 5请求)")`
- Consecutive failure counter incremented

**Validation**:
- 2/5 failures = 40% triggers suspension
- Threshold sensitivity unchanged
- Faster detection without increased false positives

---

## ADDED Requirements

None. This is a pure threshold reduction with existing logic preserved.

---

## REMOVED Requirements

None. All existing requirements maintained.

---

## Cross-References

- **Depends on**: None
- **Related to**:
  - `immediate-failover-detection` (handles critical errors before sample collection)
  - `optimized-retry-logic` (ensures detected failures trigger retry)
- **Preserves**: Low-traffic handling, 30% threshold, exponential backoff
