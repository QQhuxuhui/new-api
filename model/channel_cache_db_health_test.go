package model

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func TestGetRandomSatisfiedChannelDetailed_DatabaseModeSkipsSuspendedChannel(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	if err := DB.AutoMigrate(&Channel{}, &Ability{}); err != nil {
		t.Fatalf("migrate channel tables: %v", err)
	}
	enableUserPlanExpiryRedis(t)

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousUsingSQLite := common.UsingSQLite
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousGroupCol := commonGroupCol
	common.MemoryCacheEnabled = false
	common.UsingSQLite = true
	common.UsingPostgreSQL = false
	initCol()
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.UsingSQLite = previousUsingSQLite
		common.UsingPostgreSQL = previousUsingPostgreSQL
		commonGroupCol = previousGroupCol
	})

	priority := int64(10)
	channel := &Channel{
		Name: "suspended-db-channel", Key: "test",
		Status: common.ChannelStatusEnabled, Group: "suspended", Models: "gpt-test", Priority: &priority,
	}
	if err := DB.Create(channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := DB.Create(&Ability{
		Group: "suspended", Model: "gpt-test", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: 100,
	}).Error; err != nil {
		t.Fatalf("create ability: %v", err)
	}
	if err := common.RedisSet(fmt.Sprintf("channel:health:%d:suspended", channel.Id), "1", time.Minute); err != nil {
		t.Fatalf("suspend channel: %v", err)
	}

	selected, _, err := GetRandomSatisfiedChannelDetailed("suspended", "gpt-test", 0, false)
	if err != nil {
		t.Fatalf("first priority: %v", err)
	}
	if selected != nil {
		t.Fatalf("selected suspended channel %d", selected.Id)
	}
	_, _, err = GetRandomSatisfiedChannelDetailed("suspended", "gpt-test", 1, false)
	if !errors.Is(err, ErrPriorityExhausted) {
		t.Fatalf("next priority error=%v, want ErrPriorityExhausted", err)
	}
}

func TestGetRandomSatisfiedChannelDetailed_DatabaseModeChoosesHealthyPeerAtSamePriority(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	if err := DB.AutoMigrate(&Channel{}, &Ability{}); err != nil {
		t.Fatalf("migrate channel tables: %v", err)
	}
	enableUserPlanExpiryRedis(t)

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousUsingSQLite := common.UsingSQLite
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousGroupCol := commonGroupCol
	common.MemoryCacheEnabled = false
	common.UsingSQLite = true
	common.UsingPostgreSQL = false
	initCol()
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.UsingSQLite = previousUsingSQLite
		common.UsingPostgreSQL = previousUsingPostgreSQL
		commonGroupCol = previousGroupCol
	})

	priority := int64(10)
	suspended := &Channel{
		Name: "weighted-suspended", Key: "test",
		Status: common.ChannelStatusEnabled, Group: "mixed", Models: "gpt-test", Priority: &priority,
	}
	healthy := &Channel{
		Name: "healthy-peer", Key: "test",
		Status: common.ChannelStatusEnabled, Group: "mixed", Models: "gpt-test", Priority: &priority,
	}
	for _, channel := range []*Channel{suspended, healthy} {
		if err := DB.Create(channel).Error; err != nil {
			t.Fatalf("create channel: %v", err)
		}
	}
	if err := DB.Create(&Ability{
		Group: "mixed", Model: "gpt-test", ChannelId: suspended.Id,
		Enabled: true, Priority: &priority, Weight: 1_000_000,
	}).Error; err != nil {
		t.Fatalf("create suspended ability: %v", err)
	}
	if err := DB.Create(&Ability{
		Group: "mixed", Model: "gpt-test", ChannelId: healthy.Id,
		Enabled: true, Priority: &priority, Weight: 0,
	}).Error; err != nil {
		t.Fatalf("create healthy ability: %v", err)
	}
	if err := common.RedisSet(fmt.Sprintf("channel:health:%d:suspended", suspended.Id), "1", time.Minute); err != nil {
		t.Fatalf("suspend channel: %v", err)
	}

	for attempt := 0; attempt < 20; attempt++ {
		selected, _, err := GetRandomSatisfiedChannelDetailed("mixed", "gpt-test", 0, false)
		if err != nil {
			t.Fatalf("select attempt %d: %v", attempt, err)
		}
		if selected == nil || selected.Id != healthy.Id {
			t.Fatalf("attempt %d selected=%v, want healthy channel %d", attempt, selected, healthy.Id)
		}
	}
}
