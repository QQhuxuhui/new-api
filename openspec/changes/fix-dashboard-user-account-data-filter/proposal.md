# Fix Dashboard User Account Data Filter

## Problem Statement

When administrators use the dashboard search feature to filter data by username, the statistical data (request counts, quota consumption, tokens) updates correctly to show the queried user's data. However, the "Account Data" section (current balance and historical consumption) incorrectly continues to display the logged-in administrator's data instead of the queried user's data.

## Current Behavior

1. Administrator opens the dashboard
2. Clicks the search icon (magnifying glass) in the top-right corner
3. Enters a username (e.g., "UserA") to query
4. The following data updates correctly:
   - Usage Statistics (request count, statistics count)
   - Resource Consumption (statistics quota, statistics tokens)
   - Performance Metrics (avg RPM, avg TPM)
   - Charts (pie chart, line chart, etc.)
5. **Bug**: Account Data section still shows the administrator's own balance and historical consumption, not UserA's data

## Expected Behavior

When an administrator searches for a specific user, ALL dashboard sections should display that user's data, including:
- Current balance
- Historical consumption
- All usage statistics
- All performance metrics
- All charts

## Root Cause

In `web/src/hooks/dashboard/useDashboardStats.jsx`, the Account Data section uses:
- `userState?.user?.quota` (current balance)
- `userState?.user?.used_quota` (historical consumption)

These values come from `getUserData()` which calls `/api/user/self`, returning the **logged-in user's** information, not the queried user's information.

## Proposed Solution

### Option 1: Frontend State Management (Chosen)
Add a separate state in the frontend to track the queried user's account information:

1. **Backend**: Add a new API endpoint `GET /api/user/by-username?username={username}` (admin-only)
   - Returns user information including `quota` and `used_quota`
   - Implements proper authorization checks

2. **Frontend**:
   - Add `queriedUserData` state in `useDashboardData.js`
   - When `username` filter is applied, fetch that user's data from the new API
   - Update `useDashboardStats.jsx` to use `queriedUserData` when available, fall back to `userState.user` for the current user
   - Clear `queriedUserData` when search is cleared

### Option 2: Enhanced Data API Response (Alternative)
Modify `/api/data/` endpoint to include user account information in the response:
- When `username` parameter is provided, include `user_quota` and `user_used_quota` in response
- Frontend extracts and uses this data for the Account Data section

**Decision**: We choose Option 1 because it:
- Maintains separation of concerns (user data vs. usage statistics)
- Follows existing API patterns
- Provides more flexibility for future enhancements
- Allows easier caching and state management

## Impact Analysis

### Files to Modify

**Backend:**
- `controller/user.go`: Add `GetUserByUsername` function
- `router/api-router.go`: Add route for new endpoint
- `model/user.go`: Add `GetUserByUsername` model function (if needed)

**Frontend:**
- `web/src/hooks/dashboard/useDashboardData.js`: Add `queriedUserData` state and fetch logic
- `web/src/hooks/dashboard/useDashboardStats.jsx`: Use `queriedUserData` when available
- `web/src/components/dashboard/StatsCards.jsx`: No changes needed (uses data from hook)

### User Impact
- **Administrators**: Will see accurate data for queried users
- **Regular Users**: No impact (they can only see their own data)
- **Performance**: Minimal impact (one additional API call only when admin searches by username)

### Backwards Compatibility
- Fully backwards compatible
- No breaking changes to existing APIs
- No database schema changes required

## Success Criteria

1. When admin searches for user "UserA", the Account Data section shows UserA's balance and consumption
2. When admin clears the search, the Account Data section reverts to showing admin's own data
3. Regular users continue to see only their own data
4. No performance degradation
5. Proper authorization: only admins can query other users' data

## Rollback Plan

If issues arise:
1. Revert frontend changes to use `userState.user` unconditionally
2. Remove new backend endpoint (if added)
3. System behavior reverts to pre-fix state (account data shows logged-in user)

## Related Issues

None identified. This is a standalone bug fix.
