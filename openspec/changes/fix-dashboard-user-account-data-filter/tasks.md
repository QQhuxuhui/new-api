# Implementation Tasks: Fix Dashboard User Account Data Filter

## Backend Tasks

### Task 1: Add Model Function to Get User by Username
- [x] Open `model/user.go`
- [x] Add function `GetUserByUsername(username string, selectAll bool) (*User, error)`
- [x] Implement database query: `DB.Where("username = ?", username).First(&user)`
- [x] Handle case sensitivity appropriately (exact match)
- [x] Return error if user not found
- [ ] Test with existing users in the database

**Validation:** Function correctly retrieves user by username and returns appropriate errors

### Task 2: Add Controller Function for User Query API
- [x] Open `controller/user.go`
- [x] Add function `GetUserByUsername(c *gin.Context)` after the existing `GetUser` function
- [x] Extract `username` query parameter: `c.Query("username")`
- [x] Validate username is not empty
- [x] Call `model.GetUserByUsername(username, false)`
- [x] Implement authorization check: verify current user is admin
- [x] Return JSON response with user data on success
- [x] Return appropriate error responses (400 for invalid input, 403 for unauthorized, 404 for user not found)
- [x] Sanitize response: remove sensitive fields (password, admin remarks)

**Validation:** Endpoint returns correct user data for admin requests and rejects non-admin requests

### Task 3: Register API Route
- [x] Open `router/api-router.go`
- [x] Locate the `adminRoute` group (around line 97)
- [x] Add new route: `adminRoute.GET("/by-username", controller.GetUserByUsername)`
- [x] Verify route is protected by `middleware.AdminAuth()`

**Validation:** Route is accessible at `/api/user/by-username?username=<username>` with admin authentication

## Frontend Tasks

### Task 4: Add API Call Function for User Query
- [x] Open `web/src/helpers/index.js` or create helper in dashboard files
- [x] Add function to call `/api/user/by-username` endpoint
- [x] Handle response and error cases
- [x] Or integrate directly into `useDashboardData.js`

**Validation:** API call function successfully retrieves user data

### Task 5: Update Dashboard Data Hook - Add State
- [x] Open `web/src/hooks/dashboard/useDashboardData.js`
- [x] Add new state: `const [queriedUserData, setQueriedUserData] = useState(null)`
- [x] Export `queriedUserData` in the return statement

**Validation:** State is properly initialized and exported

### Task 6: Update Dashboard Data Hook - Add Fetch Logic
- [x] In `useDashboardData.js`, modify the `loadQuotaData` function
- [x] After the existing data fetch, check if `username` parameter exists and is not empty
- [x] If username exists:
  - Call API to get user data: `GET /api/user/by-username?username=${username}`
  - On success, update `setQueriedUserData(data)`
  - On error, show error message and set `setQueriedUserData(null)`
- [x] If username is empty:
  - Clear queried user data: `setQueriedUserData(null)`

**Validation:** Queried user data is fetched and stored when username filter is applied

### Task 7: Update Dashboard Data Hook - Add Reset Logic
- [x] In `useDashboardData.js`, ensure `queriedUserData` is cleared when:
  - Search modal is closed without username
  - Username filter is removed
  - Dashboard is refreshed without filter
- [x] Update `handleCloseModal` to clear `queriedUserData` if needed

**Validation:** Queried user data is properly reset when filter is cleared

### Task 8: Update Dashboard Stats Hook - Use Queried User Data
- [x] Open `web/src/hooks/dashboard/useDashboardStats.jsx`
- [x] Accept `queriedUserData` as a parameter
- [x] In the "账户数据" (Account Data) section of `groupedStatsData`:
  - For "当前余额" (Current Balance): use `queriedUserData?.quota ?? userState?.user?.quota`
  - For "历史消耗" (Historical Consumption): use `queriedUserData?.used_quota ?? userState?.user?.used_quota`
  - For "请求次数" (Request Count): use `queriedUserData?.request_count ?? userState?.user?.request_count`

**Validation:** Account Data section displays queried user's data when available, falls back to current user's data otherwise

### Task 9: Pass Queried User Data Through Component Chain
- [x] Open `web/src/components/dashboard/index.jsx`
- [x] Pass `queriedUserData` from `dashboardData` to `useDashboardStats`:
  ```javascript
  const { groupedStatsData } = useDashboardStats(
    userState,
    dashboardData.queriedUserData,  // Add this parameter
    dashboardData.consumeQuota,
    // ... other parameters
  );
  ```

**Validation:** Data flows correctly from hook to component

## Testing Tasks

### Task 10: Manual Testing - Admin User Query
- [ ] Log in as an administrator
- [ ] Open the dashboard
- [ ] Verify initial display shows admin's own account data
- [ ] Click the search icon (magnifying glass)
- [ ] Enter a known username in the search field
- [ ] Confirm the search
- [ ] Verify Account Data section updates to show the queried user's:
  - Current balance
  - Historical consumption
  - Request count
- [ ] Verify all other sections also update correctly

**Validation:** All dashboard sections display queried user's data correctly

### Task 11: Manual Testing - Clear Filter
- [ ] With user filter active (from Task 10)
- [ ] Clear the username filter or refresh without username
- [ ] Verify Account Data section reverts to admin's own data
- [ ] Verify all sections return to showing admin's data

**Validation:** Dashboard correctly reverts to current user's data when filter is cleared

### Task 12: Manual Testing - Non-Existent User
- [ ] Log in as an administrator
- [ ] Open the dashboard and click search
- [ ] Enter a username that doesn't exist
- [ ] Confirm the search
- [ ] Verify appropriate error message is displayed
- [ ] Verify dashboard data remains unchanged or shows empty state

**Validation:** Error handling works correctly for non-existent users

### Task 13: Manual Testing - Regular User Access
- [ ] Log in as a regular (non-admin) user
- [ ] Open the dashboard
- [ ] Verify no username search field is available (feature is admin-only)
- [ ] Verify only the logged-in user's data is displayed

**Validation:** Regular users cannot access user query functionality

### Task 14: Manual Testing - Authorization Check
- [ ] Attempt to call the API endpoint `/api/user/by-username?username=testuser` as a regular user
- [ ] Verify the request is rejected with 403 Forbidden
- [ ] Verify no user data is returned in the response

**Validation:** API endpoint properly enforces admin-only access

## Documentation Tasks

### Task 15: Update API Documentation (if applicable)
- [ ] If project has API documentation (e.g., in `docs/` folder), add entry for:
  - Endpoint: `GET /api/user/by-username`
  - Parameters: `username` (query string, required)
  - Authorization: Admin role required
  - Response format and fields
  - Example request/response
  - Error codes and meanings

**Validation:** API documentation is accurate and complete

## Deployment Checklist

- [ ] All code changes committed with descriptive commit message
- [ ] All tests passing (manual testing completed)
- [ ] No console errors or warnings in browser
- [ ] No backend errors in logs
- [ ] Code reviewed (if applicable)
- [ ] Changes deployed to staging/development environment
- [ ] Smoke tests performed in staging
- [ ] Ready for production deployment

---

**Total Tasks:** 15 tasks
**Estimated Effort:** 4-6 hours
**Priority:** Medium (Bug fix improving admin experience)
