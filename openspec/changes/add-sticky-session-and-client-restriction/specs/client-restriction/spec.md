# Spec: Client Restriction

## Overview

Client restriction enables API tokens to be limited to specific client applications based on User-Agent header validation, providing an additional security layer beyond IP-based restrictions.

## ADDED Requirements

### Requirement: Token client restriction configuration

API tokens MUST support configuration for client restrictions with pattern-based User-Agent filtering.

#### Scenario: Administrator enables client restriction for a token

**Given** an administrator is editing a token configuration
**When** they enable client restriction
**And** they add allowed client patterns `Claude-Code-CLI/*` and `VSCode/*`
**Then** the token's `ClientRestrictionEnabled` field is set to `true`
**And** the token's `AllowedClients` field contains `"Claude-Code-CLI/*\nVSCode/*"`
**And** requests using this token will be validated against these patterns

#### Scenario: Client restriction disabled by default

**Given** a new token is created without specifying client restriction settings
**When** the token is saved to the database
**Then** the token's `ClientRestrictionEnabled` field defaults to `false`
**And** the token's `AllowedClients` field defaults to `NULL` or empty string
**And** requests using this token will not perform User-Agent validation

#### Scenario: Administrator disables client restriction

**Given** a token has client restriction enabled with patterns configured
**When** the administrator disables client restriction
**Then** the token's `ClientRestrictionEnabled` field is set to `false`
**And** the token's `AllowedClients` field remains unchanged (preserved)
**And** requests using this token will bypass User-Agent validation

### Requirement: User-Agent validation

The system MUST validate request User-Agent headers against configured client patterns and reject non-matching clients.

#### Scenario: Allowed client passes validation

**Given** a token with `ClientRestrictionEnabled=true`
**And** allowed clients include pattern `Claude-Code-CLI/*`
**When** a request is made with User-Agent `Claude-Code-CLI/1.0.0`
**Then** the User-Agent matches the pattern
**And** the request passes client validation
**And** processing continues to channel selection

#### Scenario: Disallowed client is rejected

**Given** a token with `ClientRestrictionEnabled=true`
**And** allowed clients include patterns `Claude-Code-CLI/*` and `VSCode/*`
**When** a request is made with User-Agent `Python-requests/2.28.0`
**Then** the User-Agent does not match any pattern
**And** the request is rejected with HTTP 403 Forbidden
**And** the error message is "Client not allowed. This API key is restricted to specific clients."
**And** the request does not proceed to channel selection

#### Scenario: Missing User-Agent is rejected

**Given** a token with `ClientRestrictionEnabled=true`
**And** allowed clients include pattern `Claude-Code-CLI/*`
**When** a request is made without a User-Agent header
**Then** the User-Agent is empty string
**And** the request is rejected with HTTP 403 Forbidden
**And** the error message is "Client not allowed. This API key is restricted to specific clients."

#### Scenario: Validation skipped when restriction disabled

**Given** a token with `ClientRestrictionEnabled=false`
**When** a request is made with any User-Agent (or none)
**Then** no User-Agent validation is performed
**And** the request proceeds to channel selection regardless of User-Agent

### Requirement: Pattern matching algorithms

The system MUST support multiple pattern matching algorithms for flexible client identification.

#### Scenario: Exact match pattern

**Given** an allowed client pattern `Claude-Code-CLI/1.0.0` (no wildcards)
**When** validating User-Agent `Claude-Code-CLI/1.0.0`
**Then** the exact string match succeeds
**And** the client is allowed

#### Scenario: Exact match fails for different version

**Given** an allowed client pattern `Claude-Code-CLI/1.0.0` (no wildcards)
**When** validating User-Agent `Claude-Code-CLI/1.1.0`
**Then** the exact string match fails
**And** no other pattern matches
**And** the client is rejected

#### Scenario: Wildcard pattern matches version variants

**Given** an allowed client pattern `Claude-Code-CLI/*`
**When** validating User-Agent `Claude-Code-CLI/1.0.0`
**Then** the wildcard pattern matches (prefix `Claude-Code-CLI/` matches)
**And** the client is allowed

**When** validating User-Agent `Claude-Code-CLI/2.5.3`
**Then** the wildcard pattern matches
**And** the client is allowed

#### Scenario: Wildcard pattern requires exact prefix with slash

**Given** an allowed client pattern `Claude-Code-CLI/*`
**When** validating User-Agent `Claude-Code-CLI-Fork/1.0.0`
**Then** the wildcard pattern does not match (prefix is `Claude-Code-CLI-Fork/`, not `Claude-Code-CLI/`)
**And** the client is rejected

#### Scenario: Wildcard pattern rejects without slash

**Given** an allowed client pattern `VSCode/*`
**When** validating User-Agent `VSCodeInsiders`
**Then** the wildcard pattern does not match (no slash separator)
**And** the client is rejected

#### Scenario: Regex pattern matches complex patterns

**Given** an allowed client pattern `regex:^(VSCode|Cursor)/.*`
**When** validating User-Agent `VSCode/1.85.0`
**Then** the regex pattern matches
**And** the client is allowed

**When** validating User-Agent `Cursor/0.12.0`
**Then** the regex pattern matches
**And** the client is allowed

**When** validating User-Agent `Claude-Code-CLI/1.0.0`
**Then** the regex pattern does not match
**And** the client is rejected (if no other patterns match)

#### Scenario: Invalid regex pattern is ignored

**Given** an allowed client pattern `regex:[invalid(regex`
**When** validating any User-Agent
**Then** the regex compilation or matching fails
**And** the pattern is treated as non-matching
**And** other patterns are still evaluated

### Requirement: Pattern list parsing

The system MUST parse the allowed clients text field into individual patterns, handling comments and formatting.

#### Scenario: Parse newline-separated patterns

**Given** a token's `AllowedClients` field contains:
```
Claude-Code-CLI/*
VSCode/*
curl/7.*
```
**When** the patterns are parsed via `GetAllowedClientsMap()`
**Then** the result is a list containing 3 patterns:
- `Claude-Code-CLI/*`
- `VSCode/*`
- `curl/7.*`

#### Scenario: Parse patterns with whitespace

**Given** a token's `AllowedClients` field contains:
```
  Claude-Code-CLI/*
VSCode/*
  Cursor/*
```
**When** the patterns are parsed via `GetAllowedClientsMap()`
**Then** leading and trailing whitespace is trimmed
**And** the result contains patterns without extra spaces:
- `Claude-Code-CLI/*`
- `VSCode/*`
- `Cursor/*`

#### Scenario: Parse patterns with comments

**Given** a token's `AllowedClients` field contains:
```
# Official CLI only
Claude-Code-CLI/*
# Allow VSCode
VSCode/*
```
**When** the patterns are parsed via `GetAllowedClientsMap()`
**Then** lines starting with `#` are ignored
**And** the result contains 2 patterns:
- `Claude-Code-CLI/*`
- `VSCode/*`

#### Scenario: Handle empty lines

**Given** a token's `AllowedClients` field contains:
```
Claude-Code-CLI/*

VSCode/*

```
**When** the patterns are parsed via `GetAllowedClientsMap()`
**Then** empty lines are ignored
**And** the result contains 2 patterns:
- `Claude-Code-CLI/*`
- `VSCode/*`

#### Scenario: Handle NULL or empty AllowedClients

**Given** a token's `AllowedClients` field is NULL
**When** the patterns are parsed via `GetAllowedClientsMap()`
**Then** the result is an empty list

**Given** a token's `AllowedClients` field is an empty string `""`
**When** the patterns are parsed via `GetAllowedClientsMap()`
**Then** the result is an empty list

### Requirement: User-Agent logging

The system MUST log User-Agent headers for monitoring and audit purposes when client restriction is enabled.

#### Scenario: User-Agent stored in context on validation

**Given** a token with `ClientRestrictionEnabled=true`
**And** a request with User-Agent `Claude-Code-CLI/1.0.0`
**When** client validation is performed
**Then** the User-Agent is retrieved from request headers
**And** the context key `ContextKeyUserAgent` is set to `Claude-Code-CLI/1.0.0`
**And** downstream middleware can access the User-Agent from context

#### Scenario: User-Agent available for audit logging

**Given** a request with User-Agent `VSCode/1.85.0`
**And** the User-Agent was stored in context during authentication
**When** request logging is performed
**Then** the log entry includes the User-Agent field
**And** administrators can query logs by User-Agent for security analysis

### Requirement: Integration with existing authentication

Client restriction validation MUST be integrated into the existing token authentication middleware.

#### Scenario: Client restriction checked after token validation

**Given** a request with a valid API key
**When** the `TokenAuth()` middleware is executed
**Then** the API key is validated first (existing logic)
**And** IP restrictions are checked second (existing logic)
**And** client restrictions are checked third (new logic)
**And** only if all validations pass does the request proceed

#### Scenario: Client restriction failure prevents request processing

**Given** a request with a valid API key and allowed IP
**And** a token with `ClientRestrictionEnabled=true` allowing only `Claude-Code-CLI/*`
**When** the request has User-Agent `Python-requests/2.28.0`
**Then** token validation passes
**And** IP restriction check passes
**And** client restriction check fails
**And** the request is rejected with HTTP 403
**And** channel selection is never reached

### Requirement: Database schema changes

The Token model MUST be extended with client restriction configuration fields.

#### Scenario: Token table migration adds client restriction fields

**Given** the database schema is being migrated
**When** the migration for client restrictions is applied
**Then** the `tokens` table has a new column `client_restriction_enabled` of type BOOLEAN with default `0`
**And** the `tokens` table has a new column `allowed_clients` of type TEXT with default empty string
**And** existing tokens have `client_restriction_enabled=false` and `allowed_clients=''`

#### Scenario: Token update includes client restriction fields

**Given** a token exists with ID 123
**When** the token is updated via `Token.Update()`
**Then** the `client_restriction_enabled` field is included in the update
**And** the `allowed_clients` field is included in the update
**And** the Redis cache for the token is updated asynchronously

### Requirement: Error messages and user feedback

The system MUST provide clear, actionable error messages when client validation fails.

#### Scenario: Rejection message is clear and generic

**Given** a request is rejected due to client restriction
**When** the error response is returned
**Then** the HTTP status code is 403 Forbidden
**And** the error message is "Client not allowed. This API key is restricted to specific clients."
**And** the error type is `invalid_request_error` (OpenAI-compatible format)
**And** the message does not leak specific allowed patterns (security)

#### Scenario: No information leakage in error messages

**Given** a token with allowed clients `Claude-Code-CLI/*` and `VSCode/*`
**When** a request with User-Agent `Postman` is rejected
**Then** the error message does not reveal the allowed patterns
**And** an attacker cannot enumerate allowed clients via error messages

## MODIFIED Requirements

### Requirement: Token model Update method

The `Token.Update()` method MUST include new client restriction fields in the update operation.

#### Scenario: Token update persists client restriction fields

**Given** a token with ID 123 has `ClientRestrictionEnabled=false`
**When** the token is modified to set `ClientRestrictionEnabled=true` and `AllowedClients="VSCode/*"`
**And** the `Update()` method is called
**Then** the database update includes both `client_restriction_enabled` and `allowed_clients`
**And** the changes are persisted to the database
**And** the Redis cache is updated asynchronously

**Note**: The existing `Update()` method in `model/token.go` line 187 must be modified to include the new fields in the `Select()` call.

## REMOVED Requirements

None
