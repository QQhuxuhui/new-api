package common

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func TestCaptchaStoreRedisAndFallback(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	prevRDB := RDB
	prevEnabled := RedisEnabled
	RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	RedisEnabled = true
	defer func() {
		RDB = prevRDB
		RedisEnabled = prevEnabled
	}()

	captchaID := "captcha-test-1"
	StoreCaptchaAnswer(captchaID, 42)

	ctx := context.Background()
	if exists, _ := RDB.Exists(ctx, "captcha:answer:"+captchaID).Result(); exists != 1 {
		t.Fatalf("expected captcha answer in redis")
	}

	if got, ok := GetCaptchaAnswer(captchaID); !ok || got != 42 {
		t.Fatalf("expected answer 42 from redis, got %d (ok=%v)", got, ok)
	}

	if err := RedisDel("captcha:answer:" + captchaID); err != nil {
		t.Fatalf("cleanup redis key: %v", err)
	}

	StoreCaptchaAnswer(captchaID, 77)
	if err := RDB.Close(); err != nil {
		t.Fatalf("close redis: %v", err)
	}

	got, ok := GetCaptchaAnswer(captchaID)
	if !ok || got != 77 {
		t.Fatalf("expected fallback answer 77, got %d (ok=%v)", got, ok)
	}

	captchaToken := "token-test-1"
	StoreCaptchaToken(captchaToken)
	if ok := VerifyAndUseCaptchaToken(captchaToken); !ok {
		t.Fatalf("expected token verify in memory fallback")
	}
	if ok := VerifyAndUseCaptchaToken(captchaToken); ok {
		t.Fatalf("expected token to be one-time use")
	}
}

func TestCaptchaTokenRedisOneTime(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	prevRDB := RDB
	prevEnabled := RedisEnabled
	RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	RedisEnabled = true
	defer func() {
		RDB = prevRDB
		RedisEnabled = prevEnabled
	}()

	captchaToken := "token-test-redis"
	StoreCaptchaToken(captchaToken)

	if ok := VerifyAndUseCaptchaToken(captchaToken); !ok {
		t.Fatalf("expected token verify to succeed")
	}
	if ok := VerifyAndUseCaptchaToken(captchaToken); ok {
		t.Fatalf("expected token to be one-time use")
	}

	// token should be gone from redis
	ctx := context.Background()
	if exists, _ := RDB.Exists(ctx, "captcha:token:"+captchaToken).Result(); exists != 0 {
		t.Fatalf("expected token key to be deleted")
	}
}

func TestCaptchaAnswerExpiryRedis(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	prevRDB := RDB
	prevEnabled := RedisEnabled
	RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	RedisEnabled = true
	defer func() {
		RDB = prevRDB
		RedisEnabled = prevEnabled
	}()

	captchaID := "captcha-expire"
	StoreCaptchaAnswer(captchaID, 11)

	mr.FastForward(CaptchaExpiration + time.Second)

	if got, ok := GetCaptchaAnswer(captchaID); ok {
		t.Fatalf("expected expired captcha, got %d", got)
	}
}
