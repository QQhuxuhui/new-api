# Design: Configurable Channel Failover Rules

## Overview

This document describes the technical design for the configurable channel failover trigger rules feature. The rules extend `ShouldTriggerChannelFailover` to enable the health check system's temporary suspension mechanism (with auto-recovery).

## System Context

```
                     用户请求失败
                          │
                          ▼
              ┌───────────────────────┐
              │ ShouldTriggerChannelFailover │
              └───────────────────────┘
                          │
          ┌───────────────┼───────────────┐
          ▼               ▼               ▼
    ┌──────────┐   ┌──────────┐   ┌──────────────┐
    │ HTTP状态码│   │ 网络错误  │   │ 用户自定义规则│  ← 新增
    │ (4xx/5xx)│   │ 关键词    │   │   (数据库表)  │
    └──────────┘   └──────────┘   └──────────────┘
          │               │               │
          └───────────────┴───────────────┘
                          │ 任一匹配
                          ▼
              ┌───────────────────────┐
              │  RecordChannelFailure  │
              │    (健康检查系统)       │
              └───────────────────────┘
                          │
                          ▼
              ┌───────────────────────┐
              │  滑动窗口失败率统计      │
              │  60秒窗口，6个桶        │
              └───────────────────────┘
                          │
                    失败率 > 30%
                    连续3个周期
                          ▼
              ┌───────────────────────┐
              │  临时暂停（指数退避）    │
              │  5→10→20→40→60分钟     │
              │  成功请求后自动恢复      │
              └───────────────────────┘
```

## Two Independent Chains (For Reference)

The system has two distinct error handling chains. This feature modifies only the Health Check Chain.

### Health Check Chain (This Feature's Target)
```go
// controller/relay.go
if service.ShouldTriggerChannelFailover(newAPIError.StatusCode, newAPIError.Error()) ||
    newAPIError.StatusCode == 504 || newAPIError.StatusCode == 524 {
    service.RecordChannelFailure(channel.Id, newAPIError.StatusCode, newAPIError.Error())
}
```
- **Effect**: Temporary suspension (5-60 min)
- **Recovery**: Automatic after suspension period
- **Mechanism**: Sliding window failure rate (30% threshold)

### Disable Chain (NOT Modified)
```go
// controller/relay.go
if service.ShouldDisableChannel(channelError.ChannelType, err) && channelError.AutoBan {
    service.DisableChannel(channelError, err.Error())
}
```
- **Effect**: Permanent disable
- **Recovery**: Manual intervention required

## Data Model

### Database Schema

```sql
CREATE TABLE channel_disable_rules (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(100) NOT NULL COMMENT '规则名称',
    status_codes JSON COMMENT '状态码列表，如 [401, 403, 429]',
    keywords JSON COMMENT '关键词列表，如 ["quota", "expired"]',
    match_type ENUM('AND', 'OR', 'STATUS_ONLY', 'KEYWORD_ONLY')
        NOT NULL DEFAULT 'AND' COMMENT '匹配类型',
    enabled TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否启用',
    description TEXT COMMENT '规则说明',
    priority INT NOT NULL DEFAULT 0 COMMENT '优先级（仅用于排序显示）',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_enabled (enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='渠道故障转移触发规则表';
```

### Go Model

```go
type ChannelDisableRule struct {
    Id          int       `json:"id" gorm:"primaryKey"`
    Name        string    `json:"name" gorm:"type:varchar(100);not null"`
    StatusCodes []int     `json:"status_codes" gorm:"serializer:json"`
    Keywords    []string  `json:"keywords" gorm:"serializer:json"`
    MatchType   string    `json:"match_type" gorm:"type:varchar(20);default:AND"`
    Enabled     bool      `json:"enabled" gorm:"default:true"`
    Description string    `json:"description" gorm:"type:text"`
    Priority    int       `json:"priority" gorm:"default:0"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

## Match Type Logic

### AND Match
Both status code AND keyword must match:
```go
func (r *ChannelDisableRule) matchAND(statusCode int, msg string) bool {
    if len(r.StatusCodes) == 0 || len(r.Keywords) == 0 {
        return false // AND requires both defined
    }
    return r.hasStatusCode(statusCode) && r.hasKeyword(msg)
}
```

### OR Match
Either status code OR keyword matches:
```go
func (r *ChannelDisableRule) matchOR(statusCode int, msg string) bool {
    if len(r.StatusCodes) > 0 && r.hasStatusCode(statusCode) {
        return true
    }
    if len(r.Keywords) > 0 && r.hasKeyword(msg) {
        return true
    }
    return false
}
```

### STATUS_ONLY Match
Only check status code:
```go
func (r *ChannelDisableRule) matchStatusOnly(statusCode int) bool {
    return len(r.StatusCodes) > 0 && r.hasStatusCode(statusCode)
}
```

### KEYWORD_ONLY Match
Only check keyword in error message:
```go
func (r *ChannelDisableRule) matchKeywordOnly(msg string) bool {
    return len(r.Keywords) > 0 && r.hasKeyword(msg)
}
```

## Caching Strategy

### Cache Structure
```go
var (
    disableRulesCache     []*ChannelDisableRule
    disableRulesCacheLock sync.RWMutex
    disableRulesCacheTime time.Time
    disableRulesCacheTTL  = 5 * time.Minute
)
```

### Cache Operations

1. **Read Path**: Check TTL → return cached if valid → else refresh from DB
2. **Write Path**: CRUD operation → invalidate cache (set time to zero)
3. **Refresh**: Lock → query DB → update cache → update timestamp

### Multi-Instance Considerations

With 5-minute TTL, different instances may have slightly different rule sets for up to 5 minutes after a change. This is acceptable because:
- Rule changes are infrequent (admin operation)
- Eventually consistent behavior is sufficient
- No distributed cache coordination needed
- Health system's sliding window provides additional damping

## API Design

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/channel/disable-rules` | List all rules |
| POST | `/api/channel/disable-rules` | Create rule |
| PUT | `/api/channel/disable-rules/:id` | Update rule |
| DELETE | `/api/channel/disable-rules/:id` | Delete rule |
| POST | `/api/channel/disable-rules/test` | Test rule matching |

### Request/Response Examples

#### Create Rule
```json
POST /api/channel/disable-rules
{
    "name": "OpenAI配额耗尽",
    "status_codes": [429],
    "keywords": ["rate_limit", "quota"],
    "match_type": "AND",
    "enabled": true,
    "description": "OpenAI返回429且包含配额相关关键词时触发故障转移"
}
```

#### Test Rules
```json
POST /api/channel/disable-rules/test
{
    "status_code": 429,
    "error_message": "Rate limit exceeded: quota"
}

Response:
{
    "success": true,
    "data": {
        "would_trigger_failover": true,
        "hardcoded_match": false,
        "user_rule_matches": [
            {
                "rule_id": 1,
                "rule_name": "OpenAI配额耗尽",
                "matched": true,
                "status_match": true,
                "keyword_match": true,
                "match_type": "AND"
            }
        ]
    }
}
```

## Integration Points

### ShouldTriggerChannelFailover Modification

```go
// service/error.go
func ShouldTriggerChannelFailover(statusCode int, errorMessage string) bool {
    // ... existing checks (HTTP status codes, network keywords) ...

    // Existing logic returns true for:
    // - 4xx (except 400)
    // - 5xx (except 504/524)
    // - Network error keywords (connection, timeout, dns, tls, ssl, network)

    // NEW: User-defined rules (after hardcoded checks)
    rules := model.GetEnabledDisableRules()
    for _, rule := range rules {
        if rule.Match(statusCode, errorMessage) {
            common.SysLog(fmt.Sprintf("故障转移规则「%s」匹配成功 (状态码=%d)",
                rule.Name, statusCode))
            return true
        }
    }

    return false
}
```

### Health Check Flow (Unchanged)

The caller already handles the result:
```go
// controller/relay.go - existing code, no changes needed
if service.ShouldTriggerChannelFailover(newAPIError.StatusCode, newAPIError.Error()) ||
    newAPIError.StatusCode == 504 || newAPIError.StatusCode == 524 {
    service.RecordChannelFailure(channel.Id, newAPIError.StatusCode, newAPIError.Error())
}
```

When `ShouldTriggerChannelFailover` returns `true` (from user rules or hardcoded), `RecordChannelFailure` is called, which:
1. Records failure to sliding window
2. Calculates failure rate
3. Triggers suspension if threshold exceeded (30% for 3 consecutive periods)
4. Applies exponential backoff (5→10→20→40→60 min)

## Validation Rules

### Rule Creation/Update
1. `name` is required, max 100 characters
2. `match_type` must be one of: AND, OR, STATUS_ONLY, KEYWORD_ONLY
3. AND match requires both `status_codes` and `keywords` non-empty
4. STATUS_ONLY requires `status_codes` non-empty
5. KEYWORD_ONLY requires `keywords` non-empty
6. `status_codes` must be valid HTTP status codes (100-599)

## Frontend Design

### Page Location
运营设置 (Operation Settings) → 渠道故障转移规则

### Components
1. **Rule List Table**: Shows all rules with enable/disable toggle
2. **Create/Edit Modal**: Form for rule configuration
3. **Test Panel**: Input status code and message to test against all rules
4. **Info Banner**: Explains that matched rules trigger temporary suspension (not permanent disable)

## Migration Strategy

1. Create database table via GORM auto-migrate
2. No data migration needed (new table)
3. Existing `ShouldTriggerChannelFailover` logic remains functional
4. Feature is opt-in (no rules = no impact)

## Performance Considerations

1. **Rule Matching**: O(n × m) where n = rules, m = keywords per rule
   - Expected n < 50 rules, m < 10 keywords
   - Sub-millisecond matching time
2. **Cache Hit Rate**: Expected > 99% (5-min TTL, infrequent changes)
3. **Memory Footprint**: < 1KB per rule, negligible overhead
4. **Health System Integration**: No additional overhead - same `RecordChannelFailure` path

## Security Considerations

1. **Admin-only Access**: All CRUD operations require admin authentication
2. **Input Validation**: Keywords and status codes validated server-side
3. **No Code Injection**: Keywords are matched via `strings.Contains`, not regex
4. **Audit Trail**: Standard GORM timestamps for created_at/updated_at

## Comparison: Health Check vs Disable

| Aspect | Health Check (This Feature) | Disable |
|--------|----------------------------|---------|
| Function | `ShouldTriggerChannelFailover` | `ShouldDisableChannel` |
| Effect | Temporary suspension | Permanent disable |
| Recovery | Auto (after suspension expires) | Manual only |
| Threshold | 30% failure rate, 3 consecutive periods | Single match |
| Backoff | Exponential (5→60 min) | N/A |
| Risk | Low (auto-recovers) | High (needs manual intervention) |

This design chooses Health Check because most upstream unavailability is temporary, and auto-recovery provides better operational experience.
