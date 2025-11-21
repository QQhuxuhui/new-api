# Proposal: Distributed Channel Health Tracking

## Summary

Add Redis-based distributed channel health tracking with sliding window failure rate detection, smart exponential backoff suspension, manual recovery controls, and UI visibility for channel availability status.

## Why

Current channel failover system has a critical limitation: it immediately and permanently disables channels on certain errors without distinguishing between transient failures and persistent issues. This can lead to:

1. **False-positive disables**: A single transient error (network hiccup, temporary overload) permanently disables healthy channels
2. **No distributed state**: Multiple instances can have inconsistent views of channel health
3. **No automatic recovery**: Channels remain disabled until manual intervention
4. **Invisible health state**: Administrators cannot see channel health metrics or suspension status
5. **Single-user problem**: Individual user's client bug or small number of problematic users can trigger channel suspension affecting all users

The aggressive failover detection logic (added in previous commits) correctly identifies failure scenarios but needs a failure rate mechanism to prevent false positives from individual users while maintaining aggressive detection of real channel failures.

## Problem Statement

**Current Behavior:**
- Single failure matching `shouldTriggerChannelFailover()` → immediate permanent disable
- No retry mechanism at channel level (only priority degradation)
- No visibility into channel health metrics
- Manual re-enable required for every false positive
- **Critical**: Single user with bug (60 failures/min) among 9 normal users (540 successes/min) → channel suspended → all 10 users affected

**User Impact:**
- Healthy channels get disabled unnecessarily
- Service availability degrades due to reduced channel pool
- Administrators spend time investigating and re-enabling channels
- No data-driven insight into channel reliability
- **Single user problems cascade to affect entire user base**

## Proposed Solution

Implement a 3-tier failure rate-based tracking system with sliding window detection:

### Core Innovation: Sliding Window Failure Rate Detection

**Problem Solved**: Distinguishes between channel-level failures and user-level failures

**Mechanism**:
- **60-second sliding window** divided into 6 buckets (10 seconds each)
- Track **all requests** (successes + failures) in bucket-based counters
- Calculate **failure rate** = failures / total_requests
- Only count as "high failure period" when rate exceeds threshold

**Example Scenarios**:
- **Single user bug**: 60 failures + 540 successes = 10% rate → NOT counted ✅
- **Real channel failure**: 298 failures + 2 successes = 99% rate → Counted and suspended ✅
- **Low traffic**: Special handling with 5+ failures and >80% rate → Counted

**Thresholds**:
- Standard traffic (≥30 req/min): 30% failure rate threshold
- Low traffic (<30 req/min): 50% failure rate threshold
- Minimum sample size: 10 requests before evaluation

### Health State Tiers

1. **Normal Operation** (0-2 consecutive high-failure-rate periods)
   - Channel operates normally
   - Single high-failure-rate period → failover to next channel
   - Next period with normal rate → reset failure counter
   - Success requests dilute failure rate

2. **Temporary Suspension** (3-9 consecutive high-failure-rate periods)
   - Suspend channel with exponential backoff cooldown:
     - 1st suspension: 5 minutes
     - 2nd suspension: 10 minutes
     - 3rd suspension: 20 minutes
     - 4th suspension: 40 minutes
     - 5th+ suspension: 60 minutes (capped)
   - Channel excluded from selection during suspension
   - After cooldown → automatically re-enable
   - If immediately fails again → extend suspension with longer cooldown

3. **Permanent Disable** (10+ consecutive high-failure-rate periods)
   - Mark channel as permanently disabled
   - Requires manual administrator intervention
   - Logged for investigation

### Implementation Approach

- **Backend**: Redis-based health tracking for distributed state
- **Frontend**: Channel list status column + health detail modal
- **Integration**: Minimal changes to existing selection/error logic

### Key Features

1. **Distributed State Management**
   - Redis as single source of truth for channel health
   - Atomic operations for failure counting
   - TTL-based automatic suspension expiry

2. **Sliding Window Failure Rate Detection**
   - 60-second window with 10-second bucket granularity
   - Tracks all requests (not just failures) for statistical accuracy
   - Automatic filtering of single-user problems
   - Dynamic threshold adjustment for low-traffic scenarios
   - Minimal storage overhead: 96 bytes per channel

3. **Automatic Recovery**
   - No manual intervention needed for transient issues
   - Smart exponential backoff: increases cooldown for persistent problems
   - Clear threshold for permanent disable

4. **Manual Recovery Controls**
   - Administrators can manually reset channel health status
   - Complete reset: clears failure counters and suspension state
   - Accessible via UI button in health detail modal
   - Preserves historical statistics for analysis

5. **UI Visibility**
   - Real-time channel availability status in list
   - Detailed health metrics in modal:
     - Consecutive high-failure-rate period count
     - Current window failure rate
     - Suspension status and remaining cooldown
     - Last success/failure timestamps
     - Historical health data
     - Manual recovery button (when applicable)

## Success Criteria

1. **No False Positives**: Single-user problems do not trigger channel suspension
2. **Single-User Immunity**: Single user's 60 failures/min among 540 successes/min from other users → channel remains available
3. **Quick Detection**: Real channel failures (>30% failure rate) detected within 30 seconds
4. **Automatic Recovery**: Temporarily suspended channels automatically re-enable after cooldown
5. **Smart Cooldown**: Exponential backoff reduces retry frequency for persistent issues (5min → 60min max)
6. **Manual Control**: Administrators can instantly reset channel health via UI
7. **Distributed Consistency**: All instances see identical channel health state
8. **Operator Visibility**: Administrators can view real-time health metrics and failure rates
9. **Minimal Latency Impact**: Health check adds <5ms to channel selection
10. **Backward Compatible**: Existing manual disable/enable still works

## Non-Goals

- Replace existing error classification logic (`shouldTriggerChannelFailover`)
- Change channel selection algorithm (weighted random)
- Implement channel-level load balancing
- Track health metrics for non-failover errors (user quota, invalid requests, etc.)
- Implement user-level failure tracking (out of scope)

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Redis failure causes all channels to appear unavailable | Fall back to in-memory state if Redis unavailable; log warning |
| Race conditions in failure counting | Use Redis atomic operations (INCR, SET NX) |
| Clock skew in distributed environment | Use Redis server time for TTL calculations |
| Performance impact on high-traffic systems | Use bucket-based storage (96 bytes/channel); batch Redis operations |
| Bucket rotation during low traffic | Use minimum sample size (10 requests) before evaluation; special low-traffic handling |
| Storage overhead for sliding window | Use bucket expiry (120s TTL); only 12 keys per active channel |

## Dependencies

- Existing Redis integration (common/redis.go)
- Existing channel selection logic (model/channel_cache.go)
- Existing error classification (service/error.go)

## Timeline Estimate

- Backend implementation: 2-3 days
- Frontend implementation: 1-2 days
- Testing and validation: 1-2 days
- **Total**: ~5-7 days
