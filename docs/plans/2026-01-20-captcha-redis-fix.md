# CAPTCHA Redis & 配置修复 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 修复 CAPTCHA 开关未生效、注册页无条件弹窗，以及多实例验证码存储问题，并在 Redis 不可用时回退到内存存储。

**Architecture:** 后端以 Redis 为主存储验证码答案与 token（带 TTL），Redis 不可用时回退到内存；配置项 `CaptchaEnabled` 接入现有 OptionMap；前端注册页依据 `/api/status` 的 `captcha_enabled` 决定是否弹窗。

**Tech Stack:** Go（gin、go-redis v8）、React（Semi UI）。

### Task 1: 接入 CaptchaEnabled 配置

**Files:**
- Modify: `common/init.go`
- Modify: `model/option.go`
- Modify: `main.go`

**Step 1: Write the failing test**

```go
func TestCaptchaEnabledFromOptions(t *testing.T) {
    // 期望 OptionMap 初始化后能更新 CaptchaEnabled
}
```

**Step 2: Run test to verify it fails**

Run: `GOCACHE=/tmp/go-build-cache go test ./model -run TestCaptchaEnabledFromOptions -v`
Expected: FAIL (配置未接入，或测试尚未实现)。

**Step 3: Write minimal implementation**

- 在 `common/init.go` 增加 `CaptchaEnabled = GetEnvOrDefaultBool("CAPTCHA_ENABLED", false)`。
- 在 `model/option.go` 的 `InitOptionMap()` 中加入 `CaptchaEnabled`。
- 在 `model/option.go` 的 `updateOptionMap()` 中处理 `CaptchaEnabled`。
- 在 `main.go` 中仅当 `CaptchaEnabled` 为 true 时初始化 CAPTCHA（避免禁用时启动失败）。

**Step 4: Run test to verify it passes**

Run: `GOCACHE=/tmp/go-build-cache go test ./model -run TestCaptchaEnabledFromOptions -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add common/init.go model/option.go main.go model/option_test.go
git commit -m "fix: wire captcha enabled option"
```

### Task 2: Redis 存储验证码答案与 token（回退内存）

**Files:**
- Modify: `common/captcha_store.go`
- Test: `common/captcha_store_test.go`

**Step 1: Write the failing test**

```go
func TestCaptchaStoreMemoryFallback(t *testing.T) {
    // Redis 不可用时，Store/Get/Verify 仍能正常工作
}
```

**Step 2: Run test to verify it fails**

Run: `GOCACHE=/tmp/go-build-cache go test ./common -run TestCaptchaStoreMemoryFallback -v`
Expected: FAIL (尚未实现 Redis 分支/回退逻辑)。

**Step 3: Write minimal implementation**

- 为答案与 token 增加 Redis key 前缀（如 `captcha:answer:` / `captcha:token:`）。
- `StoreCaptchaAnswer/StoreCaptchaToken`：优先写 Redis，失败则写内存。
- `GetCaptchaAnswer/VerifyAndUseCaptchaToken`：优先读 Redis，Redis 返回 nil 时仍允许回退内存；Redis 真实错误则记录日志并回退内存。
- Redis token 使用 Lua 脚本原子“取用并删除”。
- 保留现有内存清理逻辑。

**Step 4: Run test to verify it passes**

Run: `GOCACHE=/tmp/go-build-cache go test ./common -run TestCaptchaStoreMemoryFallback -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add common/captcha_store.go common/captcha_store_test.go
git commit -m "fix: store captcha in redis with memory fallback"
```

### Task 3: 注册页按配置决定是否弹出 CAPTCHA

**Files:**
- Modify: `web/src/components/auth/RegisterForm.jsx`

**Step 1: Write the failing test**

```jsx
// 若无前端测试框架，记录为手工验证：captcha_enabled=false 时不弹窗
```

**Step 2: Run test to verify it fails**

Run: 手工验证（当前总是弹窗）。
Expected: 失败（禁用时仍弹窗）。

**Step 3: Write minimal implementation**

- 从 `status.captcha_enabled` 读取开关并存入 state。
- `handleGetVerificationCode` 在启用时弹窗，否则直接调用 `sendVerificationCode()`。

**Step 4: Run test to verify it passes**

Run: 手工验证。
Expected: 通过。

**Step 5: Commit**

```bash
git add web/src/components/auth/RegisterForm.jsx
git commit -m "fix: respect captcha_enabled on register"
```

### Task 4: 全量回归

**Files:**
- None

**Step 1: Run tests**

Run: `GOCACHE=/tmp/go-build-cache go test ./...`
Expected: PASS.

> 注意：为避免 `web/dist` 缺失导致 `go test` 失败，可在本地生成或临时创建 `web/dist/index.html`（无需提交）。

**Step 2: Commit (if needed)**

若有额外改动，再提交。
