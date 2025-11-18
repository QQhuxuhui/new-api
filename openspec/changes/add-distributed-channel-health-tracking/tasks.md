# Tasks: Distributed Channel Health Tracking

## Phase 1: Backend - Service Layer

### Task 1.1: Create Health Tracking Service
**File**: `service/channel_health.go`

- [ ] Define constants (thresholds, key patterns, sliding window parameters, exponential backoff):
  - Window parameters: WindowDuration=60s, BucketSize=10s, BucketCount=6
  - Failure rate thresholds: FailureRateThreshold=0.30, FailureRateThresholdHigh=0.50
  - Sample parameters: MinSampleSize=10, LowTrafficThreshold=30
  - Health thresholds: SuspensionThreshold=3, DisableThreshold=10
  - Backoff parameters: BaseSuspensionMinutes=5.0, MaxSuspensionMinutes=60.0
  - Key patterns including bucket keys, suspension_count
- [ ] Define `ChannelHealth` struct with sliding window fields:
  - `SuspensionCount int` (for exponential backoff)
  - `CurrentFailureRate float64` (current window failure rate)
  - `WindowTotalRequests int64` (total requests in 60s window)
  - `WindowFailureCount int64` (failures in 60s window)
- [ ] Implement `RecordChannelRequest(channelID int, isSuccess bool)`:
  - Calculate bucket ID from current timestamp
  - INCR bucket total counter
  - INCR bucket failure counter (if failure)
  - SET bucket TTL to 120s (2x window duration)
- [ ] Implement `GetWindowStats(channelID int)` → (totalCount, failureCount):
  - Calculate current bucket ID
  - Sum last 6 buckets for total and failure counts
  - Return aggregated statistics
- [ ] Implement `IsHighFailureRate(channelID int)` → (isHigh bool, rate float64, reason string):
  - Get window stats
  - Check minimum sample size (10 requests)
  - Special handling for low traffic (5+ failures, 80% rate)
  - Calculate failure rate and compare with dynamic threshold
  - Return result with detailed reason
- [ ] Implement `RecordChannelFailure(channelID int)`:
  - Call `RecordChannelRequest(channelID, false)` to record to window
  - Call `IsHighFailureRate(channelID)` to check window state
  - If NOT high rate: log and return (do not increment consecutive failures)
  - If high rate: INCR consecutive failures counter
  - Record last_failure timestamp
  - INCR total_failures counter
  - Check thresholds (3 → suspend, 10 → disable)
- [ ] Implement `RecordChannelSuccess(channelID int)`:
  - Call `RecordChannelRequest(channelID, true)` to record to window
  - DEL consecutive failures counter
  - DEL suspension key
  - DEL suspension_count counter (reset on success)
  - Record last_success timestamp
  - INCR total_successes counter
- [ ] Implement `IsChannelAvailable(channelID int)`:
  - Check suspension key EXISTS
  - Return false if suspended, true otherwise
  - Fail open if Redis unavailable
- [ ] Implement `GetChannelHealth(channelID int)`:
  - Fetch all health metrics from Redis including suspension_count
  - Call `GetWindowStats()` for current window data
  - Calculate CurrentFailureRate from window stats
  - Parse TTL for suspended_until calculation
  - Return full ChannelHealth struct
- [ ] Implement `suspendChannel(channelID int)` with exponential backoff:
  - INCR suspension_count
  - Calculate cooldown: min(BASE * 2^(count-1), MAX)
  - SET suspension key with calculated TTL
  - Log suspension event with count and duration
- [ ] Implement `disableChannelPermanently(channelID int)`:
  - Call `model.UpdateChannelStatusById()`
  - Log disable event with "high-failure-rate periods" terminology
- [ ] Implement `ResetChannelHealth(channelID int)`:
  - DEL failures, suspended, suspension_count keys
  - Preserve historical statistics (total_failures, total_successes, timestamps)
  - Do NOT delete window bucket keys (they auto-expire)
  - Log manual reset event

**Dependencies**: `common/redis.go`, `model/channel.go`

**Testing**:
- Unit test: Single request recorded to bucket correctly
- Unit test: Window stats sum 6 buckets correctly
- Unit test: High failure rate detection (60 failures + 540 successes = 10% → NOT high)
- Unit test: High failure rate detection (298 failures + 2 successes = 99% → IS high)
- Unit test: Low traffic handling (5 failures + 1 success = 83% → IS high)
- Unit test: Minimum sample size (5 requests → NOT evaluated)
- Unit test: Consecutive high-failure-rate periods increment only when rate high
- Unit test: 3 high-failure-rate periods → suspension triggered
- Unit test: 10 high-failure-rate periods → permanent disable
- Unit test: Success resets counters and records to window
- Unit test: Exponential backoff calculation (1st: 5min, 2nd: 10min, etc.)
- Unit test: Redis unavailable → fail open

---

### Task 1.2: Integrate Health Check into Channel Selection
**File**: `model/channel_cache.go`

- [ ] Import `service` package
- [ ] Modify `GetRandomSatisfiedChannel()`:
  - Filter out suspended channels using `service.IsChannelAvailable()`
  - Return error if no available channels remain
  - Continue with weighted random selection on filtered list
- [ ] Add logging for filtered channel count

**Dependencies**: Task 1.1

**Testing**:
- Integration test: Suspended channel not selected
- Integration test: All channels suspended → error returned
- Integration test: Normal channels selected with correct weights

---

### Task 1.3: Integrate Health Recording into Error Handling
**File**: `controller/relay.go`

- [ ] Import `service` package
- [ ] In `relayRequest()` after `relayChannelRequest()`:
  - On success: call `service.RecordChannelSuccess(channelId)` (records to window + resets counters)
  - On error:
    - Check `shouldTriggerChannelFailover()`
    - If true: call `service.RecordChannelFailure(channelId)` (records to window + checks rate + conditionally counts)
- [ ] Ensure recording happens before retry logic

**Dependencies**: Task 1.1

**Testing**:
- Integration test: Successful request → success recorded to window
- Integration test: Failover error + high rate → failure counted as consecutive period
- Integration test: Failover error + low rate (single user) → NOT counted
- Integration test: Non-failover error → no recording

---

## Phase 2: Backend - API Layer

### Task 2.1: Create Health API Endpoints
**File**: `controller/channel.go`

- [ ] Implement `GetChannelHealth(c *gin.Context)`:
  - Parse channel ID from URL param
  - Call `service.GetChannelHealth()`
  - Return JSON response
- [ ] Implement `GetAllChannelsHealth(c *gin.Context)`:
  - Fetch all channels from database
  - Loop and call `service.GetChannelHealth()` for each
  - Return JSON array response
- [ ] Implement `ResetChannelHealth(c *gin.Context)`:
  - Parse channel ID from URL param
  - Call `service.ResetChannelHealth()`
  - Return success/error JSON response

**Dependencies**: Task 1.1

**Testing**:
- API test: GET /api/channel/123/health → 200 with health data (including current_failure_rate, window stats, suspension_count)
- API test: GET /api/channel/invalid/health → 400 bad request
- API test: GET /api/channels/health → 200 with array
- API test: POST /api/channel/123/health/reset → 200 success
- API test: POST /api/channel/123/health/reset (verify Redis keys deleted, buckets preserved)

---

### Task 2.2: Register API Routes
**File**: `router/api.go`

- [ ] Add route: `GET /api/channel/:id/health` → `GetChannelHealth`
- [ ] Add route: `GET /api/channels/health` → `GetAllChannelsHealth`
- [ ] Add route: `POST /api/channel/:id/health/reset` → `ResetChannelHealth`
- [ ] Apply authentication middleware (reset endpoint requires admin role)

**Dependencies**: Task 2.1

**Testing**:
- Manual test: curl endpoints verify routing
- Manual test: verify admin authorization required for reset endpoint

---

## Phase 3: Frontend - Components

### Task 3.1: Create Health Status Component
**File**: `web/src/components/table/channels/ChannelHealthStatus.jsx`

- [ ] Create functional component accepting `health` and `onClick` props
- [ ] Render status tag based on health state:
  - `is_suspended=true` → Orange "已暂停" tag
  - `consecutive_failures>0` → Yellow "警告" tag
  - Normal → Green "正常" tag
- [ ] Add tooltip with failure count
- [ ] Add click handler
- [ ] Style with cursor pointer

**Dependencies**: None

**Testing**:
- Visual test: Render with different health states
- Interaction test: Click triggers onClick callback

---

### Task 3.2: Create Health Detail Modal
**File**: `web/src/components/table/channels/ChannelHealthModal.jsx`

- [ ] Create functional component accepting `visible`, `health`, `channelId`, `onClose`, `onHealthReset` props
- [ ] Add state for `isResetting` loading state
- [ ] Use Semi-UI Modal component
- [ ] Use Descriptions to show:
  - Status (tag)
  - Consecutive failures (X / 10) - label as "连续高失败率周期"
  - Current window failure rate (percentage)
  - Window request count (total requests in last 60s)
  - Suspension count (if > 0)
  - Cooldown progress bar (if suspended) with dynamic duration display
  - Last success time (relative)
  - Last failure time (relative)
  - Total requests
  - Success count
  - Failure count
  - Success rate percentage
- [ ] Calculate cooldown progress from `suspended_until` and `suspension_count`
  - Use exponential backoff formula: min(5 * 2^(count-1), 60)
- [ ] Use `date-fns` for time formatting
- [ ] Add custom footer with:
  - Close button
  - "重置健康状态" button (danger theme, shown only when suspended or has failures)
- [ ] Implement `handleReset()` function:
  - Call API.post(`/api/channel/${channelId}/health/reset`)
  - Show success/error Toast
  - Call onHealthReset() callback
  - Close modal on success

**Dependencies**: None

**Testing**:
- Visual test: Render with suspended channel (showing suspension count, dynamic cooldown, failure rate)
- Visual test: Render with normal channel (no reset button, showing window stats)
- Visual test: Render with warning state (showing reset button, current failure rate)
- Visual test: Cooldown progress bar updates correctly
- Visual test: Window failure rate displays as percentage
- Interaction test: Reset button calls API and refreshes data

---

### Task 3.3: Add Health API Client
**File**: `web/src/helpers/api.js` or create new `web/src/api/channel.js`

- [ ] Create `fetchChannelHealth(channelId)` function:
  - GET /api/channel/:id/health
  - Return health data
- [ ] Create `fetchAllChannelsHealth()` function:
  - GET /api/channels/health
  - Return array of health data
- [ ] Create `resetChannelHealth(channelId)` function:
  - POST /api/channel/:id/health/reset
  - Return success/error response
- [ ] Add error handling for all functions

**Dependencies**: None

**Testing**:
- Unit test: Mock API calls and verify responses
- Unit test: Verify reset function sends POST request

---

### Task 3.4: Integrate Health Status into Channel List
**File**: `web/src/components/table/channels/ChannelsColumnDefs.jsx`

- [ ] Import ChannelHealthStatus and ChannelHealthModal components
- [ ] Add new column definition:
  - title: '健康状态'
  - dataIndex: 'health'
  - render: ChannelHealthStatus component
- [ ] Add state for modal visibility
- [ ] Add click handler to open modal

**Dependencies**: Task 3.1, Task 3.2

**Testing**:
- Visual test: Health column appears in table
- Interaction test: Click status opens modal

---

### Task 3.5: Fetch and Display Health Data
**File**: `web/src/pages/Channel/index.jsx` (or wherever channel list is rendered)

- [ ] Import `fetchAllChannelsHealth` from API client
- [ ] Fetch health data on component mount
- [ ] Merge health data with channel data by channel_id
- [ ] Add auto-refresh every 30 seconds for health data
- [ ] Handle loading and error states

**Dependencies**: Task 3.3, Task 3.4

**Testing**:
- Integration test: Health data loads and displays
- Integration test: Auto-refresh updates health data
- Integration test: Error handling shows graceful fallback

---

## Phase 4: Testing & Validation

### Task 4.1: Backend Unit Tests
**File**: `service/channel_health_test.go`

- [ ] Test `RecordChannelRequest()` records to correct bucket
- [ ] Test `GetWindowStats()` sums buckets correctly
- [ ] Test `IsHighFailureRate()` with various scenarios:
  - 60 failures + 540 successes = 10% → NOT high
  - 298 failures + 2 successes = 99% → IS high
  - 5 failures + 1 success = 83% low traffic → IS high
  - 5 failures + 5 successes = 50%, < 10 requests → NOT evaluated
  - 15 failures + 25 successes = 37.5%, low traffic → IS high (>50% threshold)
- [ ] Test `RecordChannelFailure()` only increments when rate high
- [ ] Test suspension triggered at threshold (3 high-failure-rate periods)
- [ ] Test exponential backoff calculation (1st: 5min, 2nd: 10min, 3rd: 20min, etc.)
- [ ] Test suspension_count increments on each suspension
- [ ] Test permanent disable at threshold (10 high-failure-rate periods)
- [ ] Test `RecordChannelSuccess()` resets all counters (including suspension_count)
- [ ] Test `RecordChannelSuccess()` records to window correctly
- [ ] Test `ResetChannelHealth()` deletes correct keys
- [ ] Test `GetChannelHealth()` returns window stats correctly
- [ ] Test `IsChannelAvailable()` with various states
- [ ] Test Redis unavailable scenarios (fail open)
- [ ] Test bucket auto-expiry (TTL 120s)

---

### Task 4.2: Backend Integration Tests
**File**: `controller/relay_test.go`, `model/channel_cache_test.go`

- [ ] Test end-to-end: failure → high rate → suspension → recovery
- [ ] Test end-to-end: single user problem (low rate) → NOT suspended
- [ ] Test channel selection filters suspended channels
- [ ] Test error recording in relay handler (with rate check)
- [ ] Test success recording in relay handler (with window recording)

---

### Task 4.3: Frontend Component Tests
**Files**: `*.test.jsx`

- [ ] Test ChannelHealthStatus renders correct tag (normal/warning/suspended)
- [ ] Test ChannelHealthModal displays correct data (including suspension_count, current_failure_rate, window_stats)
- [ ] Test modal cooldown progress calculation with exponential backoff
- [ ] Test modal displays window failure rate and request count
- [ ] Test "连续高失败率周期" label instead of "连续失败次数"
- [ ] Test manual reset button appears only when needed
- [ ] Test reset button calls API and refreshes data

---

### Task 4.4: Manual Testing Checklist

- [ ] Create test channel
- [ ] **Single-user problem test**:
  - Simulate 1 user with bug: send 60 failures/min
  - Simulate 9 normal users: send 540 successes/min
  - Verify channel NOT suspended (window rate = 10%)
  - Verify UI shows low failure rate
- [ ] **Real channel failure test**:
  - Send 298 failures + 2 successes (99% rate)
  - Verify 1st suspension within 30 seconds (5 minutes, suspension_count=1)
  - Verify UI shows high failure rate and suspension
- [ ] **Exponential backoff test**:
  - Wait for auto-recovery
  - Trigger 3 more high-failure-rate periods → verify 2nd suspension (10 minutes, suspension_count=2)
  - Repeat → verify 3rd suspension (20 minutes), 4th (40 minutes), 5th (60 minutes capped)
- [ ] **Manual reset test**:
  - Click reset button → verify suspension cleared
  - Verify suspension_count reset to 0
  - Verify window stats preserved
  - Verify UI updates immediately
- [ ] **Progressive cooldown reset test**:
  - After manual reset, trigger 3 high-failure-rate periods again
  - Verify 1st suspension (5 minutes, count resets to 1)
- [ ] **Permanent disable test**:
  - Trigger 10 consecutive high-failure-rate periods
  - Verify permanent disable
- [ ] **Low traffic test**:
  - Send 5 failures + 1 success (83% rate, low traffic)
  - Verify counted as high-failure-rate period
- [ ] Check UI updates in real-time (window stats, failure rate)
- [ ] Test with Redis unavailable → verify fail open
- [ ] Test with multiple instances → verify distributed state consistent

---

## Phase 5: Documentation & Deployment

### Task 5.1: Update Documentation
**File**: `docs/channel-health-tracking.md`

- [ ] Document feature overview with sliding window concept
- [ ] Document failure rate detection mechanism (30% threshold, 60s window)
- [ ] Document single-user problem solution
- [ ] Document thresholds and behavior (3 high-failure-rate periods → suspend, 10 → disable)
- [ ] Document exponential backoff strategy (5min → 10min → 20min → 40min → 60min max)
- [ ] Document manual recovery feature and UI usage
- [ ] Document Redis key schema (including bucket keys and suspension_count)
- [ ] Document sliding window parameters (window size, bucket size, thresholds)
- [ ] Document API endpoints (including /reset endpoint)
- [ ] Document UI usage (status indicators, health modal, reset button, failure rate display)

---

### Task 5.2: Add Configuration Options
**File**: `common/constants.go` or environment variables

- [ ] Make sliding window duration configurable (default: 60s)
- [ ] Make bucket size configurable (default: 10s)
- [ ] Make failure rate threshold configurable (default: 30%)
- [ ] Make low-traffic threshold configurable (default: 50%)
- [ ] Make minimum sample size configurable (default: 10)
- [ ] Make suspension threshold configurable (default: 3 periods)
- [ ] Make disable threshold configurable (default: 10 periods)
- [ ] Make base suspension duration configurable (default: 5min)
- [ ] Make max suspension duration configurable (default: 60min)
- [ ] Add feature flag to enable/disable health tracking

---

### Task 5.3: Database Migration (if needed)

- [ ] Check if any database schema changes needed
- [ ] Create migration script if necessary
- [ ] Test migration on dev environment

---

### Task 5.4: Deploy and Monitor

- [ ] Deploy to staging environment
- [ ] Run load tests with single-user failure scenarios
- [ ] Monitor Redis metrics (bucket storage, operation latency)
- [ ] Monitor suspension/disable rates
- [ ] Verify single-user failures do NOT trigger suspension
- [ ] Verify real channel failures trigger suspension within 30s
- [ ] Verify bucket storage overhead (<100 bytes per channel)
- [ ] Verify <5ms latency impact
- [ ] Deploy to production
- [ ] Monitor for 24 hours

---

## Task Summary

### Phase 1: Backend Service (2-3 days)
- 3 tasks: Service layer, channel selection integration, error handling integration

### Phase 2: Backend API (0.5 day)
- 2 tasks: API endpoints, route registration

### Phase 3: Frontend (1-2 days)
- 5 tasks: Status component, modal component, API client, column integration, data fetching

### Phase 4: Testing (1-2 days)
- 4 tasks: Unit tests, integration tests, component tests, manual testing

### Phase 5: Documentation & Deployment (0.5-1 day)
- 4 tasks: Documentation, configuration, migration, deployment

**Total Estimated Time**: 5-7 days

---

## Dependencies Graph

```
Phase 1.1 (Service Layer)
    ↓
Phase 1.2 (Channel Selection) + Phase 1.3 (Error Handling)
    ↓
Phase 2.1 (API Endpoints)
    ↓
Phase 2.2 (Routes)
    ↓
Phase 3.3 (API Client)
    ↓
Phase 3.1 (Status Component) + Phase 3.2 (Modal Component)
    ↓
Phase 3.4 (Column Integration)
    ↓
Phase 3.5 (Data Fetching)
    ↓
Phase 4.* (Testing)
    ↓
Phase 5.* (Documentation & Deployment)
```
