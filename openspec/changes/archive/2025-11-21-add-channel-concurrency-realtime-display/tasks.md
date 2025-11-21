# Implementation Tasks

## 1. Backend - Concurrency Query Service
- [x] 1.1 Add `GetChannelConcurrencyInfo(channelId int, apiKey string)` function to `service/concurrency.go`
  - Returns struct with `current`, `limit`, `usage_percent` fields
  - Handles Redis unavailability with -1 return value
- [x] 1.2 Add `GetBatchChannelsConcurrency(channelIds []int)` function for efficient batch queries
  - Uses Redis pipelining or MGET for performance
  - Returns map[channelId]ConcurrencyInfo
- [x] 1.3 Add application-level caching (5 second TTL) for concurrency queries
  - Implement in-memory cache with sync.Map or similar
  - Add cache expiration logic

## 2. Backend - Channel API Extension
- [x] 2.1 Modify `GetAllChannels` in `controller/channel.go` to include concurrency info
  - Call `GetBatchChannelsConcurrency` for channels with limits configured
  - Add `concurrency_info` field to response JSON
- [x] 2.2 Add `GetChannelConcurrency` API endpoint (`GET /api/channel/:id/concurrency`)
  - Returns detailed per-key concurrency for multi-key channels
  - Handles single-key and multi-key channels differently
- [x] 2.3 Update `GetChannelById` to optionally include concurrency info
  - Add query parameter `include_concurrency=true`
  - Populate concurrency data for edit modal

## 3. Backend - Data Structure
- [x] 3.1 Define `ConcurrencyInfo` struct in `dto/channel.go` or `types/`
  - Fields: `Current int`, `Limit int`, `UsagePercent float64`, `LastUpdated int64`
- [x] 3.2 Define `MultiKeyConcurrencyInfo` struct for multi-key channels
  - Fields: `Keys []KeyConcurrency`, `TotalCurrent int`, `TotalCapacity int`
  - `KeyConcurrency` includes: `KeyIndex int`, `Current int`, `Limit int`, `Status string`
- [x] 3.3 Update Channel JSON tags to include `concurrency_info omitempty`

## 4. Frontend - Channel Table Display
- [x] 4.1 Add concurrency column to channel table in `web/src/components/table/channels/`
  - Display format: "current/limit" with color coding
  - Handle null/undefined concurrency info gracefully
- [x] 4.2 Implement color-coded status badges
  - Green (0-50%), Yellow (50-80%), Red (80-100%), Gray (unlimited/disabled)
  - Use Semi Design `Tag` or `Badge` components
- [x] 4.3 Add tooltip for multi-key channels
  - Show per-key breakdown on hover
  - Use Semi Design `Tooltip` component

## 5. Frontend - Auto-refresh Mechanism
- [x] 5.1 Implement auto-refresh hook in `web/src/hooks/channels/useChannelsData.jsx`
  - Default refresh interval: 10 seconds
  - Make interval configurable via settings
- [x] 5.2 Add manual refresh button to channel table header
  - Icon button with refresh indicator
  - Disable during refresh to prevent double-clicks
- [x] 5.3 Optimize refresh to update only concurrency data
  - Avoid re-fetching entire channel list
  - Use partial state update to prevent flicker

## 6. Frontend - Edit Modal Enhancement
- [x] 6.1 Add "Concurrency Status" section to `EditChannelModal.jsx`
  - Position below concurrency limit configuration field
  - Show only when `max_concurrent_requests_per_key > 0`
- [x] 6.2 Display current concurrency metrics with progress bar
  - Use Semi Design `Progress` component
  - Show last updated timestamp
- [x] 6.3 Implement per-key breakdown for multi-key channels
  - Table or card layout for each key
  - Visual indicators for disabled keys
- [x] 6.4 Add real-time refresh for modal concurrency display
  - Poll concurrency API while modal is open (5-second interval)
  - Stop polling when modal closes

## 7. Frontend - Warning Indicators
- [x] 7.1 Add high usage warning indicator (>80%)
  - Warning icon or badge in table row
  - Tooltip with usage details
- [x] 7.2 Add critical status indicator (all keys at limit)
  - Red alert badge with actionable suggestions
  - Display in both table and modal
- [x] 7.3 Add staleness indicator for outdated data
  - Show timestamp and "Refresh" prompt
  - Display when data is >30 seconds old

## 8. API Integration
- [x] 8.1 Update API client in `web/src/helpers/api.js` or similar
  - Add `fetchChannelConcurrency(channelId)` function (backend API already provides this)
  - Add `fetchBatchConcurrency(channelIds)` function (included in channel list response)
- [x] 8.2 Update channel data hooks to fetch concurrency
  - Modify `useChannelsData` to include concurrency in initial fetch
  - Add separate `useChannelConcurrency(channelId)` hook for modal

## 9. Internationalization
- [x] 9.1 Add i18n keys for concurrency display
  - `channel.concurrency.current`, `channel.concurrency.limit`, etc.
  - Add translations for zh, en, fr, ja, ru locales (zh completed)
- [x] 9.2 Add i18n keys for status messages
  - "High usage", "All keys at limit", "Data unavailable", etc.

## 10. Testing and Validation
- [ ] 10.1 Test concurrency display with single-key channels
  - Verify metrics update in real-time
  - Test color coding thresholds
- [ ] 10.2 Test concurrency display with multi-key channels
  - Verify per-key breakdown
  - Test aggregate metrics calculation
- [ ] 10.3 Test Redis unavailability handling
  - Verify graceful degradation (show "N/A")
  - Ensure UI doesn't crash
- [ ] 10.4 Test auto-refresh functionality
  - Verify refresh interval
  - Test manual refresh button
  - Ensure no memory leaks on long sessions
- [ ] 10.5 Test performance with many channels
  - Verify batch query efficiency
  - Test with 50+ channels in list
  - Ensure <200ms response time

## 11. Documentation
- [ ] 11.1 Update user documentation in `docs/使用文档/` (if needed)
  - Explain concurrency monitoring feature
  - Document color coding and indicators
- [ ] 11.2 Add inline help/tooltips in UI
  - Explain what concurrency metrics mean
  - Provide guidance on limit tuning
