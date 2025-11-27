# Change: Add User Plan System

## Why

Current system uses a single `Group` field on users and channels for access control, but lacks:
1. **Independent quota management** - Users need separate quotas per subscription type (monthly vs pay-as-you-go)
2. **Plan-based routing** - Different plans should route to different channel pools
3. **Smart switching** - Automatic upgrade to higher-priority plans when available
4. **Admin control** - Administrators need full control over user plan assignments and permissions

Business scenarios driving this change:
- Monthly subscribers should use dedicated high-quality channels
- Pay-as-you-go users should use cost-optimized channels
- Users may have multiple active plans simultaneously
- Administrators must prevent unauthorized plan switching

## What Changes

### New Capabilities
- **Plan Management**: Admin CRUD for plan templates (monthly, payg, trial, enterprise, etc.)
- **User Plan Binding**: Assign plans to users with individual quota and permissions
- **Plan Switching**: Smart switching logic with admin-controlled user permissions
- **Quota Consumption**: Per-plan quota tracking instead of per-user

### Data Model
- NEW TABLE `plans`: Plan templates with priority, channel_group, default settings
- NEW TABLE `user_plans`: User-plan assignments with quota, permissions, status
- MODIFIED: Request routing uses plan's channel_group instead of user's group
- MODIFIED: Quota deducted from user_plan instead of user

### **BREAKING** Changes
- Quota consumption moves from `users.quota` to `user_plans.quota`
- Channel selection uses plan's channel_group, not user's group
- Migration required for existing users

## Impact

- **Affected specs**: None (new capability)
- **Affected code**:
  - `model/`: New plan.go, user_plan.go models
  - `service/`: New plan_selector.go, modified quota.go
  - `middleware/distributor.go`: Plan-based routing
  - `controller/`: New plan.go, user_plan.go controllers
  - `router/`: New admin and user plan routes
  - `web/src/pages/`: New Plan management pages
- **Database**: New tables, migration for existing users
- **API**: New endpoints for plan management

## Dependencies

- Existing channel group mechanism (Channel.Group field)
- Existing quota system (model/quota.go, service/quota.go)
- Existing distributor middleware (middleware/distributor.go)
