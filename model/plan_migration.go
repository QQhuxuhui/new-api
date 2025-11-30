package model

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	// LegacyPlanName is the name for the default plan created for existing users
	LegacyPlanName = "legacy"
	// LegacyPlanDisplayName is the display name for the legacy plan
	LegacyPlanDisplayName = "遗留套餐"
)

// MigrationResult holds the result of a migration operation
type MigrationResult struct {
	TotalUsers      int
	MigratedUsers   int
	SkippedUsers    int
	FailedUsers     int
	PlanCreated     bool
	PlanId          int
	Errors          []string
}

// CreateLegacyPlan creates or retrieves the legacy plan for existing users
func CreateLegacyPlan() (*Plan, error) {
	// Check if legacy plan already exists
	existingPlan, err := GetPlanByName(LegacyPlanName)
	if err == nil && existingPlan != nil {
		common.SysLog(fmt.Sprintf("Legacy plan already exists with ID: %d", existingPlan.Id))
		return existingPlan, nil
	}

	// Create a new legacy plan
	plan := &Plan{
		Name:               LegacyPlanName,
		DisplayName:        LegacyPlanDisplayName,
		Description:        "为现有用户自动创建的遗留套餐，保留原有额度和使用习惯",
		Type:               PlanTypeConsumption, // Consumption type for pay-as-you-go
		Priority:           0,                   // Lowest priority
		ChannelGroup:       "",                  // Use default channel group
		DefaultQuota:       0,                   // No default quota, will use user's existing quota
		ValidityDays:       0,                   // Never expires
		DefaultAllowSwitch: 1,                   // Allow users to switch
		DefaultAllowToggle: 1,                   // Allow users to toggle auto-switch
		Status:             PlanStatusEnabled,
	}

	if err := plan.Insert(); err != nil {
		return nil, fmt.Errorf("failed to create legacy plan: %v", err)
	}

	common.SysLog(fmt.Sprintf("Created legacy plan with ID: %d", plan.Id))
	return plan, nil
}

// MigrateExistingUsers migrates all existing users to the plan system
// It creates a user_plan for each user with their current quota
func MigrateExistingUsers(dryRun bool) (*MigrationResult, error) {
	result := &MigrationResult{
		Errors: make([]string, 0),
	}

	// Step 1: Create or get the legacy plan
	plan, err := CreateLegacyPlan()
	if err != nil {
		return result, err
	}
	result.PlanCreated = true
	result.PlanId = plan.Id

	// Step 2: Get all users
	var users []User
	if err := DB.Find(&users).Error; err != nil {
		return result, fmt.Errorf("failed to fetch users: %v", err)
	}
	result.TotalUsers = len(users)

	if dryRun {
		common.SysLog(fmt.Sprintf("[DRY RUN] Would migrate %d users to legacy plan", result.TotalUsers))
		return result, nil
	}

	// Step 3: Migrate each user
	now := time.Now().UnixMilli()
	for _, user := range users {
		// Check if user already has a user_plan
		var existingCount int64
		if err := DB.Model(&UserPlan{}).Where("user_id = ?", user.Id).Count(&existingCount).Error; err != nil {
			result.FailedUsers++
			result.Errors = append(result.Errors, fmt.Sprintf("failed to check existing plans for user %d: %v", user.Id, err))
			continue
		}

		if existingCount > 0 {
			// User already has plans, skip
			result.SkippedUsers++
			continue
		}

		// Create user_plan with user's current quota
		userPlan := &UserPlan{
			UserId:          user.Id,
			PlanId:          plan.Id,
			Quota:           int64(user.Quota),     // Transfer user's current quota
			UsedQuota:       int64(user.UsedQuota), // Transfer used quota for historical tracking
			IsCurrent:       1,                      // Set as current plan
			AutoSwitch:      1,                      // Enable auto-switch
			AllowUserSwitch: plan.DefaultAllowSwitch,
			AllowUserToggle: plan.DefaultAllowToggle,
			Locked:          0,
			StartedAt:       now,
			ExpiresAt:       0, // Never expires
			Status:          UserPlanStatusActive,
		}

		if err := DB.Create(userPlan).Error; err != nil {
			result.FailedUsers++
			result.Errors = append(result.Errors, fmt.Sprintf("failed to create user_plan for user %d: %v", user.Id, err))
			continue
		}

		result.MigratedUsers++
	}

	common.SysLog(fmt.Sprintf("Migration completed: %d total, %d migrated, %d skipped, %d failed",
		result.TotalUsers, result.MigratedUsers, result.SkippedUsers, result.FailedUsers))

	return result, nil
}

// MigrateSingleUser migrates a single user to the plan system
// This is useful for migrating users individually or for new users in hybrid mode
func MigrateSingleUser(userId int) error {
	// Check if user already has a user_plan
	var existingCount int64
	if err := DB.Model(&UserPlan{}).Where("user_id = ?", userId).Count(&existingCount).Error; err != nil {
		return fmt.Errorf("failed to check existing plans: %v", err)
	}
	if existingCount > 0 {
		return nil // User already has plans
	}

	// Get the user
	var user User
	if err := DB.First(&user, userId).Error; err != nil {
		return fmt.Errorf("failed to get user: %v", err)
	}

	// Get or create the legacy plan
	plan, err := CreateLegacyPlan()
	if err != nil {
		return err
	}

	// Create user_plan
	now := time.Now().UnixMilli()
	userPlan := &UserPlan{
		UserId:          user.Id,
		PlanId:          plan.Id,
		Quota:           int64(user.Quota),
		UsedQuota:       int64(user.UsedQuota),
		IsCurrent:       1,
		AutoSwitch:      1,
		AllowUserSwitch: plan.DefaultAllowSwitch,
		AllowUserToggle: plan.DefaultAllowToggle,
		Locked:          0,
		StartedAt:       now,
		ExpiresAt:       0,
		Status:          UserPlanStatusActive,
	}

	return DB.Create(userPlan).Error
}

// RollbackMigration removes all user_plans associated with the legacy plan
// WARNING: This will delete user plan data. Use with caution.
func RollbackMigration(dryRun bool) (*MigrationResult, error) {
	result := &MigrationResult{
		Errors: make([]string, 0),
	}

	// Get the legacy plan
	plan, err := GetPlanByName(LegacyPlanName)
	if err != nil {
		return result, fmt.Errorf("legacy plan not found: %v", err)
	}
	result.PlanId = plan.Id

	// Count user_plans to be deleted
	var count int64
	if err := DB.Model(&UserPlan{}).Where("plan_id = ?", plan.Id).Count(&count).Error; err != nil {
		return result, fmt.Errorf("failed to count user_plans: %v", err)
	}
	result.TotalUsers = int(count)

	if dryRun {
		common.SysLog(fmt.Sprintf("[DRY RUN] Would delete %d user_plans and the legacy plan", count))
		return result, nil
	}

	// Delete all user_plans for the legacy plan
	if err := DB.Where("plan_id = ?", plan.Id).Delete(&UserPlan{}).Error; err != nil {
		return result, fmt.Errorf("failed to delete user_plans: %v", err)
	}
	result.MigratedUsers = int(count)

	// Delete the legacy plan
	if err := DB.Delete(plan).Error; err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to delete legacy plan: %v", err))
	} else {
		result.PlanCreated = false
	}

	common.SysLog(fmt.Sprintf("Rollback completed: deleted %d user_plans", count))
	return result, nil
}

// GetMigrationStatus returns the current migration status
func GetMigrationStatus() map[string]interface{} {
	status := make(map[string]interface{})

	// Check if legacy plan exists
	plan, err := GetPlanByName(LegacyPlanName)
	if err != nil {
		status["legacy_plan_exists"] = false
		status["legacy_plan_id"] = 0
	} else {
		status["legacy_plan_exists"] = true
		status["legacy_plan_id"] = plan.Id
	}

	// Count total users
	var totalUsers int64
	DB.Model(&User{}).Count(&totalUsers)
	status["total_users"] = totalUsers

	// Count users with user_plans
	var usersWithPlans int64
	DB.Model(&UserPlan{}).Select("DISTINCT user_id").Count(&usersWithPlans)
	status["users_with_plans"] = usersWithPlans

	// Count users without user_plans
	status["users_without_plans"] = totalUsers - usersWithPlans

	// Plan system enabled status
	status["plan_system_enabled"] = common.PlanSystemEnabled

	return status
}
