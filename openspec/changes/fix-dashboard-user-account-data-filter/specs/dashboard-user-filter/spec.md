# Dashboard User Filter - Account Data Display

## ADDED Requirements

### Requirement: Admin Dashboard User Query MUST Return Complete User Information

When an administrator queries a specific user via the dashboard search feature, the system MUST return and display that user's complete account information, including balance and historical consumption data.

#### Scenario: Administrator Searches for Specific User's Account Data

**Given:**
- User is logged in as an administrator
- Dashboard is loaded and displaying the administrator's own data

**When:**
- Administrator clicks the search icon in the dashboard header
- Administrator enters username "UserA" in the search modal
- Administrator confirms the search

**Then:**
- The "Account Data" section displays UserA's current balance
- The "Account Data" section displays UserA's historical consumption
- The "Usage Statistics" section displays UserA's request count and statistics count
- The "Resource Consumption" section displays UserA's quota and token consumption
- The "Performance Metrics" section displays UserA's RPM and TPM
- All charts update to show UserA's data

#### Scenario: Administrator Clears User Filter

**Given:**
- Administrator has searched for user "UserA"
- Dashboard is displaying UserA's data in all sections

**When:**
- Administrator clears the username filter or closes the search modal without a username
- Administrator refreshes the dashboard

**Then:**
- The "Account Data" section reverts to displaying the administrator's own balance
- The "Account Data" section reverts to displaying the administrator's own historical consumption
- All other sections revert to displaying the administrator's own data

#### Scenario: Regular User Cannot Query Other Users' Data

**Given:**
- User is logged in as a regular user (non-administrator)
- Dashboard is loaded

**When:**
- Regular user attempts to access the dashboard

**Then:**
- The search feature does not display a username input field (admin-only feature)
- The dashboard displays only the logged-in user's data
- The user cannot query or view other users' account information

### Requirement: Backend API MUST Provide User Query by Username

The system MUST provide an admin-only API endpoint to retrieve user information by username, including account balance and consumption data.

#### Scenario: Admin Queries User by Username via API

**Given:**
- Request is made by an authenticated administrator
- User "UserA" exists in the system

**When:**
- Administrator calls `GET /api/user/by-username?username=UserA`

**Then:**
- API returns 200 OK status
- Response includes user data:
  - `id`: User's unique identifier
  - `username`: "UserA"
  - `quota`: Current balance
  - `used_quota`: Historical consumption
  - `request_count`: Total request count
  - `role`: User role
  - Other relevant user fields
- Response excludes sensitive data (password hash, API keys, etc.)

#### Scenario: Non-Admin Attempts to Query User by Username

**Given:**
- Request is made by a regular user (non-administrator)
- User "UserA" exists in the system

**When:**
- Regular user calls `GET /api/user/by-username?username=UserA`

**Then:**
- API returns 403 Forbidden status
- Response includes error message: "Insufficient permissions"
- No user data is returned

#### Scenario: Admin Queries Non-Existent User

**Given:**
- Request is made by an authenticated administrator
- User "NonExistentUser" does not exist in the system

**When:**
- Administrator calls `GET /api/user/by-username?username=NonExistentUser`

**Then:**
- API returns 404 Not Found status
- Response includes error message: "User not found"

### Requirement: Frontend MUST Maintain Separate State for Queried User Data

The dashboard frontend MUST maintain separate state for the currently queried user's data versus the logged-in administrator's data.

#### Scenario: Dashboard Displays Queried User's Account Data

**Given:**
- Administrator is logged in
- Administrator has searched for user "UserA"
- API has returned UserA's data successfully

**When:**
- Dashboard renders the Account Data section

**Then:**
- `queriedUserData` state contains UserA's account information
- Account Data cards display values from `queriedUserData.quota` and `queriedUserData.used_quota`
- Visual indicator shows that data is filtered by username
- Administrator's own data remains available in `userState.user` but is not displayed in filtered sections

#### Scenario: Dashboard Falls Back to Current User Data When No Filter Applied

**Given:**
- Administrator is logged in
- No username filter is currently applied
- `queriedUserData` state is null or empty

**When:**
- Dashboard renders the Account Data section

**Then:**
- Account Data cards display values from `userState.user.quota` and `userState.user.used_quota`
- No visual indicator for filtering is shown
- Data represents the administrator's own account information

## MODIFIED Requirements

None. This change adds new functionality without modifying existing behavior.

## REMOVED Requirements

None. All existing dashboard functionality is preserved.
