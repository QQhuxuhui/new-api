package common

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

// CaptchaData 存储验证码数据
type CaptchaData struct {
	CorrectX  int
	CreatedAt time.Time
}

// CaptchaToken 存储验证 token
type CaptchaToken struct {
	Verified bool
	Used     bool
	ExpireAt time.Time
}

var (
	captchaMap      = make(map[string]CaptchaData)
	captchaTokenMap = make(map[string]CaptchaToken)
	captchaMutex    sync.RWMutex
	cleanupShutdown = make(chan struct{})
	shutdownOnce    sync.Once
)

const (
	CaptchaExpiration      = 2 * time.Minute
	CaptchaTokenExpiration = 2 * time.Minute
	CaptchaMapMaxSize      = 10000
	TokenMapMaxSize        = 10000
	CleanupInterval        = 1 * time.Minute
)

const (
	captchaAnswerKeyPrefix = "captcha:answer:"
	captchaTokenKeyPrefix  = "captcha:token:"
)

var captchaTokenConsumeScript = redis.NewScript(`
local val = redis.call('GET', KEYS[1])
if not val then
  return 0
end
redis.call('DEL', KEYS[1])
return 1
`)

func captchaAnswerKey(captchaID string) string {
	return captchaAnswerKeyPrefix + captchaID
}

func captchaTokenKey(token string) string {
	return captchaTokenKeyPrefix + token
}

func redisAvailable() bool {
	return RedisEnabled && RDB != nil
}

func init() {
	go backgroundCleanup()
}

func backgroundCleanup() {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			captchaMutex.Lock()
			cleanExpiredCaptchaData()
			cleanExpiredTokens()
			captchaMutex.Unlock()
		case <-cleanupShutdown:
			return
		}
	}
}

// StoreCaptchaAnswer 存储验证码答案
func StoreCaptchaAnswer(captchaID string, correctX int) {
	captchaMutex.Lock()
	captchaMap[captchaID] = CaptchaData{
		CorrectX:  correctX,
		CreatedAt: time.Now(),
	}
	// Only clean up if map is getting large
	if len(captchaMap) >= CaptchaMapMaxSize*9/10 {
		cleanExpiredCaptchaData()
	}
	captchaMutex.Unlock()

	if redisAvailable() {
		ctx := context.Background()
		if err := RDB.Set(ctx, captchaAnswerKey(captchaID), correctX, CaptchaExpiration).Err(); err != nil {
			SysLog("failed to store captcha answer in redis: " + err.Error())
		}
	}
}

// GetCaptchaAnswer 获取验证码答案
func GetCaptchaAnswer(captchaID string) (int, bool) {
	if redisAvailable() {
		ctx := context.Background()
		val, err := RDB.Get(ctx, captchaAnswerKey(captchaID)).Result()
		if err == nil {
			parsed, parseErr := strconv.Atoi(val)
			if parseErr != nil {
				return 0, false
			}
			return parsed, true
		}
		if !errors.Is(err, redis.Nil) {
			SysLog("failed to get captcha answer from redis: " + err.Error())
			// Redis error -> fallback to memory
		} else {
			// Redis says not found -> treat as invalid without fallback
			return 0, false
		}
	}

	captchaMutex.RLock()
	defer captchaMutex.RUnlock()

	data, exists := captchaMap[captchaID]
	if !exists {
		return 0, false
	}

	// 检查是否过期
	if time.Since(data.CreatedAt) > CaptchaExpiration {
		return 0, false
	}

	return data.CorrectX, true
}

// DeleteCaptchaAnswer 删除验证码答案
func DeleteCaptchaAnswer(captchaID string) {
	captchaMutex.Lock()
	delete(captchaMap, captchaID)
	captchaMutex.Unlock()

	if redisAvailable() {
		if err := RDB.Del(context.Background(), captchaAnswerKey(captchaID)).Err(); err != nil && !errors.Is(err, redis.Nil) {
			SysLog("failed to delete captcha answer from redis: " + err.Error())
		}
	}
}

// StoreCaptchaToken 存储验证 token
func StoreCaptchaToken(token string) {
	captchaMutex.Lock()
	captchaTokenMap[token] = CaptchaToken{
		Verified: true,
		Used:     false,
		ExpireAt: time.Now().Add(CaptchaTokenExpiration),
	}
	// Only clean up if map is getting large
	if len(captchaTokenMap) >= TokenMapMaxSize*9/10 {
		cleanExpiredTokens()
	}
	captchaMutex.Unlock()

	if redisAvailable() {
		ctx := context.Background()
		if err := RDB.Set(ctx, captchaTokenKey(token), "1", CaptchaTokenExpiration).Err(); err != nil {
			SysLog("failed to store captcha token in redis: " + err.Error())
		}
	}
}

// VerifyAndUseCaptchaToken 验证并使用 token（一次性）
func VerifyAndUseCaptchaToken(token string) bool {
	if redisAvailable() {
		ctx := context.Background()
		res, err := captchaTokenConsumeScript.Run(ctx, RDB, []string{captchaTokenKey(token)}).Int()
		if err == nil {
			if res == 1 {
				captchaMutex.Lock()
				delete(captchaTokenMap, token)
				captchaMutex.Unlock()
				return true
			}
			return false
		}
		if !errors.Is(err, redis.Nil) {
			SysLog("failed to verify captcha token in redis: " + err.Error())
			// Redis error -> fallback to memory
		} else {
			return false
		}
	}

	captchaMutex.Lock()
	defer captchaMutex.Unlock()

	tokenData, exists := captchaTokenMap[token]
	if !exists {
		return false
	}

	// 检查是否过期
	if time.Now().After(tokenData.ExpireAt) {
		delete(captchaTokenMap, token)
		return false
	}

	// 检查是否已使用
	if tokenData.Used {
		return false
	}

	// 标记为已使用
	tokenData.Used = true
	captchaTokenMap[token] = tokenData

	return true
}

// cleanExpiredCaptchaData 清理过期的验证码数据（无锁，调用者需加锁）
func cleanExpiredCaptchaData() {
	now := time.Now()
	for id, data := range captchaMap {
		if now.Sub(data.CreatedAt) > CaptchaExpiration {
			delete(captchaMap, id)
		}
	}
}

// cleanExpiredTokens 清理过期的 token（无锁，调用者需加锁）
func cleanExpiredTokens() {
	now := time.Now()
	for token, data := range captchaTokenMap {
		if now.After(data.ExpireAt) {
			delete(captchaTokenMap, token)
		}
	}
}

// GenerateCaptchaToken 生成唯一的 token
func GenerateCaptchaToken() string {
	return uuid.New().String()
}

// StopBackgroundCleanup stops the background cleanup goroutine
func StopBackgroundCleanup() {
	shutdownOnce.Do(func() {
		close(cleanupShutdown)
	})
}
