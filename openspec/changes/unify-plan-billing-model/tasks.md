# Tasks: Unify Plan Billing Model

## Implementation Order

### Phase 1: Database Schema Changes

1. **Add fields to redemptions table**
   - Add `plan_id INT DEFAULT 0`
   - Add `validity_days INT DEFAULT 0`
   - GORM AutoMigrate handles this automatically
   - Estimated effort: Small

### Phase 2: Backend Logic Changes

2. **Modify PostConsumeQuota logic**
   - File: `service/quota.go`
   - Change from dual deduction to plan-priority with fallback
   - When userPlanId > 0 and plan has enough quota: only deduct plan quota
   - When userPlanId > 0 but insufficient quota: fallback to user balance
   - When no plan: deduct user balance (backward compatible)
   - Add `BillingSource` field to `relayInfo` to track billing source
   - Estimated effort: Medium

3. **Modify PreConsumeQuota logic**
   - File: `service/pre_consume_quota.go`
   - Add plan quota check before user balance check
   - Implementation steps:
     1. Query user's active plan via `model.GetActiveUserPlan(userId)`
     2. If plan exists and quota sufficient: set `relayInfo.BillingSource = "plan"`
     3. If plan insufficient or not exists: check user balance as fallback
     4. Set `relayInfo.BillingSource = "user_balance"` for fallback
     5. Integrate with trust quota mechanism (plan quota > trustQuota = trust mode)
   - Pre-consume deduction from selected source only (not both)
   - Estimated effort: Medium

4. **Add BillingSource to RelayInfo**
   - File: `relay/common/relay_info.go`
   - Add `BillingSource string` field ("plan" or "user_balance")
   - Used to coordinate between PreConsumeQuota and PostConsumeQuota
   - Estimated effort: Small

5. **Modify Redemption model and logic**
   - File: `model/redemption.go`
   - Add PlanId and ValidityDays fields to struct
   - Modify Redeem() function:
     - plan_id == 0: legacy mode, add to user quota
     - plan_id > 0 + user has plan: increase plan quota + reset expiry
     - plan_id > 0 + user doesn't have plan: create user_plan with quota
   - Add plan existence and status validation
   - Implement expiry reset rule (not accumulation)
   - Estimated effort: Medium

6. **Add trial plan binding for new users**
   - Files: `controller/user.go`, OAuth controllers
   - Create `bindTrialPlan(userId)` helper function
   - Call after successful registration/OAuth login
   - Handle "plan not found" gracefully (skip, don't block)
   - Estimated effort: Small

7. **Add trial plan initialization**
   - File: `model/main.go`
   - Create `initTrialPlan()` function called during migration
   - Check if trial plan exists, create if not
   - Use idempotent creation (safe for repeated runs)
   - Estimated effort: Small

### Phase 3: API Changes

8. **Update Redemption Create API**
   - File: `controller/redemption.go`
   - Accept `plan_id` and `validity_days` in request
   - Validate plan exists and is enabled
   - Estimated effort: Small

9. **Update Redeem Response**
   - File: `controller/user.go` or `controller/redemption.go`
   - Return plan_name and validity_days in response
   - Estimated effort: Small

### Phase 4: Frontend Changes

10. **Update Redemption management page**
    - File: `web/src/pages/Redemption/` or similar
    - Add plan selector dropdown
    - Add validity_days input field
    - Show plan info in redemption list
    - Estimated effort: Medium

11. **Update Redeem result display**
    - Show plan name and validity when redeeming plan-linked codes
    - Estimated effort: Small

### Phase 5: Logging & Audit

12. **Add billing source logging**
    - File: `service/quota.go`
    - Log billing source (plan vs user_balance) on each consumption
    - Log fallback events when plan quota insufficient
    - Estimated effort: Small

13. **Enhance consume log records**
    - File: `model/log.go` or relevant
    - Add `billing_source`, `user_plan_id` to ConsumeLogOther
    - Track plan quota before/after for auditing
    - Estimated effort: Small

14. **Add redemption audit logging**
    - File: `model/redemption.go`
    - Log redemption mode (plan vs legacy)
    - Log user_plan action (create vs extend)
    - Estimated effort: Small

### Phase 6: Testing & Validation

15. **Unit tests**
    - TestPreConsumeQuota_PlanFirst
    - TestPreConsumeQuota_FallbackToUserBalance
    - TestPostConsumeQuota_PlanPriority
    - TestPostConsumeQuota_FallbackToUserQuota
    - TestRedeem_WithPlanId
    - TestRedeem_Legacy
    - TestRedeem_ExpiryReset (not accumulation)
    - TestUserRegistration_TrialPlanBinding
    - Estimated effort: Medium

16. **Integration tests**
    - New user flow: register → use trial → exhaust → redeem consumption plan
    - Existing user compatibility: no-plan users continue using balance
    - Mixed redemption codes scenario
    - Concurrent redemption test
    - Estimated effort: Medium

### Phase 7: Data Setup

17. **Verify trial plan exists**
    - Check trial plan created by initTrialPlan()
    - Configuration: name="trial", quota=1000000, validity=7 days
    - Estimated effort: Small (verification only)

## Dependencies

```
Phase 1 (Schema)
    │
    ▼
Phase 2 (Backend Logic) ◄── Phase 7 (Data Setup) can run in parallel
    │
    ▼
Phase 3 (API)
    │
    ▼
Phase 4 (Frontend)
    │
    ├── Phase 5 (Logging) ◄── can start after Phase 2
    │
    ▼
Phase 6 (Testing) ◄── depends on all above
```

## Rollback Strategy

1. **Database changes**: Additive fields with defaults (safe, no rollback needed)
2. **Code changes**: Git revert to previous version
3. **Trial plan**: Can be disabled via status field
4. **User plans created**: Can be preserved or soft-deleted
5. **Logs**: Historical logs preserved for audit

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| PreConsumeQuota race condition | Use relayInfo to pass billing decision |
| Concurrent redemption | SELECT FOR UPDATE lock |
| Trial plan missing | Graceful skip with warning log |
| Plan quota goes negative | Allow negative, notify user, don't block request |
