package model

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/go-redis/redis/v8"
)

// UserBase struct remains the same as it represents the cached data structure
type UserBase struct {
	Id             int    `json:"id"`
	Group          string `json:"group"`
	Email          string `json:"email"`
	Quota          int    `json:"quota"`
	Status         int    `json:"status"`
	Username       string `json:"username"`
	Setting        string `json:"setting"`
	MaxConcurrency int    `json:"max_concurrency"`
}

func (user *UserBase) WriteContext(c *gin.Context) {
	common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
	common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
	common.SetContextKey(c, constant.ContextKeyUserStatus, user.Status)
	common.SetContextKey(c, constant.ContextKeyUserEmail, user.Email)
	common.SetContextKey(c, constant.ContextKeyUserName, user.Username)
	common.SetContextKey(c, constant.ContextKeyUserSetting, user.GetSetting())
	common.SetContextKey(c, constant.ContextKeyUserMaxConcurrency, user.MaxConcurrency)
}

func (user *UserBase) GetSetting() dto.UserSetting {
	setting := dto.UserSetting{}
	if user.Setting != "" {
		err := common.Unmarshal([]byte(user.Setting), &setting)
		if err != nil {
			common.SysLog("failed to unmarshal setting: " + err.Error())
		}
	}
	return setting
}

// getUserCacheKey returns the key for user cache
func getUserCacheKey(userId int) string {
	return fmt.Sprintf("user:%d", userId)
}

func getUserCacheGenerationKey(userId int) string {
	return fmt.Sprintf("user:%d:generation", userId)
}

func getUserCacheGeneration(userId int) (int64, error) {
	if !common.RedisEnabled {
		return 0, nil
	}
	return common.RedisGetGeneration(getUserCacheGenerationKey(userId))
}

// invalidateUserCache clears user cache
func invalidateUserCache(userId int) error {
	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}
	return common.RedisBumpGenerationAndDelete(getUserCacheGenerationKey(userId), getUserCacheKey(userId))
}

// InvalidateUserCache clears the cached user record after a transactional
// balance update performed outside the standard quota helpers.
func InvalidateUserCache(userId int) error {
	return invalidateUserCache(userId)
}

func updateUserCacheAtGeneration(user User, generation int64) (bool, error) {
	if !common.RedisEnabled {
		return false, nil
	}

	return common.RedisHSetObjIfGeneration(
		getUserCacheKey(user.Id),
		user.ToBaseUser(),
		time.Duration(common.RedisKeyCacheSeconds())*time.Second,
		getUserCacheGenerationKey(user.Id),
		generation,
	)
}

// GetUserCache gets complete user cache from hash
func GetUserCache(userId int) (userCache *UserBase, err error) {
	var user *User
	var fromDB bool
	var refillGeneration int64

	// Try getting from Redis first
	userCache, err = cacheGetUserBase(userId)
	if err == nil {
		return userCache, nil
	}

	// If Redis fails, get from DB
	fromDB = true
	refillGeneration, generationErr := getUserCacheGeneration(userId)
	if generationErr != nil {
		common.SysLog("failed to read user cache generation: " + generationErr.Error())
	}
	user, err = GetUserById(userId, false)
	if err != nil {
		return nil, err // Return nil and error if DB lookup fails
	}

	// Create cache object from user data
	userCache = &UserBase{
		Id:             user.Id,
		Group:          user.Group,
		Quota:          user.Quota,
		Status:         user.Status,
		Username:       user.Username,
		Setting:        user.Setting,
		Email:          user.Email,
		MaxConcurrency: user.MaxConcurrency,
	}
	if generationErr == nil && shouldUpdateRedis(fromDB, nil) {
		gopool.Go(func() {
			if _, cacheErr := updateUserCacheAtGeneration(*user, refillGeneration); cacheErr != nil {
				common.SysLog("failed to update user status cache: " + cacheErr.Error())
			}
		})
	}

	return userCache, nil
}

func cacheGetUserBase(userId int) (*UserBase, error) {
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	var userCache UserBase
	// Try getting from Redis first
	err := common.RedisHGetObj(getUserCacheKey(userId), &userCache)
	if err != nil {
		return nil, err
	}
	return &userCache, nil
}

// Add atomic quota operations using hash fields
func cacheIncrUserQuota(userId int, delta int64) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisBumpGenerationAndHIncrByIfExists(
		getUserCacheGenerationKey(userId), getUserCacheKey(userId), "Quota", delta,
	)
}

func cacheDecrUserQuota(userId int, delta int64) error {
	return cacheIncrUserQuota(userId, -delta)
}

// Helper functions to get individual fields if needed
func getUserGroupCache(userId int) (string, error) {
	cache, err := GetUserCache(userId)
	if err != nil {
		return "", err
	}
	return cache.Group, nil
}

func getUserQuotaCache(userId int) (int, error) {
	cache, err := GetUserCache(userId)
	if err != nil {
		return 0, err
	}
	return cache.Quota, nil
}

func getUserStatusCache(userId int) (int, error) {
	cache, err := GetUserCache(userId)
	if err != nil {
		return 0, err
	}
	return cache.Status, nil
}

func getUserNameCache(userId int) (string, error) {
	cache, err := GetUserCache(userId)
	if err != nil {
		return "", err
	}
	return cache.Username, nil
}

func getUserSettingCache(userId int) (dto.UserSetting, error) {
	cache, err := GetUserCache(userId)
	if err != nil {
		return dto.UserSetting{}, err
	}
	return cache.GetSetting(), nil
}

// New functions for individual field updates
func updateUserStatusCache(userId int, status bool) error {
	if !common.RedisEnabled {
		return nil
	}
	statusInt := common.UserStatusEnabled
	if !status {
		statusInt = common.UserStatusDisabled
	}
	return common.RedisBumpGenerationAndHSetIfExists(
		getUserCacheGenerationKey(userId), getUserCacheKey(userId), "Status", fmt.Sprintf("%d", statusInt),
	)
}

func updateUserQuotaCache(userId int, quota int) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisBumpGenerationAndHSetIfExists(
		getUserCacheGenerationKey(userId), getUserCacheKey(userId), "Quota", fmt.Sprintf("%d", quota),
	)
}

func updateUserGroupCache(userId int, group string) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisBumpGenerationAndHSetIfExists(
		getUserCacheGenerationKey(userId), getUserCacheKey(userId), "Group", group,
	)
}

func updateUserNameCache(userId int, username string) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisBumpGenerationAndHSetIfExists(
		getUserCacheGenerationKey(userId), getUserCacheKey(userId), "Username", username,
	)
}

func updateUserSettingCache(userId int, setting string) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisBumpGenerationAndHSetIfExists(
		getUserCacheGenerationKey(userId), getUserCacheKey(userId), "Setting", setting,
	)
}

// ------------------ User concurrency counters ------------------

const userConcurrencyTTL = 5 * time.Minute

func getUserConcurrencyKey(userId int) string {
	return fmt.Sprintf("user_concurrency:%d", userId)
}

// IncrUserConcurrency increments user's active request count and refreshes TTL.
// Returns the current count after increment. Fail-open when Redis is disabled.
func IncrUserConcurrency(userId int) (int64, error) {
	if !common.RedisEnabled {
		return 0, nil
	}
	key := getUserConcurrencyKey(userId)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var incrCmd *redis.IntCmd
	_, err := common.RDB.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		incrCmd = pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, userConcurrencyTTL) // refresh TTL each increment
		return nil
	})
	if err != nil {
		return 0, err
	}
	if incrCmd == nil {
		return 0, fmt.Errorf("increment command not executed")
	}
	return incrCmd.Val(), nil
}

// DecrUserConcurrency decrements user's active request count and cleans up when it drops to zero.
func DecrUserConcurrency(userId int) error {
	if !common.RedisEnabled {
		return nil
	}
	key := getUserConcurrencyKey(userId)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	val, err := common.RDB.Decr(ctx, key).Result()
	if err != nil {
		return err
	}
	if val <= 0 {
		// remove counter to avoid stale keys
		_, _ = common.RDB.Del(ctx, key).Result()
	}
	return nil
}

// GetUserConcurrency returns the current active request count for a user.
func GetUserConcurrency(userId int) (int64, error) {
	if !common.RedisEnabled {
		return 0, nil
	}
	key := getUserConcurrencyKey(userId)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	val, err := common.RDB.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(val, 10, 64)
}
