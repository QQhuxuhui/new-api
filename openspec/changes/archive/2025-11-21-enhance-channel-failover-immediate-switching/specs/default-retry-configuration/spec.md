# Default Retry Configuration

## MODIFIED Requirements

### Requirement: Enable failover by default for new installations

The system SHALL set `RetryTimes = 2` as the default value instead of 0, enabling automatic channel failover for new installations without requiring manual configuration. Existing installations with environment variable overrides SHALL preserve their configured behavior.

**Previous Behavior**: `RetryTimes = 0` (no retry/failover by default)

**New Behavior**: `RetryTimes = 2` (enable basic failover out-of-the-box)

#### Scenario: Fresh installation has failover enabled

**Given**:
- New installation of the system
- No custom `RETRY_TIMES` environment variable set
- Default configuration loaded from `common/constants.go`

**When**:
- System starts
- Configuration is initialized

**Then**:
- `RetryTimes` variable equals `2`
- User can access up to 3 channels per request (1 initial + 2 retries)
- Failover works without manual configuration

**Validation**:
- Check `common.RetryTimes == 2` at runtime
- No environment variable override needed
- Backward compatible: ENV var still overrides default

---

#### Scenario: Existing installations preserve custom configuration

**Given**:
- Existing installation with `RETRY_TIMES=0` in environment
- System upgrade to new version

**When**:
- System restarts with environment variable set

**Then**:
- `RetryTimes` equals `0` (environment override)
- Behavior unchanged from previous version
- User retains explicit no-retry preference

**Validation**:
- Environment variable takes precedence
- No forced behavior change for existing users
- Migration guide documents new default

---

### Requirement: Retry configuration supports priority-based channel selection

The retry mechanism MUST work correctly with the existing priority-based channel selection system.

#### Scenario: RetryTimes=2 allows 3 priority levels

**Given**:
- Model has channels at priority 0, 1, 2
- `RetryTimes = 2`
- Priority 0 channel fails

**When**:
- Request fails on priority 0
- `shouldRetry` returns `true`
- Retry loop continues with `retryCount=1`

**Then**:
- System selects channel from priority 1
- If priority 1 fails, tries priority 2
- Up to 3 channels total (1 initial + 2 retries)

**Validation**:
- `getChannel(c, group, originalModel, retryCount)` called with retryCount 0, 1, 2
- Each retry attempts next priority level
- Loop terminates after RetryTimes exhausted or success

---

#### Scenario: RetryTimes=2 with fewer priorities still works

**Given**:
- Model has only 1 priority level (priority 0)
- Multiple channels at priority 0
- `RetryTimes = 2`

**When**:
- First channel fails
- Retry attempts made

**Then**:
- System selects different channel at same priority
- Retries up to 2 times at priority 0
- Random channel selection within priority

**Validation**:
- No error when priority levels < RetryTimes
- Channel selection handles priority exhaustion gracefully
- Error returned after all channels attempted

---

## ADDED Requirements

### Requirement: Document default retry behavior in release notes

The change in default `RetryTimes` MUST be documented for users upgrading from previous versions.

#### Scenario: Release notes warn about behavior change

**Given**:
- User reads release notes for version with this change

**When**:
- User reviews "Breaking Changes" or "Important Changes" section

**Then**:
- Release notes contain entry:
  - **Title**: "Default retry behavior changed to improve failover"
  - **Description**: RetryTimes default changed from 0 to 2
  - **Action Required**: Set `RETRY_TIMES=0` in environment if you want to preserve no-retry behavior
  - **Rationale**: Improves out-of-box experience and system resilience

**Validation**:
- Release notes include migration guide
- Environment variable override documented
- No silent behavior change

---

## Cross-References

- **Depends on**: None
- **Related to**:
  - `optimized-retry-logic` (retry logic respects new default)
  - `immediate-failover-detection` (works with or without retry enabled)
