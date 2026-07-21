package model

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

func tokenCacheKey(key string) string {
	return fmt.Sprintf("token:%s", common.GenerateHMAC(key))
}

func tokenCacheGenerationKey(key string) string {
	return fmt.Sprintf("token:%s:generation", common.GenerateHMAC(key))
}

func getTokenCacheGeneration(key string) (int64, error) {
	if !common.RedisEnabled {
		return 0, nil
	}
	return common.RedisGetGeneration(tokenCacheGenerationKey(key))
}

func cacheSetTokenAtGeneration(token Token, generation int64) (bool, error) {
	key := token.Key
	token.Clean()
	return common.RedisHSetObjIfGeneration(
		tokenCacheKey(key),
		&token,
		time.Duration(common.RedisKeyCacheSeconds())*time.Second,
		tokenCacheGenerationKey(key),
		generation,
	)
}

func cacheDeleteToken(key string) error {
	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}
	return common.RedisBumpGenerationAndDelete(tokenCacheGenerationKey(key), tokenCacheKey(key))
}

// InvalidateTokenQuotaCacheByID removes the cached token snapshot after a
// transactional quota update. Deletion is idempotent, so accounting retries
// cannot apply a quota delta twice.
func InvalidateTokenQuotaCacheByID(id int) error {
	if id <= 0 || !common.RedisEnabled {
		return nil
	}
	var key string
	if err := DB.Model(&Token{}).Where("id = ?", id).Pluck("key", &key).Error; err != nil {
		return err
	}
	if key == "" {
		return nil
	}
	return cacheDeleteToken(key)
}

func cacheIncrTokenQuota(key string, increment int64) error {
	if !common.RedisEnabled {
		return nil
	}
	err := common.RedisBumpGenerationAndHIncrByIfExists(
		tokenCacheGenerationKey(key), tokenCacheKey(key), constant.TokenFiledRemainQuota, increment,
	)
	if err != nil {
		return err
	}
	return nil
}

func cacheDecrTokenQuota(key string, decrement int64) error {
	return cacheIncrTokenQuota(key, -decrement)
}

func cacheSetTokenField(key string, field string, value string) error {
	if !common.RedisEnabled {
		return nil
	}
	err := common.RedisBumpGenerationAndHSetIfExists(
		tokenCacheGenerationKey(key), tokenCacheKey(key), field, value,
	)
	if err != nil {
		return err
	}
	return nil
}

// CacheGetTokenByKey 从缓存中获取 token，如果缓存中不存在，则从数据库中获取
func cacheGetTokenByKey(key string) (*Token, error) {
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	var token Token
	err := common.RedisHGetObj(tokenCacheKey(key), &token)
	if err != nil {
		return nil, err
	}
	token.Key = key
	return &token, nil
}
