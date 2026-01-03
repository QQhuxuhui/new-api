# channel-disable-rules Specification

## Purpose

Define the configurable rule system for channel failover triggering, allowing administrators to create custom status code and keyword combinations that trigger the health check system for temporary suspension (with auto-recovery).

## ADDED Requirements

### Requirement: Rule Data Model

The system SHALL store failover trigger rules with support for status codes, keywords, and match type combinations.

#### Scenario: Create rule with AND match type
- **GIVEN** an administrator creates a new failover rule
- **WHEN** setting match_type to "AND"
- **AND** providing status_codes [429] and keywords ["rate_limit", "quota"]
- **THEN** the rule SHALL be saved to the database
- **AND** the rule SHALL only match when BOTH status code 429 AND a keyword is present

#### Scenario: Create rule with OR match type
- **GIVEN** an administrator creates a new failover rule
- **WHEN** setting match_type to "OR"
- **AND** providing status_codes [502, 503] and keywords ["unavailable"]
- **THEN** the rule SHALL match when status code is 502 OR 503 OR message contains "unavailable"

#### Scenario: Create rule with STATUS_ONLY match type
- **GIVEN** an administrator creates a new failover rule
- **WHEN** setting match_type to "STATUS_ONLY"
- **AND** providing status_codes [502, 503]
- **THEN** the rule SHALL match when status code is 502 OR 503
- **AND** keywords field SHALL be ignored

#### Scenario: Create rule with KEYWORD_ONLY match type
- **GIVEN** an administrator creates a new failover rule
- **WHEN** setting match_type to "KEYWORD_ONLY"
- **AND** providing keywords ["billing", "payment"]
- **THEN** the rule SHALL match when message contains "billing" OR "payment"
- **AND** status_codes field SHALL be ignored

### Requirement: Rule Validation

The system SHALL validate rule configuration before saving.

#### Scenario: AND match type requires both fields
- **GIVEN** an administrator attempts to create a rule with match_type "AND"
- **WHEN** status_codes is provided but keywords is empty
- **THEN** the system SHALL reject the rule with error "AND模式必须同时配置状态码和关键词"

#### Scenario: STATUS_ONLY requires status codes
- **GIVEN** an administrator attempts to create a rule with match_type "STATUS_ONLY"
- **WHEN** status_codes is empty
- **THEN** the system SHALL reject the rule with error "STATUS_ONLY模式必须配置状态码"

#### Scenario: KEYWORD_ONLY requires keywords
- **GIVEN** an administrator attempts to create a rule with match_type "KEYWORD_ONLY"
- **WHEN** keywords is empty
- **THEN** the system SHALL reject the rule with error "KEYWORD_ONLY模式必须配置关键词"

#### Scenario: Rule name is required
- **GIVEN** an administrator attempts to create a rule
- **WHEN** name field is empty
- **THEN** the system SHALL reject the rule with error "规则名称不能为空"

### Requirement: Rule Matching Integration

User-defined rules SHALL be evaluated after all hardcoded logic in ShouldTriggerChannelFailover.

#### Scenario: User rule matches after hardcoded rules pass
- **GIVEN** an error with status code 429 and message "Custom rate limit exceeded"
- **AND** hardcoded rules do not match this error (429 would normally match 4xx rule)
- **AND** a user-defined rule exists: {status_codes: [429], keywords: ["custom"], match_type: "AND"}
- **WHEN** ShouldTriggerChannelFailover evaluates the error
- **THEN** the user-defined rule SHALL match
- **AND** the system SHALL return true (trigger failover recording)
- **AND** the system SHALL log "故障转移规则「<rule_name>」匹配成功"

#### Scenario: Hardcoded rules take precedence
- **GIVEN** an error with status code 401
- **WHEN** ShouldTriggerChannelFailover evaluates the error
- **THEN** the hardcoded 4xx rule SHALL match first
- **AND** user-defined rules SHALL NOT be evaluated (short-circuit)

#### Scenario: Network error keywords compatibility
- **GIVEN** an error message containing "connection refused"
- **AND** this keyword matches hardcoded network error check
- **WHEN** ShouldTriggerChannelFailover evaluates the error
- **THEN** the hardcoded network keyword match SHALL trigger before user-defined rules
- **AND** user-defined rules SHALL NOT be evaluated (short-circuit)

#### Scenario: No matching rules
- **GIVEN** an error with status code 400 and message "Bad request format"
- **AND** no hardcoded rules match (400 is excluded)
- **AND** no user-defined rules match
- **WHEN** ShouldTriggerChannelFailover evaluates the error
- **THEN** the system SHALL return false (do not trigger failover)

### Requirement: Health Check Integration

When rules match, the system SHALL trigger health check recording for temporary suspension.

#### Scenario: Rule match triggers RecordChannelFailure
- **GIVEN** a user-defined rule matches an error
- **AND** ShouldTriggerChannelFailover returns true
- **WHEN** the caller (relay controller) handles the result
- **THEN** RecordChannelFailure SHALL be called
- **AND** the failure SHALL be recorded to the sliding window (60s, 6 buckets)

#### Scenario: Failure rate triggers temporary suspension
- **GIVEN** multiple failures recorded to the sliding window
- **AND** failure rate exceeds 30% for 3 consecutive periods
- **WHEN** the health check system evaluates the channel
- **THEN** the channel SHALL be temporarily suspended
- **AND** suspension SHALL use exponential backoff (5→10→20→40→60 minutes)

#### Scenario: Success request auto-recovers channel
- **GIVEN** a channel is temporarily suspended
- **AND** a successful request is made after suspension expires
- **WHEN** RecordChannelSuccess is called
- **THEN** the channel SHALL be automatically recovered
- **AND** no manual intervention SHALL be required

### Requirement: Rule Caching

The system SHALL cache enabled rules in memory to minimize database queries.

#### Scenario: Cache hit within TTL
- **GIVEN** rules were loaded 3 minutes ago
- **AND** cache TTL is 5 minutes
- **WHEN** GetEnabledDisableRules is called
- **THEN** the cached rules SHALL be returned
- **AND** no database query SHALL be executed

#### Scenario: Cache refresh after TTL expiry
- **GIVEN** rules were loaded 6 minutes ago
- **AND** cache TTL is 5 minutes
- **WHEN** GetEnabledDisableRules is called
- **THEN** the system SHALL query the database for enabled rules
- **AND** the cache SHALL be updated with fresh data

#### Scenario: Cache invalidation on CRUD
- **GIVEN** cached rules exist
- **WHEN** a rule is created, updated, or deleted
- **THEN** the cache SHALL be invalidated immediately
- **AND** the next GetEnabledDisableRules call SHALL refresh from database

### Requirement: Rule Management API

The system SHALL provide RESTful APIs for rule CRUD operations.

#### Scenario: List all rules
- **GIVEN** 3 failover rules exist in the database
- **WHEN** GET /api/channel/disable-rules is called by an admin
- **THEN** the response SHALL contain all 3 rules
- **AND** rules SHALL be sorted by priority DESC, id ASC

#### Scenario: Create rule
- **GIVEN** valid rule data
- **WHEN** POST /api/channel/disable-rules is called by an admin
- **THEN** the rule SHALL be created in the database
- **AND** the cache SHALL be invalidated
- **AND** the response SHALL contain the created rule with id

#### Scenario: Update rule
- **GIVEN** an existing rule with id 5
- **WHEN** PUT /api/channel/disable-rules/5 is called with updated data
- **THEN** the rule SHALL be updated in the database
- **AND** the cache SHALL be invalidated

#### Scenario: Delete rule
- **GIVEN** an existing rule with id 5
- **WHEN** DELETE /api/channel/disable-rules/5 is called
- **THEN** the rule SHALL be deleted from the database
- **AND** the cache SHALL be invalidated

#### Scenario: Non-admin access denied
- **GIVEN** a non-admin user
- **WHEN** any /api/channel/disable-rules endpoint is called
- **THEN** the system SHALL return HTTP 403 Forbidden

### Requirement: Rule Testing

The system SHALL provide a testing endpoint to validate rule matching.

#### Scenario: Test with matching rule
- **GIVEN** a rule {status_codes: [429], keywords: ["quota"], match_type: "AND"}
- **WHEN** POST /api/channel/disable-rules/test with {status_code: 429, error_message: "quota exceeded"}
- **THEN** the response SHALL show this rule as matched
- **AND** the response SHALL include status_match: true, keyword_match: true

#### Scenario: Test with non-matching rule
- **GIVEN** a rule {status_codes: [429], keywords: ["quota"], match_type: "AND"}
- **WHEN** POST /api/channel/disable-rules/test with {status_code: 429, error_message: "rate limit"}
- **THEN** the response SHALL show this rule as NOT matched
- **AND** the response SHALL include status_match: true, keyword_match: false

#### Scenario: Test shows all rules and hardcoded behavior
- **GIVEN** 5 rules exist (some enabled, some disabled)
- **WHEN** POST /api/channel/disable-rules/test is called
- **THEN** the response SHALL include whether hardcoded rules would match
- **AND** the response SHALL include match results for ALL user rules
- **AND** disabled rules SHALL show matched: false

### Requirement: Frontend Management UI

The system SHALL provide a management interface for failover rules.

#### Scenario: View rule list
- **GIVEN** an admin navigates to 运营设置 > 渠道故障转移规则
- **THEN** the page SHALL display all rules in a table
- **AND** the table SHALL show: name, status_codes, keywords, match_type, enabled, actions
- **AND** an info banner SHALL explain that matched rules trigger temporary suspension

#### Scenario: Create rule via modal
- **GIVEN** an admin clicks "新建规则"
- **THEN** a modal SHALL appear with form fields
- **AND** the modal SHALL include: name, match_type (radio), status_codes, keywords, description, enabled toggle
- **WHEN** the admin fills the form and clicks "保存"
- **THEN** the rule SHALL be created via API
- **AND** the table SHALL refresh to show the new rule

#### Scenario: Edit rule via modal
- **GIVEN** an admin clicks "编辑" on a rule row
- **THEN** a modal SHALL appear with the rule's current values
- **WHEN** the admin modifies values and clicks "保存"
- **THEN** the rule SHALL be updated via API

#### Scenario: Delete rule with confirmation
- **GIVEN** an admin clicks "删除" on a rule row
- **THEN** a confirmation dialog SHALL appear
- **WHEN** the admin confirms deletion
- **THEN** the rule SHALL be deleted via API
- **AND** the table SHALL refresh

#### Scenario: Toggle rule enabled status
- **GIVEN** a rule displayed in the table
- **WHEN** the admin clicks the enabled toggle
- **THEN** the rule's enabled status SHALL be updated via API
- **AND** the toggle SHALL reflect the new state

#### Scenario: Test rules panel
- **GIVEN** an admin enters a status code and error message in the test panel
- **WHEN** the admin clicks "测试"
- **THEN** the system SHALL call the test API
- **AND** the results SHALL display which rules matched and why
- **AND** the results SHALL show whether hardcoded rules would also match
