# Design: Decouple Plan Template from User Instance

## Architecture Decision

### Current Architecture (Problematic)

```
UserPlan (instance) ──[depends on]──> Plan (template)
      │                                    │
      ├─ Queries JOIN plans                │
      ├─ Filters by plans.status ─────────┤
      └─ Displays plan.DisplayName ───────┘

Problem: Disabling/deleting Plan breaks UserPlan
```

### Target Architecture (Decoupled)

```
UserPlan (snapshot) ──[references]──> Plan (template)
      │                                    │
      ├─ PlanName (snapshot)              │
      ├─ PlanDisplayName (snapshot)        │
      ├─ PlanCategory (snapshot)           │
      ├─ PlanPriority (snapshot)           │
      └─ PlanId (optional reference) ─────┘ (for admin view only)

Benefit: UserPlan is fully independent after creation
```

## Schema Changes

### UserPlan Table Additions

```sql
ALTER TABLE user_plans ADD COLUMN plan_name VARCHAR(64) DEFAULT '';
ALTER TABLE user_plans ADD COLUMN plan_display_name VARCHAR(128) DEFAULT '';
ALTER TABLE user_plans ADD COLUMN plan_category VARCHAR(20) DEFAULT 'monthly';
ALTER TABLE user_plans ADD COLUMN plan_priority INT DEFAULT 0;

-- Create indexes for common queries
CREATE INDEX idx_user_plans_priority ON user_plans(plan_priority DESC, id ASC);
CREATE INDEX idx_user_plans_category ON user_plans(plan_category);
```

### GORM Model Changes

```go
type UserPlan struct {
    // ... existing fields ...

    // Snapshot fields from Plan template (immutable after assignment)
    PlanName        string `json:"plan_name" gorm:"type:varchar(64);default:''"`
    PlanDisplayName string `json:"plan_display_name" gorm:"type:varchar(128);default:''"`
    PlanCategory    string `json:"plan_category" gorm:"type:varchar(20);default:'monthly'"`
    PlanPriority    int    `json:"plan_priority" gorm:"default:0;index:idx_priority"`

    // Association for admin reference only (not used in user queries)
    Plan *Plan `json:"plan,omitempty" gorm:"foreignKey:PlanId"`
}
```

## Query Pattern Changes

### Before (Dependent on Plan)

```go
// GetUserCurrentPlan - BREAKS if plan is disabled
func GetUserCurrentPlan(userId int) (*UserPlan, error) {
    var userPlan UserPlan
    err := DB.Preload("Plan").
        Joins("JOIN plans ON plans.id = user_plans.plan_id").
        Where("user_plans.user_id = ? AND user_plans.is_current = 1 AND user_plans.status = ? AND plans.status = ?",
            userId, UserPlanStatusActive, PlanStatusEnabled). // ❌ Breaks here
        First(&userPlan).Error
    return &userPlan, err
}
```

### After (Independent)

```go
// GetUserCurrentPlan - Works regardless of plan status
func GetUserCurrentPlan(userId int) (*UserPlan, error) {
    var userPlan UserPlan
    err := DB.Preload("Plan"). // Still preload for admin view, but query doesn't depend on it
        Where("user_id = ? AND is_current = 1 AND status = ?",
            userId, UserPlanStatusActive). // ✅ Only checks UserPlan status
        First(&userPlan).Error
    return &userPlan, err
}
```

## Migration Strategy

### Phase 1: Automatic Migration on Startup

```go
func migrateUserPlanSnapshots() error {
    // Only migrate records with empty snapshot fields
    var userPlans []UserPlan
    err := DB.Preload("Plan").
        Where("plan_name = '' OR plan_name IS NULL").
        FindInBatches(&userPlans, 100).Error

    for _, up := range userPlans {
        if up.Plan != nil {
            up.PlanName = up.Plan.Name
            up.PlanDisplayName = up.Plan.DisplayName
            up.PlanCategory = up.Plan.Category
            up.PlanPriority = up.Plan.Priority
            DB.Model(&up).Select("plan_name", "plan_display_name", "plan_category", "plan_priority").Updates(&up)
        }
    }

    return nil
}
```

### Phase 2: Update Assignment Logic

All plan assignment functions must save snapshots:
- `AssignPlanToUser()`
- `AddPlanToQueue()`
- Admin assign operations

```go
func AddPlanToQueue(userId int, planId int, ...) (*UserPlan, error) {
    plan, err := GetPlanById(planId)
    // ...

    userPlan := &UserPlan{
        // ... existing fields ...

        // Snapshot template fields
        PlanName:        plan.Name,
        PlanDisplayName: plan.DisplayName,
        PlanCategory:    plan.Category,
        PlanPriority:    plan.Priority,
    }

    return userPlan, DB.Create(userPlan).Error
}
```

### Phase 3: Remove Hard Dependencies

Update these queries to NOT check `plans.status`:
- `GetUserValidPlans()` - line 283-288
- `GetUserCurrentPlan()` - line 299-307
- `SwitchUserCurrentPlan()` - line 332-340

Update sort order to use snapshot field:
- Change `Order("plans.priority DESC")` → `Order("plan_priority DESC, id ASC")`

## Backward Compatibility

### Handling NULL Snapshot Fields

```go
func (up *UserPlan) GetDisplayName() string {
    if up.PlanDisplayName != "" {
        return up.PlanDisplayName
    }
    // Fallback for unmigrated records
    if up.Plan != nil {
        return up.Plan.DisplayName
    }
    return "Unknown Plan"
}
```

### Template Deletion Rules

```go
func (p *Plan) Delete() error {
    // Check for unmigrated instances (snapshot fields empty)
    var count int64
    err := DB.Model(&UserPlan{}).
        Where("plan_id = ? AND (plan_name = '' OR plan_name IS NULL)", p.Id).
        Count(&count).Error

    if count > 0 {
        return errors.New("模板仍被使用，请等待数据迁移完成")
    }

    // Safe to delete - all instances have snapshots
    return DB.Delete(p).Error
}
```

## Display Logic Updates

### Frontend Data Binding

**Before**: `userPlan.plan.display_name`
**After**: `userPlan.plan_display_name` (with fallback)

```javascript
const getPlanDisplayName = (userPlan) => {
  return userPlan.plan_display_name || userPlan.plan?.display_name || 'Unknown';
};
```

### Backend Response

```go
type UserPlanResponse struct {
    Id              int    `json:"id"`
    PlanDisplayName string `json:"plan_display_name"` // Always from snapshot
    PlanCategory    string `json:"plan_category"`     // Always from snapshot
    // ... other fields

    Plan *Plan `json:"plan,omitempty"` // For admin view only
}
```

## Rollback Plan

If issues arise:

1. **Queries fail**: The old queries with JOIN still work, just add them back
2. **Migration incomplete**: System continues using Plan association as fallback
3. **Display broken**: Frontend can always fallback to `plan.display_name`

No data loss occurs - snapshot fields are additive only.

## Performance Considerations

### Query Performance

**Before**: JOIN adds overhead
```sql
SELECT * FROM user_plans
JOIN plans ON plans.id = user_plans.plan_id
WHERE user_plans.user_id = ? AND plans.status = 1
```

**After**: Simpler, faster query
```sql
SELECT * FROM user_plans
WHERE user_id = ? AND status = 1
```

**Impact**: ~20-30% faster for queries with Plan JOIN

### Storage Impact

- 4 new columns per UserPlan row
- Estimated: ~100 bytes per record
- For 10K users with 3 plans each: ~3MB additional storage
- **Acceptable** for gained independence

## Testing Strategy

### Unit Tests

- Test snapshot saving on assignment
- Test queries with Plan disabled/deleted
- Test migration with NULL fields
- Test fallback logic for unmigrated records

### Integration Tests

- Assign plan → disable template → verify user can still use
- Assign plan → delete template → verify display works
- Migrate old data → verify snapshot populated
- Query user plans → verify no JOIN needed

### Manual Testing Checklist

- [ ] Create user plan with template
- [ ] Disable Plan template
- [ ] Verify user can still access plan
- [ ] Delete Plan template (after migration)
- [ ] Verify user plan still displays correctly
- [ ] Check admin view shows template info
- [ ] Verify queue ordering uses snapshot priority

## Future Enhancements

After this change, we can:

1. **Archive old plans**: Physically delete unused Plan templates
2. **Per-user customization**: Admins can modify individual UserPlan display names
3. **Template evolution**: Change Plan templates without affecting existing users
4. **Audit improvements**: Track template changes vs user instance changes separately
