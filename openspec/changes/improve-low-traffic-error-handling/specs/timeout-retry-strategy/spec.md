# timeout-retry-strategy Specification

## Purpose

Define retry behavior for Gateway Timeout errors (504/524) to enable limited channel failover while preventing cascading timeouts. This allows temporary network issues to be recovered through backup channels without introducing system instability.

## ADDED Requirements

### Requirement: 504/524 timeout errors MUST allow single channel-switching retry

When an upstream channel returns a Gateway Timeout error (HTTP 504 or 524), the system MUST attempt to retry the request on one alternate channel before returning the error to the client.

#### Scenario: Timeout triggers single retry to backup channel

**Given**:
- Channel A configured with upstream `http://slow-provider.com`
- Channel B configured with upstream `http://fast-provider.com`
- Client sends request with `RetryTimes=2` (default configuration)

**When**:
1. Request routed to Channel A (priority 100, `retry=0`)
2. Channel A returns HTTP 504 Gateway Timeout after 60 seconds
3. `shouldRetry()` evaluates timeout error with `retryTimes=1`

**Then**:
- `shouldRetry()` returns `true` (allows retry since `retryTimes > 0`)
- System selects Channel B via `getChannel(retry=1)`
- Request retries on Channel B
- If Channel B succeeds: Client receives successful response
- If Channel B also times out: Error returned to client (no further retries)

**Observable Behavior**:
```
[INFO] Request channelId=4, model=gpt-4
[ERR] Channel 4 error: 504 Gateway Timeout
[SYS] Timeout detected, attempting retry on backup channel
[INFO] Retry attempt 1: using channel #5
[INFO] 200 OK from channel #5
```

#### Scenario: Timeout retry exhausted after first attempt

**Given**:
- Two channels configured, both with timeout-prone upstreams
- Client request in progress

**When**:
1. Channel A returns 504 (first failure)
2. Retry on Channel B also returns 504 (second failure)
3. `shouldRetry()` called with `retryTimes=0`

**Then**:
- `shouldRetry()` returns `false` (retry exhausted)
- Error returned to client with message "bad response status code 504"
- No third retry attempt occurs

**Observable Behavior**:
```
[ERR] Channel 4 error: 504 Gateway Timeout
[INFO] Retry attempt 1: using channel #5
[ERR] Channel 5 error: 504 Gateway Timeout
[ERR] relay error: bad response status code 504
[GIN] 504 | POST /v1/chat/completions
```

#### Scenario: Non-timeout 5xx errors retain unlimited retry behavior

**Given**:
- Three channels configured for model `gpt-4`
- Client sends request with `RetryTimes=2`

**When**:
1. Channel A returns HTTP 500 Internal Server Error
2. `shouldRetry()` evaluates with `statusCode=500`

**Then**:
- `shouldRetry()` returns `true` (non-timeout 5xx always retry)
- System continues retrying across available channels
- Behavior unchanged from existing implementation

**Observable Behavior**:
```
[ERR] Channel 1 error: 500 Internal Server Error
[INFO] using channel #2 to retry
[ERR] Channel 2 error: 500 Internal Server Error
[INFO] using channel #3 to retry
[INFO] 200 OK from channel #3
```

### Requirement: Timeout retry MUST switch to different channel

When retrying a timeout error, the system MUST NOT reuse the same channel that timed out. The retry MUST be routed to a different available channel.

#### Scenario: Retry selects different channel via priority system

**Given**:
- Channel A (id=1, priority=100) experiences timeout
- Channel B (id=2, priority=50) available
- Channel C (id=3, priority=10) available

**When**:
1. Initial request routed to Channel A (highest priority, `retry=0`)
2. Channel A returns 504
3. `getChannel()` called with `retry=1`

**Then**:
- `CacheGetRandomSatisfiedChannel()` evaluates `retry=1` priority tier
- Channel B selected (next priority level)
- Context updated with Channel B details
- Request retries through Channel B
- Channel A remains in context history for logging

**Observable Behavior**:
```
[INFO] Selected channel #1 (priority 100) for initial attempt
[ERR] Channel #1 timeout after 60s
[INFO] Selecting backup channel for retry (priority tier: 50)
[INFO] Selected channel #2 (priority 50) for retry
```

#### Scenario: No available backup channels

**Given**:
- Only one channel configured for requested model
- Channel experiences timeout

**When**:
1. Channel A returns 504
2. `getChannel(retry=1)` attempts to find backup
3. No alternate channels available

**Then**:
- `getChannel()` returns `nil`
- Error wrapper generated: "分组 X 下模型 Y 的优先级 1 无健康渠道"
- Original 504 error returned to client
- No retry attempted

**Observable Behavior**:
```
[ERR] Channel #1 error: 504 Gateway Timeout
[WARN] No backup channels available for retry
[ERR] relay error: bad response status code 504
```

### Requirement: Timeout errors MUST be recorded to sliding window statistics

Regardless of retry outcome, all timeout errors MUST be recorded to the channel's sliding window for health tracking purposes.

#### Scenario: Both initial timeout and retry timeout are recorded

**Given**:
- Channel A and Channel B configured
- Both channels experiencing intermittent timeouts

**When**:
1. Request to Channel A returns 504 at T+0:00
   - `RecordChannelRequest(channelA, false)` called
2. Retry to Channel B returns 504 at T+1:00
   - `RecordChannelRequest(channelB, false)` called

**Then**:
- Redis key `channel:health:1:bucket:{N}:failures` incremented by 1
- Redis key `channel:health:2:bucket:{N}:failures` incremented by 1
- Both channels accumulate failure statistics independently
- If failure rates exceed thresholds, channels may be suspended

**Observable Behavior (Redis)**:
```redis
> GET channel:health:1:bucket:123456:total
"3"
> GET channel:health:1:bucket:123456:failures
"2"
> GET channel:health:2:bucket:123456:failures
"1"
```

#### Scenario: Successful retry records success to backup channel

**Given**:
- Channel A times out, Channel B succeeds on retry

**When**:
1. Channel A returns 504
   - `RecordChannelFailure(1, 504, "timeout")` called
2. Channel B returns 200 OK
   - `RecordChannelSuccess(2)` called

**Then**:
- Channel A failure count increments
- Channel B success count increments
- Channel B consecutive failures reset to 0
- Channels maintain independent health status

**Observable Behavior (logs)**:
```
[SYS] Channel 1 failure NOT counted: 样本数不足: 1 < 5 (rate=0.00%)
[INFO] record error log: channelId=1, content=bad response status code 504
[INFO] Channel 2 request successful, resetting health counters
```

## MODIFIED Requirements

None. This specification adds new behavior without modifying existing retry logic for non-timeout errors.

## REMOVED Requirements

None. Previous behavior is preserved for all non-504/524 status codes.

## Related Capabilities

- **adaptive-failure-detection**: Provides low-traffic detection that complements timeout retry by quickly identifying persistently failing channels
- **concurrency-limit-retry-behavior**: Establishes precedent for error-specific retry bypassing `RetryTimes` limit (applied to channel errors)
- Existing `channel-health` tracking: Timeout statistics feed into sliding window for suspension decisions
