# Add Plan Order Payment System

## Overview

Implement a complete online payment system for plan subscriptions, allowing users to purchase plans directly through integrated payment methods (Epay initially, with Stripe/Creem support in phase 2). This creates a closed-loop purchase flow separate from the existing TopUp recharge system.

## Motivation

Currently, users can only top up their account balance (TopUp) but cannot directly purchase plan subscriptions online. The plan purchase flow is incomplete:
- Users click "Buy Now" on a plan → redirected to recharge page → recharge balance increases
- But the plan is NOT automatically purchased or activated
- This creates a broken user experience and limits monetization

The new system will:
- Enable direct online plan purchases with real-time payment
- Provide order management for users and administrators
- Maintain separation between account recharge (TopUp) and plan purchases (PlanOrder)
- Support flexible pricing with discounts and promotions
- Ensure transaction safety with idempotency and atomic operations

## Business Rules Confirmed

### Core Rules
1. **Plan Delivery Timing**: Synchronous delivery (create UserPlan immediately in payment callback)
2. **Delivery Failure Handling**: Scheduled task retry (scan anomalous orders every minute)
3. **User Group Rate**: NOT applied (plan prices uniform for all users)
4. **Discount Configuration**: Per-plan configuration using `Price` (actual) and `OriginalPrice` (before discount)
5. **Order Expiration**: 30 minutes for unpaid orders
6. **Queue Full Handling**: Reject purchase (max 10 active plans per user)
7. **High Priority Plans**: Added to queue end (users can manually switch)

### Implementation Scope
8. **TopUp Relationship**: Coexist independently (recharge and plan purchase separate)
9. **Payment Methods**: Epay only for MVP (Stripe/Creem in phase 2)
10. **Refund Functionality**: Not implemented (clearly state "no refunds" + manual handling)
11. **Purchasable Plans**: `purchasable` field in Plans table (each plan independently configured)
12. **Admin Backend**: Full implementation (order list + search + manual completion)

## Scope

### In Scope
- **Database**: New `plan_orders` table with price snapshots and status management
- **Backend APIs**:
  - Order creation with price calculation
  - Epay payment integration (request + callback)
  - Plan delivery service (create UserPlan instances)
  - Order status management (pending/paid/delivered/expired/cancelled)
  - User order history
  - Admin order management (list/search/manual completion)
- **Frontend**:
  - Plan purchase flow (pricing page → order confirmation → payment)
  - User order list page
  - Admin order management interface
- **Background Tasks**:
  - Order expiration cleanup (30min timeout)
  - Delivery retry for failed orders

### Out of Scope (Phase 2)
- Stripe and Creem payment integration
- Refund functionality
- Coupon/promotion system
- Subscription auto-renewal
- Order statistics and analytics dashboard

## Success Criteria

1. Users can purchase plans online and payment is processed successfully
2. Plans are automatically delivered after payment (UserPlan instance created)
3. Orders expire after 30 minutes if unpaid
4. Queue validation prevents purchases beyond 10 active plans
5. Admin can view, search, and manually complete orders
6. Payment callbacks are idempotent (no duplicate charges)
7. Price snapshots preserve transaction details even if plan prices change

## Dependencies

### Existing Systems
- User plan system (`user_plans` table and queue management)
- Epay integration (existing TopUp payment code can be reused)
- Plans table (with `Price`, `OriginalPrice`, and new `purchasable` field)

### Database Changes
- Add `purchasable` column to `plans` table
- Create new `plan_orders` table

### No Breaking Changes
- Existing TopUp recharge system remains unchanged
- Existing plan management system remains compatible
- No changes to UserPlan activation logic (reuse existing)

## Risks and Mitigations

### Risk 1: Payment Callback Failures
**Mitigation**:
- Synchronous delivery in database transaction
- Scheduled task scans for paid-but-undelivered orders (retry mechanism)
- Order-level locking to prevent concurrent processing

### Risk 2: Race Conditions
**Mitigation**:
- Database row-level locks (`SELECT ... FOR UPDATE`)
- Order locks using `sync.Map`
- Idempotency checks on order status

### Risk 3: Price Changes During Purchase
**Mitigation**:
- Snapshot all pricing details in PlanOrder record
- Order creation locks in price at time of purchase
- Clear display of price on order confirmation page

### Risk 4: Queue Overflow
**Mitigation**:
- Frontend validation before order creation
- Backend validation during order creation
- Clear error messages guiding users

## Timeline Estimate

**MVP Implementation**: 7 days

### Breakdown
- Database schema + migrations: 0.5 day
- Backend APIs (order CRUD, payment, delivery): 2 days
- Frontend purchase flow: 1.5 days
- Admin order management: 1.5 days
- Background tasks (expiration, retry): 0.5 day
- Testing and bug fixes: 1 day

### Phase 2 (Future)
- Stripe/Creem integration: +2 days
- Refund functionality: +3 days
- Coupon system: +2 days

## Open Questions

None - all business rules confirmed with user.
