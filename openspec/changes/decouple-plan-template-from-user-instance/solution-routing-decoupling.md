# Solution: Complete Plan Template Decoupling - Routing Fields

## Problem Summary

Current implementation achieves **partial decoupling** for display fields but **routing logic remains coupled**:

- ✅ Decoupled: `PlanName`, `PlanDisplayName`, `PlanCategory`, `PlanPriority`
- ❌ Still coupled: `Type`, `ChannelGroup`, `ChannelGroups`, `RateLimitRules`, `DailyQuotaLimit`

**Impact**:
- Cannot delete Plan templates (routing dependency)
- Disabling Plan affects existing users' request routing
- Modifying Plan routing config retroactively affects all users

## Solution Design

### Phase 1: Schema Extension

Add routing snapshot fields to `UserPlan` model:

```go
// model/user_plan.go

type UserPlan struct {
    // ... existing fields ...

    // Existing snapshot fields (display/sorting)
    PlanName        string `json:"plan_name" gorm:"type:varchar(64);default:''"`
    PlanDisplayName string `json:"plan_display_name" gorm:"type:varchar(128);default:''"`
    PlanCategory    string `json:"plan_category" gorm:"type:varchar(20);default:'monthly';index:idx_user_plans_category"`
    PlanPriority    int    `json:"plan_priority" gorm:"default:0;index:idx_user_plans_priority"`

    // NEW: Routing snapshot fields (for complete decoupling)
    PlanType              string `json:"plan_type" gorm:"type:varchar(20);default:'regular'"`
    PlanChannelGroup      int    `json:"plan_channel_group" gorm:"default:0"`                     // Single channel group (legacy)
    PlanChannelGroups     string `json:"plan_channel_groups" gorm:"type:text"`                    // JSON array of groups: [1,2,3]
    PlanRateLimitRules    string `json:"plan_rate_limit_rules" gorm:"type:text"`                  // JSON serialized rules
    PlanDailyQuotaLimit   int64  `json:"plan_daily_quota_limit" gorm:"default:0"`                 // -1=unlimited, 0=no limit, >0=limit

    // Associations (for admin reference only)
    Plan *Plan `json:"plan,omitempty" gorm:"foreignKey:PlanId"`
    User *User `json:"user,omitempty" gorm:"foreignKey:UserId"`
}
```

**Field Details**:
- `PlanType`: "regular", "subscription", etc. (for type-based logic)
- `PlanChannelGroup`: Legacy single group support
- `PlanChannelGroups`: JSON array string for multi-group support
- `PlanRateLimitRules`: JSON serialized rate limit configuration
- `PlanDailyQuotaLimit`: Snapshot of daily quota limit from template

### Phase 2: Helper Methods

Add routing field accessors with fallback:

```go
// model/user_plan.go

// GetType returns the plan type from snapshot or falls back to Plan
func (up *UserPlan) GetType() string {
    if up.PlanName != "" { // Use PlanName as migration indicator
        return up.PlanType
    }
    if up.Plan != nil {
        return up.Plan.Type
    }
    return PlanTypeRegular // Default
}

// GetChannelGroup returns the channel group from snapshot or falls back to Plan
func (up *UserPlan) GetChannelGroup() int {
    if up.PlanName != "" {
        return up.PlanChannelGroup
    }
    if up.Plan != nil {
        return up.Plan.ChannelGroup
    }
    return 0
}

// GetChannelGroups returns the channel groups from snapshot or falls back to Plan
func (up *UserPlan) GetChannelGroups() []int {
    if up.PlanName != "" {
        // Parse JSON from snapshot
        if up.PlanChannelGroups != "" {
            var groups []int
            if err := json.Unmarshal([]byte(up.PlanChannelGroups), &groups); err == nil {
                return groups
            }
        }
        // Fallback to single group
        if up.PlanChannelGroup > 0 {
            return []int{up.PlanChannelGroup}
        }
        return []int{}
    }
    // Fallback to Plan
    if up.Plan != nil {
        return up.Plan.GetChannelGroupsArray()
    }
    return []int{}
}

// GetRateLimitRules returns the rate limit rules from snapshot or falls back to Plan
func (up *UserPlan) GetRateLimitRules() string {
    if up.PlanName != "" {
        return up.PlanRateLimitRules
    }
    if up.Plan != nil {
        return up.Plan.RateLimitRules
    }
    return ""
}

// GetDailyQuotaLimit returns the daily quota limit from snapshot or falls back to Plan
func (up *UserPlan) GetDailyQuotaLimit() int64 {
    if up.PlanName != "" {
        return up.PlanDailyQuotaLimit
    }
    if up.Plan != nil {
        return up.Plan.DailyQuotaLimit
    }
    return -1 // Default: unlimited
}
```

### Phase 3: Migration Function

Extend existing migration to include routing fields:

```go
// model/main.go

func migrateUserPlanSnapshots() error {
    common.SysLog("starting user plan snapshot migration...")

    var totalMigrated int
    batchSize := 100

    result := DB.Preload("Plan").
        Where("plan_name = ? OR plan_name IS NULL", "").
        FindInBatches(&[]UserPlan{}, batchSize, func(tx *gorm.DB, batch int) error {
            var userPlans []UserPlan
            tx.Find(&userPlans)

            for i := range userPlans {
                up := &userPlans[i]
                if up.Plan != nil {
                    // Copy ALL snapshot fields from Plan template
                    up.PlanName = up.Plan.Name
                    up.PlanDisplayName = up.Plan.DisplayName
                    up.PlanCategory = up.Plan.Category
                    up.PlanPriority = up.Plan.Priority

                    // NEW: Copy routing fields
                    up.PlanType = up.Plan.Type
                    up.PlanChannelGroup = up.Plan.ChannelGroup
                    up.PlanChannelGroups = up.Plan.ChannelGroups // Already JSON string
                    up.PlanRateLimitRules = up.Plan.RateLimitRules
                    up.PlanDailyQuotaLimit = up.Plan.DailyQuotaLimit

                    // Update all snapshot fields
                    if err := DB.Model(up).Select(
                        "plan_name",
                        "plan_display_name",
                        "plan_category",
                        "plan_priority",
                        "plan_type",
                        "plan_channel_group",
                        "plan_channel_groups",
                        "plan_rate_limit_rules",
                        "plan_daily_quota_limit",
                    ).Updates(up).Error; err != nil {
                        common.SysLog("failed to migrate user plan " + fmt.Sprint(up.Id) + ": " + err.Error())
                        continue
                    }
                    totalMigrated++
                }
            }

            if batch > 1 {
                common.SysLog(fmt.Sprintf("migrated batch %d (%d records so far)", batch, totalMigrated))
            }

            return nil
        })

    if result.Error != nil {
        return result.Error
    }

    if totalMigrated > 0 {
        common.SysLog(fmt.Sprintf("user plan snapshot migration completed: %d records updated", totalMigrated))
    } else {
        common.SysLog("user plan snapshot migration: no records to migrate")
    }

    return nil
}
```

### Phase 4: Update Assignment Logic

Update `AssignPlanToUser` and `AddPlanToQueue`:

```go
// model/user_plan.go - AssignPlanToUser

userPlan := &UserPlan{
    // ... existing fields ...

    // Display snapshots
    PlanName:        plan.Name,
    PlanDisplayName: plan.DisplayName,
    PlanCategory:    plan.Category,
    PlanPriority:    plan.Priority,

    // NEW: Routing snapshots
    PlanType:            plan.Type,
    PlanChannelGroup:    plan.ChannelGroup,
    PlanChannelGroups:   plan.ChannelGroups,
    PlanRateLimitRules:  plan.RateLimitRules,
    PlanDailyQuotaLimit: plan.DailyQuotaLimit,
}

// Same changes for AddPlanToQueue()
```

### Phase 5: Update Routing Logic

#### 5.1 Update SelectPlanForRequest

```go
// service/plan_selector.go

func SelectPlanForRequest(userId int, modelName string) (*PlanSelection, error) {
    // ... existing logic to get userPlans ...

    for _, userPlan := range userPlans {
        // OLD: if userPlan.Plan == nil { continue }
        // NEW: Check snapshot fields exist (indicates migrated record)

        // Use snapshot methods instead of Plan fields
        channelGroups := userPlan.GetChannelGroups()
        planType := userPlan.GetType()

        // Continue with routing logic using snapshot data
        // ...
    }
}
```

#### 5.2 Update HasChannelGroupAccess

```go
// service/plan_selector.go

func HasChannelGroupAccess(userPlan *model.UserPlan, channelGroupId int) bool {
    if userPlan == nil {
        return false
    }

    // Use snapshot instead of Plan
    channelGroups := userPlan.GetChannelGroups()

    // Check if channelGroupId is in allowed groups
    for _, groupId := range channelGroups {
        if groupId == channelGroupId {
            return true
        }
    }
    return false
}
```

#### 5.3 Update Rate Limiting Logic

```go
// middleware/rate_limit.go (or wherever rate limiting is implemented)

func GetRateLimitForPlan(userPlan *model.UserPlan) *RateLimitConfig {
    if userPlan == nil {
        return nil
    }

    // Use snapshot instead of Plan
    rulesJSON := userPlan.GetRateLimitRules()
    if rulesJSON == "" {
        return nil
    }

    var config RateLimitConfig
    if err := json.Unmarshal([]byte(rulesJSON), &config); err != nil {
        return nil
    }

    return &config
}
```

### Phase 6: Update Cache Serialization

```go
// model/user_plan_cache.go

func FromUserPlan(up *UserPlan) *UserPlanCacheEntry {
    entry := &UserPlanCacheEntry{
        // ... existing fields ...
    }

    // Use snapshot fields (with fallback for unmigrated records)
    if up.PlanName != "" {
        // Migrated record - use ALL snapshots
        entry.PlanName = up.PlanName
        entry.PlanPriority = up.PlanPriority
        entry.PlanType = up.PlanType
        entry.PlanChannelGroup = up.PlanChannelGroup
        entry.PlanChannelGroups = up.PlanChannelGroups
        entry.PlanDailyQuotaLimit = up.PlanDailyQuotaLimit
        entry.PlanRateLimitRules = up.PlanRateLimitRules
    } else if up.Plan != nil {
        // Unmigrated record - fallback to Plan
        entry.PlanName = up.Plan.Name
        entry.PlanPriority = up.Plan.Priority
        entry.PlanType = up.Plan.Type
        entry.PlanChannelGroup = up.Plan.ChannelGroup
        entry.PlanChannelGroups = up.Plan.ChannelGroups
        entry.PlanDailyQuotaLimit = up.Plan.DailyQuotaLimit
        entry.PlanRateLimitRules = up.Plan.RateLimitRules
    }

    // Status still read from Plan (or snapshot if we add it)
    if up.Plan != nil {
        entry.PlanStatus = up.Plan.Status
    }

    return entry
}
```

### Phase 7: Update Plan.Delete()

Now we can safely allow deletion:

```go
// model/plan.go

func (p *Plan) Delete() error {
    if p.Id == 0 {
        return errors.New("套餐ID不能为空")
    }

    // Check for unmigrated instances only
    var count int64
    if err := DB.Model(&UserPlan{}).
        Where("plan_id = ? AND (plan_name = ? OR plan_name IS NULL)", p.Id, "").
        Count(&count).Error; err != nil {
        return err
    }

    if count > 0 {
        return errors.New("该套餐模板仍有未迁移的用户实例，请等待迁移完成")
    }

    // Safe to delete - all instances have complete snapshots
    go InvalidateUserPlanCacheByPlanId(p.Id)
    return DB.Delete(p).Error
}
```

### Phase 8: Handle Plan.Status

**Option A**: Ignore Plan.Status completely (recommended)
- Once migrated, UserPlan operates independently
- Plan.Status only affects NEW assignments
- Existing users unaffected by template status changes

**Option B**: Snapshot Plan.Status
- Add `PlanStatus int` to UserPlan
- But this defeats the purpose - status should affect future assignments only

**Recommendation**: Use Option A. Document that:
- Plan.Status = disabled → prevents NEW purchases/assignments
- Plan.Status = disabled → does NOT affect existing UserPlan instances

## Implementation Steps

### Step 1: Schema Update
```bash
# model/user_plan.go - add new fields
```

### Step 2: Migration Update
```bash
# model/main.go - extend migrateUserPlanSnapshots()
```

### Step 3: Assignment Update
```bash
# model/user_plan.go - update AssignPlanToUser() and AddPlanToQueue()
```

### Step 4: Routing Logic Update
```bash
# service/plan_selector.go - update SelectPlanForRequest()
# service/plan_selector.go - update HasChannelGroupAccess()
# middleware/rate_limit.go - update rate limiting logic
```

### Step 5: Cache Update
```bash
# model/user_plan_cache.go - update FromUserPlan()
```

### Step 6: Template Management
```bash
# model/plan.go - update Delete() to allow deletion of migrated instances
```

### Step 7: Testing
- Test new assignments → snapshots populated
- Test routing with Plan disabled → existing users unaffected
- Test migration → all fields copied correctly
- Test Plan deletion → only allows deletion after migration
- Test cache → snapshot fields preserved

## Migration Strategy

### For Existing Deployments

1. **Deploy schema changes** (adds new columns, defaults to empty)
2. **Migration runs on startup** (idempotent, fills snapshots)
3. **Routing logic uses fallback** (supports unmigrated records during transition)
4. **After migration completes** (all snapshots populated):
   - Plan templates can be freely modified without affecting users
   - Admins can delete unused Plan templates
   - Disabling templates only affects NEW assignments

### Rollback Safety

If issues occur:
1. Schema changes are additive (no data loss)
2. Routing logic has fallback to Plan (backward compatible)
3. Can revert routing logic changes without touching schema
4. Snapshots remain in database (no harm if unused)

## Benefits After Implementation

1. ✅ **Complete independence**: UserPlan fully decoupled from Plan template
2. ✅ **Safe deletions**: Can delete Plan templates after migration
3. ✅ **Isolated changes**: Template modifications don't affect existing users
4. ✅ **Per-user customization**: Admins can customize routing per user
5. ✅ **Better performance**: No JOINs needed for any queries
6. ✅ **Template cleanup**: Old/unused templates can be archived

## Considerations

### Storage Impact
- Additional ~200 bytes per UserPlan record
- For 10K users with 3 plans: ~6MB
- **Acceptable** for gained independence

### Cache Impact
- Larger cache entries (includes routing fields)
- Better cache hit rate (no Plan dependency)
- Net positive impact

### Admin UI
- Add indicators showing which templates can be deleted
- Show migration status per UserPlan
- Allow per-user routing customization

## Testing Checklist

- [ ] New UserPlan has all snapshots populated
- [ ] Routing works with Plan = nil (deleted template)
- [ ] Rate limiting works with snapshot rules
- [ ] Channel access check works with snapshot groups
- [ ] Cache serialization uses snapshots
- [ ] Migration handles large datasets (batch processing)
- [ ] Rollback scenario works (fallback to Plan)
- [ ] Plan.Delete() allows deletion after migration
- [ ] Plan.Delete() prevents deletion before migration
- [ ] Disabling Plan doesn't affect existing users
