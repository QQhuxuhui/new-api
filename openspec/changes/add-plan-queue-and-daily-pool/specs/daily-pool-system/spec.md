## ADDED Requirements

### Requirement: Daily Pool Management
The system SHALL provide a daily quota pool mechanism where users can purchase emergency quota that expires at the end of the day (23:59:59).

#### Scenario: Purchase daily card adds to pool
- **GIVEN** a user with no daily pool for today
- **WHEN** user purchases a daily card with 40 USD quota
- **THEN** the system creates a daily pool record for today with total_quota=40 USD
- **AND** the pool expires at 23:59:59 of the same day

#### Scenario: Multiple daily card purchases stack
- **GIVEN** a user with existing daily pool of 40 USD today
- **WHEN** user purchases another daily card with 40 USD quota
- **THEN** the system updates the daily pool total_quota to 80 USD
- **AND** both amounts share the same expiry time

#### Scenario: Daily pool expires at midnight
- **GIVEN** a user with daily pool of 100 USD, 50 USD used
- **WHEN** the clock reaches 23:59:59
- **THEN** the remaining 50 USD quota is cleared
- **AND** the daily pool record is marked expired

#### Scenario: Query today's daily pool
- **GIVEN** a user with daily pool of 80 USD, 30 USD used
- **WHEN** user queries their daily pool status
- **THEN** the system returns total=80 USD, used=30 USD, remaining=50 USD
- **AND** shows expiry time as today 23:59:59

### Requirement: Daily Pool Does Not Occupy Queue
The system SHALL NOT count daily card purchases against the 10-slot plan queue limit.

#### Scenario: Purchase daily card with full queue
- **GIVEN** a user with 10 subscription plans in queue (queue full)
- **WHEN** user attempts to purchase a daily card
- **THEN** the purchase succeeds
- **AND** the daily pool is updated
- **AND** the queue count remains at 10

### Requirement: Late Night Purchase Warning
The system SHALL warn users when purchasing daily cards after 22:00 local time.

#### Scenario: Purchase after 22:00
- **GIVEN** current time is 22:30
- **WHEN** user initiates daily card purchase
- **THEN** the system displays warning "Today only has X hours remaining, purchase carefully"
- **AND** user can still proceed with purchase
