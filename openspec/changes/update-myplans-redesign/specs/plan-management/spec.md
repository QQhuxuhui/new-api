## ADDED Requirements

### Requirement: One-Time Manual-Switch Permission Backfill
The system SHALL run one transactional permission backfill guarded by the exact option key `UserPlanAllowSwitchBackfilled`. When the marker is absent, the transaction SHALL enable manual switching on active user-plan rows and non-trial templates, then write the marker only after both updates succeed. When the marker exists, the backfill SHALL never infer state from current zeros or run again.

#### Scenario: First backfill updates only the intended rows
- **GIVEN** the marker does not exist
- **AND** active and inactive user plans plus trial and non-trial templates exist
- **WHEN** startup runs the backfill
- **THEN** every `user_plans` row with `status=1`, including queued active rows, SHALL have `allow_user_switch=1`
- **AND** every non-trial `plans` row SHALL have `default_allow_switch=1`
- **AND** user-plan statuses 2 through 6 SHALL remain unchanged
- **AND** trial templates SHALL retain their existing switch default and status
- **AND** the transaction SHALL finally create `UserPlanAllowSwitchBackfilled=true`

#### Scenario: Failed backfill does not write the marker
- **GIVEN** the marker does not exist
- **WHEN** either permission update fails
- **THEN** the transaction SHALL roll back both permission updates
- **AND** SHALL NOT create `UserPlanAllowSwitchBackfilled`

#### Scenario: Marker prevents replay over administrator choices
- **GIVEN** `UserPlanAllowSwitchBackfilled=true` exists
- **AND** an administrator subsequently sets an active assignment or non-trial template back to `0`
- **WHEN** the application restarts
- **THEN** the backfill SHALL do nothing
- **AND** the administrator's zeros SHALL remain unchanged

#### Scenario: Irreversible migration is prepared and reconciled
- **GIVEN** historical zeros cannot be distinguished from old defaults
- **WHEN** an operator prepares the rollout
- **THEN** the operator SHALL back up `user_plans`, `plans`, and the marker state
- **AND** SHALL inventory active assignment zeros and non-trial template zeros before startup
- **AND** SHALL reapply approved exceptions through administrator permission APIs only after the marker exists
- **AND** application rollback SHALL NOT delete the marker or claim to reverse the backfill

## MODIFIED Requirements

### Requirement: Plan Default Settings
The system SHALL support default permission settings on plan templates and SHALL preserve whether `default_allow_switch` was omitted or explicitly supplied. Omitted creation input for a non-trial plan SHALL resolve to `1`; explicit `0` or `1` SHALL persist unchanged. Omitted update input SHALL preserve the stored value. New assignments SHALL inherit the resolved stored default unless an administrator explicitly overrides it.

#### Scenario: New assignment inherits plan defaults
- **GIVEN** a plan stores `default_allow_switch=0` and `default_allow_toggle_auto=1`
- **WHEN** an administrator assigns the plan without assignment-level permission overrides
- **THEN** the user plan SHALL be created with `allow_user_switch=0`
- **AND** SHALL be created with `allow_user_toggle_auto=1`

#### Scenario: Admin overrides plan defaults on assignment
- **GIVEN** a plan stores `default_allow_switch=0`
- **WHEN** an administrator assigns it with `allow_user_switch=1` explicitly
- **THEN** the user plan SHALL be created with `allow_user_switch=1`

#### Scenario: Omitted non-trial creation defaults to allowed
- **GIVEN** an administrator creates a non-trial plan without `default_allow_switch`
- **WHEN** the plan is persisted
- **THEN** its resolved switch default SHALL be `1`
- **AND** assignments and purchases SHALL inherit `1` unless explicitly overridden

#### Scenario: Explicit zero survives plan creation
- **GIVEN** an administrator creates a plan with `default_allow_switch=0`
- **WHEN** the plan is persisted and reloaded
- **THEN** the stored value SHALL remain `0`
- **AND** the database default SHALL NOT replace it with `1`

#### Scenario: Omitted update preserves stored zero
- **GIVEN** a plan stores `default_allow_switch=0`
- **WHEN** an administrator updates other plan fields but omits `default_allow_switch`
- **THEN** the stored switch default SHALL remain `0`

#### Scenario: Newly seeded trial remains switch-disabled
- **GIVEN** the canonical trial template does not already exist
- **WHEN** canonical plans are seeded at startup
- **THEN** its `default_allow_switch` SHALL be explicitly `0`
- **AND** its disabled status SHALL remain unchanged
- **AND** seeding SHALL NOT overwrite an already existing trial template's administrator-managed default or status
