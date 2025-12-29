## 1. Core Session Pool Refactor

- [x] **1.1** Modify `SessionEntry` struct to track creation time for rotation
- [x] **1.2** Add `generateRandomUUID()` helper using crypto/rand
- [x] **1.3** Modify `ChannelSessionPool` to store fixed-size session slice instead of map
- [x] **1.4** Implement `initializeSessions(n int)` to pre-generate N random UUIDs
- [x] **1.5** Modify `SelectRandomSession()` to pick from fixed slice (no TTL check needed)

## 2. Concurrency-Bound Pool Creation

- [x] **2.1** Update `GetPool()` signature: add `maxSessions int` parameter
- [x] **2.2** When creating new pool, use `maxSessions` (fallback to 5 if ≤0)
- [x] **2.3** When retrieving existing pool, check if `maxSessions` changed
- [x] **2.4** If `maxSessions` changed, rebuild pool with new size

## 3. Session Rotation Mechanism

- [x] **3.1** Add `rotationInterval` config (default 2 hours)
- [x] **3.2** Track each session's creation time in `SessionEntry`
- [x] **3.3** Implement `rotateOldestSession()` to replace 1 oldest session per period
- [x] **3.4** Modify cleanup loop to call rotation instead of TTL-based cleanup

## 4. API Integration

- [x] **4.1** Update `masqueradeMetadata()` in `metadata.go` to accept `maxSessions`
- [x] **4.2** Update `MasqueradeMetadataInBody()` signature
- [x] **4.3** Modify `claude_handler.go` to pass `channel.MaxConcurrentRequestsPerKey`
- [x] **4.4** Handle nil/0 `MaxConcurrentRequestsPerKey` with default value

## 5. Remove Legacy Collection Logic

- [x] **5.1** Remove `AddSession()` method (no longer collecting from downstream)
- [x] **5.2** Remove session extraction from `MasqueradeMetadata()`
- [x] **5.3** Remove TTL-based eviction logic (replaced by rotation)
- [x] **5.4** Clean up unused constants (`defaultMasqueradeMaxSessions = 50`)

## 6. Testing

- [x] **6.1** Update `session_pool_test.go`: test pool size matches input
- [x] **6.2** Test pre-generation: verify N unique UUIDs created
- [x] **6.3** Test rotation: oldest session replaced after interval
- [x] **6.4** Test concurrency change detection: pool rebuilds on size change
- [x] **6.5** Test default behavior: no concurrency config → 5 sessions

## 7. Validation

- [ ] **7.1** Deploy to dev environment
- [ ] **7.2** Verify logs show correct session count per channel
- [ ] **7.3** Confirm session stability within rotation period
- [ ] **7.4** Monitor upstream for detection issues
