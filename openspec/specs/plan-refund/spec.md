# plan-refund Specification

## Purpose
TBD - created by archiving change add-plan-queue-and-daily-pool. Update Purpose after archive.
## Requirements
### Requirement: Refund Eligibility for Queue Plans
The system SHALL allow refunds for subscription plans that are in queue and not yet activated, within 7 days of purchase.

#### Scenario: Refund eligible queue plan
- **GIVEN** user purchased a monthly plan 3 days ago
- **AND** the plan is in queue (not activated)
- **WHEN** user requests refund
- **THEN** the system marks plan as refund_requested
- **AND** admin can approve full refund
- **AND** plan is removed from queue after refund

#### Scenario: Refund ineligible - already activated
- **GIVEN** user has an activated plan (is_current=true)
- **WHEN** user requests refund
- **THEN** the system rejects with error "Activated plans are not refundable"

#### Scenario: Refund ineligible - past 7 days
- **GIVEN** user purchased a plan 10 days ago (in queue)
- **WHEN** user requests refund
- **THEN** the system rejects with error "Refund period (7 days) has expired"

#### Scenario: Daily card not refundable
- **GIVEN** user purchased a daily card 1 hour ago
- **WHEN** user requests refund
- **THEN** the system rejects with error "Daily cards are not refundable"

### Requirement: Refund Processing by Admin
The system SHALL allow administrators to process refund requests.

#### Scenario: Admin approves refund
- **GIVEN** a pending refund request for a queue plan
- **WHEN** admin approves the refund
- **THEN** the payment is reversed to original payment method
- **AND** the plan is removed from user's queue
- **AND** queue positions are reordered
- **AND** action is logged with admin ID

#### Scenario: Admin rejects refund
- **GIVEN** a pending refund request
- **WHEN** admin rejects with reason "Policy violation"
- **THEN** the plan remains in queue
- **AND** refund status is set to "rejected"
- **AND** rejection reason is recorded

