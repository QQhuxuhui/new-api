package service

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

func setupPlanAnalyticsTestDB(t *testing.T) {
	t.Helper()

	db := setupTestDB(t)
	model.LOG_DB = db

	if err := db.AutoMigrate(&model.Log{}); err != nil {
		t.Fatalf("auto migrate log: %v", err)
	}
}

func beijingTime(year int, month time.Month, day, hour, min, sec int) time.Time {
	return time.Date(year, month, day, hour, min, sec, 0, time.FixedZone("CST", 8*3600))
}

func TestGetPlanUsageList_RespectsCustomRangeEndTime(t *testing.T) {
	setupPlanAnalyticsTestDB(t)

	user := &model.User{
		Username: "plan-list-user",
		Password: "12345678",
		Status:   1,
	}
	if err := model.DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name:        "plan-list",
		DisplayName: "Plan List",
	}
	if err := model.DB.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}

	planID := plan.Id
	userPlan := &model.UserPlan{
		UserId:          user.Id,
		PlanId:          &planID,
		Quota:           1000,
		UsedQuota:       200,
		Status:          model.UserPlanStatusActive,
		StartedAt:       beijingTime(2026, time.March, 1, 0, 0, 0).UnixMilli(),
		ExpiresAt:       beijingTime(2026, time.March, 31, 23, 59, 59).UnixMilli(),
		PlanName:        plan.Name,
		PlanDisplayName: plan.DisplayName,
	}
	if err := model.DB.Create(userPlan).Error; err != nil {
		t.Fatalf("create user plan: %v", err)
	}

	inRangeLog := &model.Log{
		UserId:     user.Id,
		UserPlanId: userPlan.Id,
		CreatedAt:  beijingTime(2026, time.March, 3, 12, 0, 0).Unix(),
		Type:       model.LogTypeConsume,
		Quota:      100,
	}
	outOfRangeLog := &model.Log{
		UserId:     user.Id,
		UserPlanId: userPlan.Id,
		CreatedAt:  beijingTime(2026, time.March, 8, 12, 0, 0).Unix(),
		Type:       model.LogTypeConsume,
		Quota:      100,
	}
	if err := model.LOG_DB.Create(inRangeLog).Error; err != nil {
		t.Fatalf("create in-range log: %v", err)
	}
	if err := model.LOG_DB.Create(outOfRangeLog).Error; err != nil {
		t.Fatalf("create out-of-range log: %v", err)
	}

	timeRange := fmt.Sprintf(
		"custom:%d:%d",
		beijingTime(2026, time.March, 1, 0, 0, 0).Unix(),
		beijingTime(2026, time.March, 7, 23, 59, 59).Unix(),
	)

	resp, err := GetPlanUsageList(&dto.PlanUsageFilters{
		TimeRange: timeRange,
		Page:      1,
		PageSize:  25,
	})
	if err != nil {
		t.Fatalf("get plan usage list: %v", err)
	}

	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}

	if resp.Items[0].RequestCount != 1 {
		t.Fatalf("expected request count 1 within custom range, got %d", resp.Items[0].RequestCount)
	}
}

func TestGetPlanConsumptionRanking_UsesHistoricalWindow(t *testing.T) {
	setupPlanAnalyticsTestDB(t)

	user := &model.User{
		Username: "plan-ranking-user",
		Password: "12345678",
		Status:   1,
	}
	if err := model.DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name:        "historical-plan",
		DisplayName: "Historical Plan",
	}
	if err := model.DB.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}

	planID := plan.Id
	userPlan := &model.UserPlan{
		UserId:          user.Id,
		PlanId:          &planID,
		Quota:           1000,
		UsedQuota:       500000,
		Status:          model.UserPlanStatusExpired,
		StartedAt:       beijingTime(2026, time.February, 20, 0, 0, 0).UnixMilli(),
		ExpiresAt:       beijingTime(2026, time.March, 31, 23, 59, 59).UnixMilli(),
		PlanName:        plan.Name,
		PlanDisplayName: plan.DisplayName,
	}
	if err := model.DB.Create(userPlan).Error; err != nil {
		t.Fatalf("create user plan: %v", err)
	}

	inRangeLog := &model.Log{
		UserId:     user.Id,
		UserPlanId: userPlan.Id,
		CreatedAt:  beijingTime(2026, time.March, 3, 12, 0, 0).Unix(),
		Type:       model.LogTypeConsume,
		Quota:      500000,
	}
	outOfRangeLog := &model.Log{
		UserId:     user.Id,
		UserPlanId: userPlan.Id,
		CreatedAt:  beijingTime(2026, time.March, 9, 12, 0, 0).Unix(),
		Type:       model.LogTypeConsume,
		Quota:      1500000,
	}
	if err := model.LOG_DB.Create(inRangeLog).Error; err != nil {
		t.Fatalf("create in-range log: %v", err)
	}
	if err := model.LOG_DB.Create(outOfRangeLog).Error; err != nil {
		t.Fatalf("create out-of-range log: %v", err)
	}

	timeRange := fmt.Sprintf(
		"custom:%d:%d",
		beijingTime(2026, time.March, 1, 0, 0, 0).Unix(),
		beijingTime(2026, time.March, 7, 23, 59, 59).Unix(),
	)

	ranking, err := GetPlanConsumptionRanking(10, timeRange)
	if err != nil {
		t.Fatalf("get plan consumption ranking: %v", err)
	}

	if len(ranking) != 1 {
		t.Fatalf("expected 1 ranked plan, got %d", len(ranking))
	}

	if ranking[0].PlanId != plan.Id {
		t.Fatalf("expected historical plan id %d, got %d", plan.Id, ranking[0].PlanId)
	}

	if ranking[0].RequestCount != 1 {
		t.Fatalf("expected request count 1 within custom range, got %d", ranking[0].RequestCount)
	}

	if math.Abs(ranking[0].TotalConsumedUSD-1.0) > 1e-9 {
		t.Fatalf("expected total consumed USD 1.0, got %f", ranking[0].TotalConsumedUSD)
	}
}

func TestGetPlanUsageList_CountsConsumeLogsForMatchingPlanOnly(t *testing.T) {
	setupPlanAnalyticsTestDB(t)

	user := &model.User{
		Username: "plan-list-scope-user",
		Password: "12345678",
		Status:   1,
	}
	if err := model.DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	planA := &model.Plan{
		Name:        "plan-a",
		DisplayName: "Plan A",
	}
	planB := &model.Plan{
		Name:        "plan-b",
		DisplayName: "Plan B",
	}
	if err := model.DB.Create(planA).Error; err != nil {
		t.Fatalf("create plan A: %v", err)
	}
	if err := model.DB.Create(planB).Error; err != nil {
		t.Fatalf("create plan B: %v", err)
	}

	planAID := planA.Id
	planBID := planB.Id
	userPlanA := &model.UserPlan{
		UserId:          user.Id,
		PlanId:          &planAID,
		Quota:           1000,
		UsedQuota:       200,
		Status:          model.UserPlanStatusActive,
		StartedAt:       beijingTime(2026, time.March, 1, 0, 0, 0).UnixMilli(),
		ExpiresAt:       beijingTime(2026, time.March, 31, 23, 59, 59).UnixMilli(),
		PlanName:        planA.Name,
		PlanDisplayName: planA.DisplayName,
	}
	userPlanB := &model.UserPlan{
		UserId:          user.Id,
		PlanId:          &planBID,
		Quota:           1000,
		UsedQuota:       100,
		Status:          model.UserPlanStatusActive,
		StartedAt:       beijingTime(2026, time.March, 1, 0, 0, 0).UnixMilli(),
		ExpiresAt:       beijingTime(2026, time.March, 31, 23, 59, 59).UnixMilli(),
		PlanName:        planB.Name,
		PlanDisplayName: planB.DisplayName,
	}
	if err := model.DB.Create(userPlanA).Error; err != nil {
		t.Fatalf("create user plan A: %v", err)
	}
	if err := model.DB.Create(userPlanB).Error; err != nil {
		t.Fatalf("create user plan B: %v", err)
	}

	logs := []*model.Log{
		{
			UserId:     user.Id,
			UserPlanId: userPlanA.Id,
			CreatedAt:  beijingTime(2026, time.March, 3, 12, 0, 0).Unix(),
			Type:       model.LogTypeConsume,
			Quota:      100,
		},
		{
			UserId:     user.Id,
			UserPlanId: userPlanB.Id,
			CreatedAt:  beijingTime(2026, time.March, 3, 13, 0, 0).Unix(),
			Type:       model.LogTypeConsume,
			Quota:      200,
		},
		{
			UserId:     user.Id,
			UserPlanId: userPlanA.Id,
			CreatedAt:  beijingTime(2026, time.March, 3, 14, 0, 0).Unix(),
			Type:       model.LogTypeTopup,
			Quota:      300,
		},
		{
			UserId:     user.Id,
			UserPlanId: userPlanA.Id,
			CreatedAt:  beijingTime(2026, time.March, 9, 12, 0, 0).Unix(),
			Type:       model.LogTypeConsume,
			Quota:      400,
		},
	}
	for _, log := range logs {
		if err := model.LOG_DB.Create(log).Error; err != nil {
			t.Fatalf("create log: %v", err)
		}
	}

	timeRange := fmt.Sprintf(
		"custom:%d:%d",
		beijingTime(2026, time.March, 1, 0, 0, 0).Unix(),
		beijingTime(2026, time.March, 7, 23, 59, 59).Unix(),
	)

	resp, err := GetPlanUsageList(&dto.PlanUsageFilters{
		UserId:    user.Id,
		TimeRange: timeRange,
		Page:      1,
		PageSize:  25,
	})
	if err != nil {
		t.Fatalf("get plan usage list: %v", err)
	}

	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}

	requestCounts := make(map[int]int, len(resp.Items))
	for _, item := range resp.Items {
		requestCounts[item.UserPlanId] = item.RequestCount
	}

	if requestCounts[userPlanA.Id] != 1 {
		t.Fatalf("expected plan A request count 1, got %d", requestCounts[userPlanA.Id])
	}
	if requestCounts[userPlanB.Id] != 1 {
		t.Fatalf("expected plan B request count 1, got %d", requestCounts[userPlanB.Id])
	}
}

func TestGetPlanConsumptionRanking_UsesConsumeLogsByUserPlanID(t *testing.T) {
	setupPlanAnalyticsTestDB(t)

	user := &model.User{
		Username: "plan-ranking-scope-user",
		Password: "12345678",
		Status:   1,
	}
	if err := model.DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	planA := &model.Plan{
		Name:        "ranking-plan-a",
		DisplayName: "Ranking Plan A",
	}
	planB := &model.Plan{
		Name:        "ranking-plan-b",
		DisplayName: "Ranking Plan B",
	}
	if err := model.DB.Create(planA).Error; err != nil {
		t.Fatalf("create plan A: %v", err)
	}
	if err := model.DB.Create(planB).Error; err != nil {
		t.Fatalf("create plan B: %v", err)
	}

	planAID := planA.Id
	planBID := planB.Id
	userPlanA := &model.UserPlan{
		UserId:          user.Id,
		PlanId:          &planAID,
		Quota:           1000,
		UsedQuota:       500000,
		Status:          model.UserPlanStatusActive,
		StartedAt:       beijingTime(2026, time.March, 1, 0, 0, 0).UnixMilli(),
		ExpiresAt:       beijingTime(2026, time.March, 31, 23, 59, 59).UnixMilli(),
		PlanName:        planA.Name,
		PlanDisplayName: planA.DisplayName,
	}
	userPlanB := &model.UserPlan{
		UserId:          user.Id,
		PlanId:          &planBID,
		Quota:           1000,
		UsedQuota:       1500000,
		Status:          model.UserPlanStatusActive,
		StartedAt:       beijingTime(2026, time.March, 1, 0, 0, 0).UnixMilli(),
		ExpiresAt:       beijingTime(2026, time.March, 31, 23, 59, 59).UnixMilli(),
		PlanName:        planB.Name,
		PlanDisplayName: planB.DisplayName,
	}
	if err := model.DB.Create(userPlanA).Error; err != nil {
		t.Fatalf("create user plan A: %v", err)
	}
	if err := model.DB.Create(userPlanB).Error; err != nil {
		t.Fatalf("create user plan B: %v", err)
	}

	logs := []*model.Log{
		{
			UserId:     user.Id,
			UserPlanId: userPlanA.Id,
			CreatedAt:  beijingTime(2026, time.March, 3, 12, 0, 0).Unix(),
			Type:       model.LogTypeConsume,
			Quota:      500000,
		},
		{
			UserId:     user.Id,
			UserPlanId: userPlanB.Id,
			CreatedAt:  beijingTime(2026, time.March, 3, 13, 0, 0).Unix(),
			Type:       model.LogTypeConsume,
			Quota:      1500000,
		},
		{
			UserId:     user.Id,
			UserPlanId: userPlanA.Id,
			CreatedAt:  beijingTime(2026, time.March, 3, 14, 0, 0).Unix(),
			Type:       model.LogTypeSystem,
			Quota:      2500000,
		},
		{
			UserId:     user.Id,
			UserPlanId: userPlanB.Id,
			CreatedAt:  beijingTime(2026, time.March, 9, 12, 0, 0).Unix(),
			Type:       model.LogTypeConsume,
			Quota:      500000,
		},
	}
	for _, log := range logs {
		if err := model.LOG_DB.Create(log).Error; err != nil {
			t.Fatalf("create log: %v", err)
		}
	}

	timeRange := fmt.Sprintf(
		"custom:%d:%d",
		beijingTime(2026, time.March, 1, 0, 0, 0).Unix(),
		beijingTime(2026, time.March, 7, 23, 59, 59).Unix(),
	)

	ranking, err := GetPlanConsumptionRanking(10, timeRange)
	if err != nil {
		t.Fatalf("get plan consumption ranking: %v", err)
	}

	if len(ranking) != 2 {
		t.Fatalf("expected 2 ranked plans, got %d", len(ranking))
	}

	rankingByPlanID := make(map[int]dto.PlanConsumptionRank, len(ranking))
	for _, item := range ranking {
		rankingByPlanID[item.PlanId] = item
	}

	if rankingByPlanID[planA.Id].RequestCount != 1 {
		t.Fatalf("expected plan A request count 1, got %d", rankingByPlanID[planA.Id].RequestCount)
	}
	if math.Abs(rankingByPlanID[planA.Id].TotalConsumedUSD-1.0) > 1e-9 {
		t.Fatalf("expected plan A total consumed USD 1.0, got %f", rankingByPlanID[planA.Id].TotalConsumedUSD)
	}
	if rankingByPlanID[planB.Id].RequestCount != 1 {
		t.Fatalf("expected plan B request count 1, got %d", rankingByPlanID[planB.Id].RequestCount)
	}
	if math.Abs(rankingByPlanID[planB.Id].TotalConsumedUSD-3.0) > 1e-9 {
		t.Fatalf("expected plan B total consumed USD 3.0, got %f", rankingByPlanID[planB.Id].TotalConsumedUSD)
	}
}
