# Spec: Plan Pricing Display

## Overview

This spec defines how subscription plans are displayed to users on a dedicated pricing page. The system must present plan information in a clear, conversion-optimized format that helps users make informed purchase decisions.

---

## ADDED Requirements

### Requirement: System SHALL display enabled subscription plans on dedicated page

The system SHALL provide a public-facing pricing page that displays all enabled subscription plans in an attractive, easy-to-understand format.

#### Scenario: User views pricing page with multiple plans

**Given** the system has 3 enabled plans (Daily $0.99, Monthly $9.99, Annual $99.99)
**And** the pricing page is accessible at `/plans`
**When** a user navigates to `/plans`
**Then** the page displays all 3 plans in a card grid layout
**And** each plan card shows: display name, price, quota, category badge, and feature list
**And** plans are sorted by priority (highest first)
**And** the page is responsive (1 column on mobile, 3 columns on desktop)

#### Scenario: User views pricing page with no plans

**Given** the system has no enabled plans
**When** a user navigates to `/plans`
**Then** the page displays an empty state message "No plans available at this time"
**And** the message includes a link to contact support

#### Scenario: Pricing page requires authentication

**Given** the system setting `PricingPageRequireAuth` is set to `true`
**And** the user is not logged in
**When** the user navigates to `/plans`
**Then** the system redirects to `/login?redirect=/plans`
**And** after successful login, the user is redirected back to `/plans`

#### Scenario: Pricing page is public (no auth required)

**Given** the system setting `PricingPageRequireAuth` is set to `false`
**When** an anonymous user navigates to `/plans`
**Then** the page displays all enabled plans without requiring login

---

### Requirement: System SHALL display plan pricing with discount information

The system SHALL show both current and original prices when a discount is active, and calculate the discount percentage.

#### Scenario: Plan has discount (original_price > price)

**Given** a plan with `price=$9.99` and `original_price=$19.99`
**When** the plan is displayed on the pricing page
**Then** the card shows the current price "$9.99" prominently
**And** the original price "$19.99" is shown with strikethrough styling
**And** a discount badge "50% OFF" is displayed near the price
**And** the discount badge is visually distinct (e.g., orange/red color)

#### Scenario: Plan has no discount (original_price == price or not set)

**Given** a plan with `price=$9.99` and `original_price=$9.99`
**When** the plan is displayed on the pricing page
**Then** only the current price "$9.99" is shown
**And** no discount badge is displayed
**And** no strikethrough price is shown

#### Scenario: Plan is free (price = 0)

**Given** a trial plan with `price=0`
**When** the plan is displayed on the pricing page
**Then** the price displays as "Free"
**And** the CTA button text changes to "Start Free Trial" or "Get Started"

---

### Requirement: System SHALL extract and display plan features automatically

The system SHALL derive a feature list from plan data fields to help users understand what's included.

#### Scenario: Extract features for subscription plan

**Given** a Monthly plan with:
- `default_quota=1000000`
- `validity_days=30`
- `daily_quota_limit=50000`
- `category=monthly`
- `channel_groups=["gpt4", "claude"]`

**When** the feature list is generated
**Then** the list includes:
- "1M tokens quota"
- "Valid for 30 days"
- "Daily limit: 50K tokens"
- "Access to 2 model groups"
- "Occupies 1 queue slot"

#### Scenario: Extract features for daily plan

**Given** a Daily plan with:
- `quota_usd=1.00`
- `validity_days=1`
- `category=daily`
- `queue_slot=0`

**When** the feature list is generated
**Then** the list includes:
- "1 USD in credits"
- "Valid for 1 day"
- "Stacks with other plans"
- "No queue slot required"

#### Scenario: Extract features for pay-as-you-go plan

**Given** a PayG plan with:
- `default_quota=0`
- `validity_days=0`
- `category=payg`
- `daily_quota_limit=0`

**When** the feature list is generated
**Then** the list includes:
- "Pay only for what you use"
- "Never expires"
- "No daily limits"
- "No queue slot required"

---

### Requirement: System SHALL highlight recommended plan

The system SHALL visually distinguish a recommended plan to guide user choice.

#### Scenario: Admin sets recommended plan

**Given** the system setting `PricingPageRecommendedPlanId` is set to `2` (Monthly plan)
**And** the Monthly plan is enabled and displayed on the pricing page
**When** the pricing page renders
**Then** the Monthly plan card has a "Most Popular" badge at the top
**And** the card has a distinct border color or shadow effect
**And** the card may be slightly larger than other cards (e.g., scale: 1.05)

#### Scenario: No recommended plan set

**Given** the system setting `PricingPageRecommendedPlanId` is not set or is `null`
**When** the pricing page renders
**Then** all plans are displayed with equal visual weight
**And** no plan has a "Most Popular" badge

---

### Requirement: System SHALL display plan category badges

The system SHALL show a category indicator to help users understand plan types at a glance.

#### Scenario: Display category badge for daily plan

**Given** a plan with `category=daily`
**When** the plan card is rendered
**Then** a blue badge labeled "Daily Card" is displayed at the top of the card

#### Scenario: Display category badge for monthly plan

**Given** a plan with `category=monthly`
**When** the plan card is rendered
**Then** a green badge labeled "Monthly Plan" is displayed at the top of the card

#### Scenario: Display category badge for pay-as-you-go plan

**Given** a plan with `category=payg`
**When** the plan card is rendered
**Then** a purple badge labeled "Pay-as-you-go" is displayed at the top of the card

---

### Requirement: System SHALL provide responsive layout that adapts to screen size

The pricing page SHALL provide an optimal viewing experience across all device sizes.

#### Scenario: Mobile view (width < 768px)

**Given** the user is viewing the pricing page on a mobile device (375px width)
**When** the page renders
**Then** plan cards are displayed in a single column
**And** each card is full-width (minus padding)
**And** cards stack vertically with 16px spacing between them
**And** text is readable without horizontal scrolling

#### Scenario: Tablet view (768px ≤ width < 1024px)

**Given** the user is viewing the pricing page on a tablet (768px width)
**When** the page renders
**Then** plan cards are displayed in a 2-column grid
**And** there is consistent spacing between columns (24px gap)
**And** cards maintain aspect ratio

#### Scenario: Desktop view (width ≥ 1024px)

**Given** the user is viewing the pricing page on a desktop (1440px width)
**When** the page renders
**Then** plan cards are displayed in a 3-column grid
**And** cards are centered with max-width constraint (1200px container)
**And** hover effects work smoothly (shadow, scale)

---

### Requirement: System SHALL provide clear loading and error states

The system SHALL provide clear feedback during data loading and handle errors gracefully.

#### Scenario: Show loading skeleton while fetching plans

**Given** the user navigates to `/plans`
**And** the API request to `/api/plan/enabled` is in progress
**When** the page renders during loading
**Then** 3-4 skeleton cards are displayed in the grid
**And** skeleton cards animate (pulse effect)
**And** actual content is hidden until data loads

#### Scenario: API request succeeds

**Given** the API returns enabled plans successfully
**When** data is received
**Then** skeleton cards are replaced with actual plan cards
**And** the transition is smooth (fade-in animation)
**And** the page is interactive immediately

#### Scenario: API request fails

**Given** the API request to `/api/plan/enabled` fails with status 500
**When** the error is received
**Then** skeleton cards are replaced with an error message
**And** the message says "Unable to load plans. Please try again."
**And** a "Retry" button is displayed
**And** clicking "Retry" re-fetches the data

---

## Relationships

- **Depends on**: `plan-purchase-flow` (for purchase button behavior)
- **Related to**: `add-user-plan-system` (plan data model)
- **Related to**: `add-plan-queue-and-daily-pool` (queue/daily card features)

---

## Acceptance Criteria

- [ ] Pricing page accessible at `/plans` route
- [ ] All enabled plans displayed in responsive grid layout
- [ ] Each plan shows: name, price, discount (if any), features, category badge
- [ ] Recommended plan has visual distinction (badge, border)
- [ ] Loading state shows skeleton cards
- [ ] Error state shows friendly message with retry option
- [ ] Mobile, tablet, desktop layouts work correctly
- [ ] Page works with and without authentication requirement
- [ ] i18n keys support multiple languages
