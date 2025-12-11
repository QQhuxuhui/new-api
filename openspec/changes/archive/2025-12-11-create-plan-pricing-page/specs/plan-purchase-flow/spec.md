# Spec: Plan Purchase Flow

## Overview

This spec defines the user journey from viewing a plan on the pricing page to initiating the purchase process. The system must guide users through authentication (if required) and redirect them to the appropriate purchase interface.

---

## ADDED Requirements

### Requirement: Purchase button SHALL redirect to topup page with plan pre-selected

When a user clicks "Purchase" on a plan card, the system SHALL navigate them to the topup page with that plan already selected.

#### Scenario: Logged-in user clicks purchase button

**Given** the user is logged in
**And** the user is viewing the pricing page at `/plans`
**And** a Monthly plan with `id=2` is displayed
**When** the user clicks the "Get Started" button on the Monthly plan card
**Then** the system navigates to `/console/topup?plan_id=2`
**And** the topup page pre-selects the Monthly plan
**And** the plan details are displayed (name, price, quota)
**And** the user can proceed to payment

#### Scenario: Anonymous user clicks purchase button (public pricing page)

**Given** the user is not logged in
**And** the pricing page does not require authentication (`PricingPageRequireAuth=false`)
**And** the user is viewing the pricing page at `/plans`
**And** a Daily plan with `id=1` is displayed
**When** the user clicks the "Purchase" button on the Daily plan card
**Then** the system redirects to `/login?redirect=/console/topup?plan_id=1`
**And** after successful login, the user is redirected to `/console/topup?plan_id=1`
**And** the Daily plan is pre-selected on the topup page

#### Scenario: Anonymous user on auth-required pricing page

**Given** the pricing page requires authentication (`PricingPageRequireAuth=true`)
**And** the user is not logged in
**When** the user attempts to navigate to `/plans`
**Then** the system immediately redirects to `/login?redirect=/plans`
**And** the user never sees the pricing page until logged in

---

### Requirement: Purchase button SHALL show different CTAs based on plan type

The call-to-action text SHALL be contextually appropriate for the plan type.

#### Scenario: Free trial plan shows "Start Free Trial"

**Given** a Trial plan with `price=0` and `type=trial`
**When** the plan card is rendered on the pricing page
**Then** the CTA button text is "Start Free Trial"
**And** the button has primary styling (solid background)

#### Scenario: Paid subscription plan shows "Get Started"

**Given** a Monthly plan with `price=9.99` and `type=subscription`
**When** the plan card is rendered on the pricing page
**Then** the CTA button text is "Get Started"
**And** the button has primary styling

#### Scenario: Pay-as-you-go plan shows "Add to Account"

**Given** a PayG plan with `type=consumption` and `category=payg`
**When** the plan card is rendered on the pricing page
**Then** the CTA button text is "Add to Account"
**And** the button has primary styling

#### Scenario: Enterprise plan shows "Contact Sales"

**Given** an Enterprise plan with `type=enterprise`
**When** the plan card is rendered on the pricing page
**Then** the CTA button text is "Contact Sales"
**And** the button has secondary styling (outline or different color)
**And** clicking the button opens a contact form or redirects to support page

---

### Requirement: System SHALL validate plan availability before purchase

The system SHALL verify that a plan is actually available (enabled) before allowing purchase.

#### Scenario: User clicks purchase on enabled plan

**Given** a plan with `id=2` and `status=1` (enabled)
**And** the user is logged in
**When** the user clicks "Purchase" on the plan card
**Then** the system validates the plan is enabled
**And** navigation to `/console/topup?plan_id=2` proceeds successfully

#### Scenario: User attempts to purchase disabled plan (edge case)

**Given** a plan with `id=3` and `status=2` (disabled)
**And** somehow the plan appears on the pricing page (cache issue or race condition)
**When** the user clicks "Purchase" on the plan card
**Then** the system validates the plan status
**And** displays an error message "This plan is no longer available"
**And** the navigation does NOT proceed
**And** the page refreshes to remove the disabled plan

---

### Requirement: System SHALL show loading indicator during purchase initiation

The system SHALL provide visual feedback that the purchase flow is starting.

#### Scenario: User clicks purchase button

**Given** the user is on the pricing page
**And** the user clicks "Get Started" on a plan card
**When** the system validates the plan and initiates navigation
**Then** the purchase button shows a loading spinner
**And** the button text changes to "Loading..." or similar
**And** the button is disabled to prevent double-clicks
**And** once navigation starts, the loading state ends

#### Scenario: Purchase button returns to normal after navigation completes

**Given** the user clicked purchase and navigation started
**When** the topup page loads successfully
**Then** (this is handled by the topup page, not the pricing page)

---

### Requirement: System SHALL track purchase intent for analytics

The system SHALL log when users click purchase buttons for analytics and conversion tracking.

#### Scenario: User clicks purchase button (analytics event)

**Given** the user is on the pricing page
**And** a Monthly plan with `id=2` is displayed
**When** the user clicks "Get Started" on the Monthly plan card
**Then** the system logs an analytics event:
- Event name: `pricing_plan_purchase_clicked`
- Properties:
  - `plan_id`: 2
  - `plan_name`: "Monthly Plan"
  - `plan_price`: 9.99
  - `user_authenticated`: true/false
  - `timestamp`: ISO 8601 datetime

**And** the event is sent to the analytics service (if configured)
**And** the event does not block navigation (fire-and-forget)

#### Scenario: Track conversion funnel from pricing page to purchase

**Given** the analytics system is enabled
**When** a user views the pricing page
**Then** the system logs `pricing_page_viewed`
**When** the user clicks a purchase button
**Then** the system logs `pricing_plan_purchase_clicked`
**When** the user completes payment on the topup page
**Then** the topup page logs `plan_purchase_completed`
**And** the funnel can be analyzed: view → click → complete

---

### Requirement: System SHALL handle purchase button for owned plans

The system SHALL adjust behavior if the user already owns the plan they're viewing.

#### Scenario: User already owns the plan (show different CTA)

**Given** the user is logged in with `user_id=123`
**And** the user has an active Monthly plan with `plan_id=2`
**And** the Monthly plan is displayed on the pricing page
**When** the page renders
**Then** the Monthly plan card shows a badge "Active Plan" or "Current Plan"
**And** the purchase button text changes to "Manage Plan" or "View Details"
**And** clicking the button navigates to `/console/myplans` instead of `/console/topup`

#### Scenario: User has plan in queue (show different CTA)

**Given** the user is logged in with `user_id=123`
**And** the user has a Monthly plan with `plan_id=2` in queue (not active)
**And** the Monthly plan is displayed on the pricing page
**When** the page renders
**Then** the Monthly plan card shows a badge "In Queue (Position #3)"
**And** the purchase button text changes to "View Queue"
**And** clicking the button navigates to `/console/myplans`

**Note**: This requires fetching user's plans, which adds complexity. Consider making this Phase 2.

---

### Requirement: Purchase button SHALL be mobile-optimized

The purchase button SHALL be easy to tap on mobile devices.

#### Scenario: Purchase button on mobile

**Given** the user is on a mobile device (width < 768px)
**And** viewing the pricing page
**When** a plan card is rendered
**Then** the purchase button has a minimum height of 44px (iOS recommendation)
**And** the button spans the full width of the card
**And** there is adequate padding around the button (at least 8px)
**And** the button text is readable (font size ≥ 14px)

#### Scenario: Tap target size on mobile

**Given** the user is on a mobile device
**When** the user taps near the purchase button (within 44px × 44px area)
**Then** the button registers the tap
**And** the purchase flow initiates

---

## MODIFIED Requirements

None (this is a new feature).

---

## REMOVED Requirements

None.

---

## Relationships

- **Requires**: `plan-pricing-display` (displays the plans)
- **Integrates with**: Existing topup page (`/console/topup`)
- **Related to**: `add-user-plan-system` (user plan ownership check)

---

## Acceptance Criteria

- [ ] Purchase button navigates to `/console/topup?plan_id={id}`
- [ ] Anonymous users are redirected to login before purchase
- [ ] Button text changes based on plan type (Free Trial, Get Started, Contact Sales)
- [ ] Loading state shows during purchase initiation
- [ ] Plan validation prevents purchasing disabled plans
- [ ] Analytics events track purchase intent
- [ ] Mobile purchase button meets accessibility standards (44px min tap target)
- [ ] (Phase 2) Owned plans show "Manage" CTA instead of "Purchase"

---

## Open Questions

1. **Should we fetch user's existing plans on pricing page?**
   - Pro: Can show "You own this" badges
   - Con: Adds API call, slows page load
   - **Decision**: Phase 2 feature, not MVP

2. **Should Enterprise plans have a contact form modal?**
   - Pro: Keeps user on page
   - Con: Requires building a contact form
   - **Decision**: Use `mailto:` link or external contact page for MVP

3. **Should we prevent duplicate purchases?**
   - E.g., "You already have this plan in queue, purchase again?"
   - **Decision**: Let topup page handle this validation

---

## Implementation Notes

### Frontend (React)

```javascript
// In PlanCard.jsx
function handlePurchase(plan) {
  // Track analytics
  trackEvent('pricing_plan_purchase_clicked', {
    plan_id: plan.id,
    plan_name: plan.display_name,
    plan_price: plan.price
  });

  // Show loading
  setLoading(true);

  // Check if user is logged in
  if (!isAuthenticated) {
    navigate(`/login?redirect=/console/topup?plan_id=${plan.id}`);
    return;
  }

  // Navigate to topup with plan_id
  navigate(`/console/topup?plan_id=${plan.id}`);
}

function getPurchaseButtonText(plan) {
  if (plan.price === 0) return t('pricing.cta.startFreeTrial');
  if (plan.type === 'enterprise') return t('pricing.cta.contactSales');
  if (plan.category === 'payg') return t('pricing.cta.addToAccount');
  return t('pricing.cta.getStarted');
}
```

### Topup Page Integration

The existing topup page should be updated to:
- Read `plan_id` from URL query params
- Pre-select that plan in the UI
- Auto-populate plan details (name, price, quota)

---

**Status**: Spec Complete
**Complexity**: Medium
**Estimated Implementation**: 1 day
