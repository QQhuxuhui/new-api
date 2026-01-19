package common

import (
	"sync"
	"time"

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
)

const (
	CaptchaExpiration      = 2 * time.Minute
	CaptchaTokenExpiration = 2 * time.Minute
	CaptchaMapMaxSize      = 10000
	TokenMapMaxSize        = 10000
	CleanupInterval        = 1 * time.Minute
)

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
	defer captchaMutex.Unlock()

	captchaMap[captchaID] = CaptchaData{
		CorrectX:  correctX,
		CreatedAt: time.Now(),
	}

	// Only clean up if map is getting large
	if len(captchaMap) >= CaptchaMapMaxSize*9/10 {
		cleanExpiredCaptchaData()
	}
}

// GetCaptchaAnswer 获取验证码答案
func GetCaptchaAnswer(captchaID string) (int, bool) {
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
	defer captchaMutex.Unlock()

	delete(captchaMap, captchaID)
}

// StoreCaptchaToken 存储验证 token
func StoreCaptchaToken(token string) {
	captchaMutex.Lock()
	defer captchaMutex.Unlock()

	captchaTokenMap[token] = CaptchaToken{
		Verified: true,
		Used:     false,
		ExpireAt: time.Now().Add(CaptchaTokenExpiration),
	}

	// Only clean up if map is getting large
	if len(captchaTokenMap) >= TokenMapMaxSize*9/10 {
		cleanExpiredTokens()
	}
}

// VerifyAndUseCaptchaToken 验证并使用 token（一次性）
func VerifyAndUseCaptchaToken(token string) bool {
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
	close(cleanupShutdown)
}
