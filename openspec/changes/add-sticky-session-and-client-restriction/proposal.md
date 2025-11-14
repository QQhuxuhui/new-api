# Add Sticky Session and Client Restriction

## Overview

This change introduces two anti-ban features inspired by Claude Relay Service (CRS) to improve session consistency and API security:

1. **Sticky Session**: Bind conversation sessions to specific channels to maintain consistency across multiple requests in the same conversation
2. **Client Restriction**: Restrict API tokens to specific User-Agent patterns to prevent unauthorized usage and abuse

These features address key limitations in the current random channel selection mechanism and lack of client-level access control.

## Problem Statement

### Current Limitations

**Channel Selection (middleware/distributor.go:100)**:
- Every request independently selects a random channel via `CacheGetRandomSatisfiedChannel()`
- Same conversation may use different channels across requests
- Can cause context loss or behavioral inconsistencies
- No session affinity mechanism

**Access Control (middleware/auth.go)**:
- Only supports IP-based restrictions (`Token.AllowIps`)
- No User-Agent validation
- Cannot restrict tokens to specific client applications
- Vulnerable to token leakage and abuse

### Motivation

Based on analysis of CRS防封策略 (CRS Anti-Ban Strategy), these features provide:

1. **Better User Experience**: Consistent model behavior within conversations
2. **Reduced Error Rates**: Fewer anomalies from channel switching
3. **Enhanced Security**: Client-level access control beyond IP restrictions
4. **Competitive Parity**: Align with CRS capabilities for Claude API usage

## Objectives

### Primary Goals

1. **Sticky Session**:
   - Bind sessions to channels using user/session identifiers
   - Support configurable TTL (time-to-live) per token
   - Gracefully handle channel failures (unbind and re-select)
   - Support both Redis and in-memory cache backends

2. **Client Restriction**:
   - Validate User-Agent against token-level allow patterns
   - Support wildcard matching (e.g., `Claude-Code-CLI/*`)
   - Support regex patterns (e.g., `regex:^VSCode/.*`)
   - Provide clear error messages for rejected clients

### Success Criteria

- ✅ Sessions maintain same channel for duration of TTL
- ✅ Channel failures trigger automatic session unbinding
- ✅ Client restrictions block unauthorized User-Agents
- ✅ Performance overhead < 5ms per request
- ✅ Backward compatible (features disabled by default)
- ✅ Works in both single-instance and distributed deployments

## Non-Goals

- **Not** implementing per-account proxy configuration (CRS-specific feature)
- **Not** adding OAuth integration for account management
- **Not** implementing custom rate limiting beyond existing mechanisms
- **Not** modifying the core channel selection algorithm weights

## Scope

### In Scope

**Backend Changes**:
- Add `StickySession`, `StickySessionTTL` fields to `model.Token`
- Add `ClientRestrictionEnabled`, `AllowedClients` fields to `model.Token`
- Create `service.SessionManager` for session binding logic
- Modify `middleware.Distribute()` to check sticky sessions
- Modify `middleware.TokenAuth()` to validate User-Agent
- Add context keys for session management
- Database migration for new Token fields

**Frontend Changes**:
- Add sticky session configuration to Token edit form
- Add client restriction configuration with pattern editor
- Add preset templates for common client patterns
- Display session and client info in token management UI

**Documentation**:
- User guide for sticky session configuration
- User guide for client restriction patterns
- Best practices and recommended TTL values
- Migration guide for existing tokens

### Out of Scope

- Performance monitoring/analytics dashboard (future enhancement)
- Advanced session analytics (future enhancement)
- Client fingerprinting beyond User-Agent (future enhancement)
- Custom session identifier extraction logic (use OpenAI `user` field)

## Dependencies

### Internal Dependencies

- Existing Redis infrastructure (`common.RedisEnabled`)
- Token model and caching system (`model.Token`, `model.cacheSetToken`)
- Channel selection service (`service.CacheGetRandomSatisfiedChannel`)
- Gin context management (`common.SetContextKey`, `common.GetContextKey`)

### External Dependencies

- **None** - Uses existing Redis and GORM libraries

## Risks and Mitigations

### Risk 1: Redis Unavailability

**Impact**: Sticky sessions unavailable if Redis fails

**Mitigation**:
- Implement in-memory cache fallback (`SessionManager.memorySessionCache`)
- Graceful degradation to random channel selection
- Log warnings when falling back to memory cache

### Risk 2: Session State Synchronization

**Impact**: Multi-instance deployments may have inconsistent session state with memory cache

**Mitigation**:
- Document Redis requirement for distributed deployments
- Detect multi-instance setup and warn if Redis disabled
- Consider sticky sessions best-effort in memory-only mode

### Risk 3: User-Agent Spoofing

**Impact**: Malicious users can bypass client restrictions by faking User-Agent

**Mitigation**:
- Document this as first-layer defense (industry standard limitation)
- Recommend combining with IP restrictions for sensitive tokens
- Provide clear security guidance in documentation

### Risk 4: Performance Overhead

**Impact**: Additional Redis operations may increase latency

**Mitigation**:
- Benchmark Redis GET/SET operations (expected < 5ms)
- Use async cache updates where possible
- Monitor performance in production rollout

## Alternatives Considered

### Alternative 1: Session Binding by Conversation ID

**Description**: Use explicit `conversation_id` parameter instead of OpenAI `user` field

**Pros**:
- More precise session tracking
- Better for multi-conversation scenarios

**Cons**:
- Requires client-side support
- Not all clients send conversation_id
- Adds API complexity

**Decision**: Use OpenAI `user` field with fallback to Token ID (simpler, broader compatibility)

### Alternative 2: Advanced Client Fingerprinting

**Description**: Combine User-Agent with TLS fingerprinting, headers analysis

**Pros**:
- Harder to spoof
- More robust security

**Cons**:
- Significantly more complex
- May break legitimate clients
- Privacy concerns

**Decision**: Start with User-Agent only (simpler, explicit opt-in)

### Alternative 3: Global Session Management

**Description**: Make sticky sessions a system-wide setting instead of per-token

**Pros**:
- Simpler configuration
- Consistent behavior across all tokens

**Cons**:
- Less flexible
- Cannot customize TTL per use-case
- May not suit all scenarios

**Decision**: Per-token configuration (more flexible, user choice)

## Timeline Estimate

Based on the technical analysis document:

- **Backend Implementation**: 5.5 days
  - Database migration: 0.5 days
  - SessionManager service: 1.5 days
  - Distributor logic changes: 1 day
  - Client validation: 1 day
  - Error handling: 0.5 day
  - Testing: 1 day

- **Frontend Implementation**: 2.5 days
  - Token edit forms: 1.5 days
  - Preset templates: 0.5 days
  - UI testing: 0.5 days

- **Documentation & QA**: 1 day

**Total**: ~9 working days (~2 weeks)

## Stakeholders

- **Users**: Developers using New API for AI model access
- **Administrators**: Platform operators managing tokens and channels
- **Development Team**: Backend and frontend engineers
- **Security Team**: Reviewing access control mechanisms

## Open Questions

1. **Session Identifier Priority**: Should we support custom session ID headers beyond OpenAI `user` field?
   - **Recommendation**: Start with `user` field only, extend later if needed

2. **Client Pattern Library**: Should we maintain a built-in library of common client patterns?
   - **Recommendation**: Provide preset templates in UI, allow user customization

3. **Session Cleanup**: Should we implement active session cleanup beyond TTL expiration?
   - **Recommendation**: Implement periodic cleanup task (every 5 minutes for memory cache)

4. **Audit Logging**: Should we log all client restriction rejections for security analysis?
   - **Recommendation**: Yes, log at INFO level with details (token ID, User-Agent, IP)

## Related Documentation

- `/docs/粘性会话和客户端限制实现方案.md` - Detailed technical implementation plan
- `/docs/CRS防封策略详解.md` - CRS anti-ban strategy analysis
- `openspec/project.md` - Project architecture and conventions
