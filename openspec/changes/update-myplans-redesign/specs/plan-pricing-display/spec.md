## ADDED Requirements

### Requirement: System SHALL provide a grouped responsive My Plans management view
The system SHALL present `/console/myplans` as a state-grouped management view that renders each user-plan instance exactly once, keeps the current plan prominent, and uses compact responsive cards for the remaining plans. The view SHALL consume the existing plan, quota-status, and billing-status reads plus the new `pinned` field without adding a frontend dependency.

#### Scenario: MyPlans renders in the defined information order
- **GIVEN** the user has a current plan, daily-pool data, plans in multiple states, and a wallet balance
- **WHEN** MyPlans renders
- **THEN** the order SHALL be header, current-plan hero, daily-pool band, available plans, queued plans, locked plans, collapsed inactive plans, wallet card, and disclaimer
- **AND** the daily-pool band SHALL render whenever `billing_status.daily_pool` is present, including a zero-total pool
- **AND** empty plan sections SHALL be omitted
- **AND** the old texture banner and three quick-stat cards SHALL not render

#### Scenario: Each plan is assigned to one group by precedence
- **GIVEN** the plan list contains overlapping current, inactive, locked, and queued state flags
- **WHEN** the frontend groups the list
- **THEN** `is_current=1` SHALL take first precedence
- **AND** statuses 2 through 6 or an active nonzero `expires_at` at or before now SHALL take second precedence as inactive
- **AND** `locked=1` SHALL take third precedence, including locked queued rows
- **AND** `queue_position>0` with `started_at=0` SHALL take fourth precedence as queued
- **AND** every remaining status-1 row SHALL be available
- **AND** no plan SHALL appear in more than one group

#### Scenario: Sections use deterministic sorting
- **GIVEN** each plan has a user-plan ID, plan priority, queue position, and expiry
- **WHEN** grouped sections render
- **THEN** available and locked plans SHALL sort by priority descending then ID ascending
- **AND** queued plans SHALL sort by queue position ascending then ID ascending
- **AND** inactive plans SHALL sort by expiry descending, with zero expiry last and ID descending as the tie-breaker

#### Scenario: Current plan owns the only editable auto-switch control
- **GIVEN** a current plan is returned
- **WHEN** MyPlans renders
- **THEN** the current plan SHALL use the only full-detail hero card
- **AND** the page SHALL contain exactly one editable auto-switch control, on that hero
- **AND** a pinned current plan SHALL show a manual-selection label and a clear action
- **AND** the clear action SHALL explain that it restores scheduling and enables auto-switch while exhaustion rescue and failover remain available
- **AND** auto-switch state in every detail modal SHALL be read-only

#### Scenario: Compact plan actions match server eligibility
- **GIVEN** a non-current plan is active, unexpired, unlocked, not queued, and has positive remaining quota
- **WHEN** its compact card renders
- **THEN** a set-current action SHALL be present
- **AND** `can_switch=0` SHALL keep that action disabled with an administrator explanation
- **AND** an eligible lock action SHALL not require positive quota
- **AND** only a user-applied lock SHALL expose unlock
- **AND** current, queued, expired, inactive, or locked targets SHALL not expose an enabled set-current action

#### Scenario: Queued plans appear once with joined ETA metadata
- **GIVEN** `/api/my_plans/billing-status` returns queued metadata
- **WHEN** the frontend joins it to the plan list
- **THEN** the join SHALL use `user_plan.id`
- **AND** each unlocked queued plan SHALL appear only in the queued section
- **AND** each locked queued plan SHALL appear only in the locked section with its queue-position label
- **AND** the old separate queue renderer and queue modal SHALL not render

#### Scenario: Inactive plans are summarized and inspectable
- **GIVEN** inactive plans include expired, disabled, completed, forfeited, and revoked states
- **WHEN** MyPlans first renders
- **THEN** the inactive section SHALL be collapsed
- **AND** its header SHALL show counts by inactive state
- **AND** expanding it SHALL show muted compact cards without action controls
- **AND** each inactive card SHALL still open the read-only detail modal

#### Scenario: Every compact plan exposes read-only details
- **GIVEN** any available, queued, locked, or inactive compact plan is displayed
- **WHEN** the user activates its card without activating an action button
- **THEN** a responsive detail modal SHALL show full quota, used and remaining values, priority, validity and timestamps, daily limit, lock owner and reason, administrator note, queue ETA, pin, and auto-switch state when available
- **AND** action-button events SHALL not accidentally open the modal

#### Scenario: Responsive grid remains dense and stable
- **GIVEN** the viewport is mobile, tablet, or desktop
- **WHEN** compact sections render
- **THEN** the grid SHALL use one column below 768px, two columns from 768px, and three columns from 1024px
- **AND** controls and loading states SHALL not resize their fixed action regions or overlap text
- **AND** no horizontal scrolling or clipped button text SHALL occur
- **AND** at 1080x1080 the current hero and the first available row SHALL be visible without scrolling
- **AND** when the available section is aligned to the top of a 1080px-high viewport, at least nine compact cards SHALL fit within that viewport when enough plans exist

#### Scenario: Action outcomes refresh without hiding the page
- **GIVEN** the user invokes switch, lock, unlock, auto-switch, or clear-pin
- **WHEN** the action succeeds or returns HTTP 200 with `success:false`
- **THEN** only the initiating control SHALL show loading
- **AND** the page SHALL remain visible
- **AND** the frontend SHALL display the returned server message for a domain failure without string matching
- **AND** SHALL refresh `/api/my_plans/`, `/api/my_plans/quota-status`, and `/api/my_plans/billing-status` after either outcome

#### Scenario: Empty state respects route availability
- **GIVEN** the user has no assigned plans
- **WHEN** MyPlans renders
- **THEN** it SHALL show an empty state
- **AND** a purchase action SHALL appear only when application status has loaded, `recharge_disabled=false`, and the `/plans` route is enabled
- **AND** activating the visible purchase action SHALL navigate to `/plans`

#### Scenario: MyPlans is localized and keyboard operable
- **GIVEN** the runtime locale is `zh`, `en`, `fr`, `ja`, or `ru`
- **WHEN** the redesigned page renders
- **THEN** every literal and computed MyPlans key SHALL have a translation
- **AND** long localized text and an 80-character plan name SHALL not overlap adjacent controls
- **AND** cards, buttons, switches, collapse controls, and modal controls SHALL expose visible keyboard focus and usable keyboard activation
- **AND** nonessential motion SHALL respect reduced-motion preferences

## MODIFIED Requirements

### Requirement: Display wallet balance as pay-as-you-go plan in my plans page
The system SHALL display the user's wallet balance as a virtual pay-as-you-go card after all real-plan state sections in MyPlans. The wallet card SHALL remain visible at zero balance, when recharge is disabled, and when the `/plans` route is disabled. Only its recharge CTA SHALL be gated by recharge and route availability.

#### Scenario: User views my plans page with wallet balance
- **GIVEN** the authenticated user's wallet balance is positive
- **WHEN** the user visits `/console/myplans`
- **THEN** the page SHALL show a pay-as-you-go wallet card after the inactive-plan section
- **AND** the card SHALL show the balance and a never-expires label
- **AND** the recharge CTA SHALL be visible only when `recharge_disabled=false` and the `/plans` route is enabled

#### Scenario: User with zero wallet balance
- **GIVEN** the authenticated user's wallet balance is zero
- **WHEN** the user visits MyPlans
- **THEN** the wallet card SHALL still render with a zero balance
- **AND** recharge availability SHALL follow the same recharge and route gates

#### Scenario: Recharge is disabled or pricing route is unavailable
- **GIVEN** `recharge_disabled=true` or application navigation disables `/plans`
- **WHEN** the wallet card renders
- **THEN** the wallet card and balance SHALL remain visible
- **AND** the recharge CTA SHALL not render
- **AND** no control SHALL navigate to an unavailable pricing route

#### Scenario: User opens pay-as-you-go recharge options
- **GIVEN** `recharge_disabled=false` and the `/plans` route is enabled
- **WHEN** the user activates the wallet recharge CTA
- **THEN** the system SHALL navigate to `/plans?category=payg`
- **AND** the pricing page SHALL select the pay-as-you-go category
