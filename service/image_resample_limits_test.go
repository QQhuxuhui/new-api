package service

import "testing"

func TestResampleLimitEnvClampsPositiveValues(t *testing.T) {
	if got := resampleLimitEnv("", 8, 32); got != 8 {
		t.Fatalf("empty value got %d", got)
	}
	t.Setenv("IMAGE_RESAMPLE_TEST_LIMIT", "64")
	if got := resampleLimitEnv("IMAGE_RESAMPLE_TEST_LIMIT", 8, 32); got != 32 {
		t.Fatalf("value above cap got %d", got)
	}
	t.Setenv("IMAGE_RESAMPLE_TEST_LIMIT", "16")
	if got := resampleLimitEnv("IMAGE_RESAMPLE_TEST_LIMIT", 8, 32); got != 16 {
		t.Fatalf("valid value got %d", got)
	}
}

func TestInitImageResampleLimitsUsesEnvironment(t *testing.T) {
	oldLocalSem := localResampleSem
	oldWaiters := resampleMaxWaiters
	oldRewriteSem := imageRewriteSem
	defer func() {
		localResampleSem = oldLocalSem
		resampleMaxWaiters = oldWaiters
		imageRewriteSem = oldRewriteSem
	}()

	t.Setenv("IMAGE_RESAMPLE_LOCAL_MAX_CONCURRENCY", "7")
	t.Setenv("IMAGE_RESAMPLE_MAX_WAITERS", "11")
	InitImageResampleLimits()

	if got := cap(localResampleSem); got != 7 {
		t.Fatalf("local concurrency after startup init = %d", got)
	}
	if got := resampleMaxWaiters; got != 11 {
		t.Fatalf("waiter limit after startup init = %d", got)
	}
}
