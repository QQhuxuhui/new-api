# Change: Add User-Agent Passthrough and Channel Key Concurrency Limit

## Why

**Problem 1: Server Identification Risk**
Currently, the system uses Go's default HTTP client User-Agent (`Go-http-client/1.1`) when proxying requests to upstream AI providers. This exposes the platform as a relay service and creates risk of being blocked by upstream APIs that detect automated/proxy traffic patterns.

**Problem 2: No Concurrency Control per API Key**
Channels with multiple API keys have no way to limit concurrent requests per individual key. This can lead to:
- Rate limit violations on upstream APIs
- Uneven load distribution across keys
- Difficulty managing provider-specific concurrency restrictions

## What Changes

### 1. User-Agent Passthrough
- **Pass through client User-Agent** instead of using Go's default
- Implement in `relay/channel/api_request.go::SetupApiRequestHeader()`
- Maintain transparency of client identity to upstream providers
- Fallback to configurable default User-Agent if client doesn't provide one

### 2. Channel Key Concurrency Limit Configuration
- Add `max_concurrent_requests_per_key` field to Channel model
- Support configuration through channel edit UI
- Implement per-key request tracking and limiting
- Apply limits before forwarding requests to upstream APIs

## Impact

### Affected Specs
- `relay` (request header handling)
- `channel-management` (configuration and concurrency control)

### Affected Code
- `relay/channel/api_request.go` - Add User-Agent passthrough logic
- `model/channel.go` - Add concurrency limit field
- `dto/channel_settings.go` - Add concurrency settings DTO
- `controller/channel.go` - Handle concurrency configuration
- `service/channel_service.go` - Implement concurrency limiting logic
- `web/src/components/table/channels/modals/EditChannelModal.jsx` - Add concurrency limit UI field

### Migration Considerations
- Existing channels will have `max_concurrent_requests_per_key = 0` (unlimited) by default
- No breaking changes to existing behavior
- Database migration required to add new field
