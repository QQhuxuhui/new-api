## ADDED Requirements

### Requirement: Extended Billing Formula with Channel Ratio
The billing system SHALL calculate charges using the formula: `Quota = Tokens × ModelRatio × UserGroupRatio × ChannelRatio`.

#### Scenario: Calculate billing with channel ratio
- **WHEN** a request is processed through a channel with ratio 1.2
- **AND** the model ratio is 15, user group ratio is 0.8
- **AND** the request consumes 1000 tokens
- **THEN** the billed quota SHALL be: 1000 × 15 × 0.8 × 1.2 = 14400

#### Scenario: Calculate billing with default channel ratio
- **WHEN** a request is processed through a channel with default ratio (1.0)
- **AND** the model ratio is 15, user group ratio is 1.0
- **AND** the request consumes 1000 tokens
- **THEN** the billed quota SHALL be: 1000 × 15 × 1.0 × 1.0 = 15000
- **AND** this SHALL maintain backward compatibility with existing behavior

#### Scenario: Pre-consumption includes channel ratio
- **WHEN** the system calculates pre-consumption quota before processing a request
- **THEN** the pre-consumption calculation SHALL include the selected channel's ratio
- **AND** the formula SHALL be consistent with final billing calculation

### Requirement: Retry Billing Correction
The system SHALL correctly handle billing when a request retries to a different channel with a different ratio.

#### Scenario: Retry to channel with different ratio
- **WHEN** a request initially routes to channel A (ratio 1.0) and pre-consumes quota Q1
- **AND** channel A fails and the system retries to channel B (ratio 1.5)
- **THEN** the system SHALL return the pre-consumed quota Q1
- **AND** the system SHALL calculate new pre-consumption Q2 based on channel B's ratio
- **AND** the system SHALL deduct Q2 from user's quota
- **AND** the final billing SHALL be based on channel B's ratio

#### Scenario: Retry to channel with same ratio
- **WHEN** a request retries to a channel with the same ratio as the original channel
- **THEN** the system SHALL NOT perform unnecessary quota return and re-deduction
- **AND** the pre-consumed quota SHALL remain unchanged

#### Scenario: Retry billing audit logging
- **WHEN** a retry causes billing recalculation due to channel ratio change
- **THEN** the system SHALL log the original quota, returned quota, and new quota
- **AND** the log SHALL include both channel IDs and their ratios for audit purposes

### Requirement: Billing Consistency for Users
Users SHALL experience consistent billing behavior regardless of internal channel selection.

#### Scenario: Same model different channels
- **WHEN** user A calls model X routed to channel with ratio 1.0
- **AND** user B calls model X routed to channel with ratio 1.2
- **THEN** user A SHALL be charged less than user B for the same token count
- **AND** neither user SHALL see channel-specific pricing details in their billing records

#### Scenario: Billing record format
- **WHEN** a billing record is created for a request
- **THEN** the record SHALL include: model name, token count, total quota consumed
- **AND** the record SHALL NOT include: channel ratio, per-token unit price
- **AND** administrators SHALL be able to access full details including channel information
