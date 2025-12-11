## ADDED Requirements

### Requirement: Channel Ratio Configuration
The system SHALL support configuring a ratio multiplier for each channel to adjust billing rates.

#### Scenario: Default channel ratio
- **WHEN** a channel is created without specifying a ratio
- **THEN** the channel ratio SHALL default to 1.0
- **AND** billing calculations SHALL use 1.0 as the channel ratio multiplier

#### Scenario: Configure custom channel ratio
- **WHEN** an administrator sets a channel ratio to a value (e.g., 1.2)
- **THEN** the system SHALL store and use this ratio for billing calculations
- **AND** the ratio SHALL be applied to all requests routed through this channel

#### Scenario: Channel ratio validation
- **WHEN** an administrator attempts to set a channel ratio
- **THEN** the system SHALL validate the ratio is within allowed range (0.1 to 100)
- **AND** the system SHALL reject values outside this range with a clear error message

#### Scenario: Edit channel ratio
- **WHEN** an administrator edits a channel in the management interface
- **THEN** the ratio field SHALL be displayed and editable
- **AND** changes SHALL take effect immediately for new requests after saving

### Requirement: Channel Ratio Visibility Control
Channel ratio information SHALL be hidden from end users and only visible to administrators.

#### Scenario: Admin views channel ratio
- **WHEN** an administrator views the channel list or edit modal
- **THEN** the channel ratio SHALL be visible
- **AND** the ratio value SHALL be displayed with appropriate precision (e.g., 1.20)

#### Scenario: User pricing page excludes channel ratio
- **WHEN** a user views the pricing page
- **THEN** the displayed prices SHALL NOT include channel ratio details
- **AND** only model base price and user group ratio SHALL be shown

#### Scenario: User billing page excludes channel ratio
- **WHEN** a user views their billing history or usage logs
- **THEN** the records SHALL NOT display channel ratio or per-token unit price
- **AND** only total consumption amount SHALL be shown
