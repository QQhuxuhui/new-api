# Implementation Tasks

## 1. User-Agent Passthrough

### 1.1 Backend Implementation
- [x] 1.1.1 Modify `relay/channel/api_request.go::SetupApiRequestHeader()` to pass through client User-Agent
- [x] 1.1.2 Add fallback logic for empty User-Agent (use configurable default)
- [x] 1.1.3 Add configuration option for default User-Agent in environment variables
- [x] 1.1.4 Update related adaptor implementations if needed

### 1.2 Testing
- [ ] 1.2.1 Test User-Agent passthrough with various client requests
- [ ] 1.2.2 Verify fallback behavior when client doesn't provide User-Agent
- [ ] 1.2.3 Test with different relay modes (audio, realtime, standard)

## 2. Channel Key Concurrency Limit

### 2.1 Database Schema
- [x] 2.1.1 Add `max_concurrent_requests_per_key` field to `channel` table (integer, default 0)
- [x] 2.1.2 Create database migration script (GORM AutoMigrate handles this)
- [x] 2.1.3 Update `model/channel.go` struct

### 2.2 Backend Logic
- [x] 2.2.1 Add concurrency tracking mechanism (Redis-based counter per key)
- [x] 2.2.2 Implement concurrency check in channel selection logic
- [x] 2.2.3 Add middleware to increment/decrement per-key counters
- [x] 2.2.4 Update `dto/channel_settings.go` if needed (Not needed - field added directly to Channel model)
- [x] 2.2.5 Add controller logic to handle concurrency configuration (Handled automatically via GORM)

### 2.3 Frontend UI
- [x] 2.3.1 Add `max_concurrent_requests_per_key` field to EditChannelModal
- [x] 2.3.2 Add form validation (non-negative integer)
- [ ] 2.3.3 Add i18n translations for the new field
- [x] 2.3.4 Add help text explaining the feature

### 2.4 Testing
- [ ] 2.4.1 Test concurrency limiting with single key
- [ ] 2.4.2 Test with multiple keys in same channel
- [ ] 2.4.3 Test Redis counter cleanup on request completion/error
- [ ] 2.4.4 Test configuration persistence and retrieval
- [ ] 2.4.5 Test UI validation and submission

## 3. Documentation
- [ ] 3.1 Update API documentation
- [ ] 3.2 Add configuration guide for User-Agent settings
- [ ] 3.3 Add guide for channel concurrency limit configuration
