# 滑动验证码集成设计方案

## 概述

为了防止恶意攻击者使用临时邮箱批量注册垃圾用户，本方案在邮箱验证码发送环节集成 go-captcha 滑动验证码，形成完整的防护链。

## 问题分析

### 当前安全措施

系统已有以下防护措施：
- 邮箱验证码发送频率限制（30秒内最多2次，按IP限制）
- Turnstile CAPTCHA 验证
- 邮箱域名白名单和别名限制

### 存在的安全隐患

1. **Turnstile 可被绕过**：攻击者可以直接调用 API 接口，绕过前端的 Turnstile 验证
2. **验证码可重复使用**：当前实现中，邮箱验证码在10分钟有效期内可以被多次使用，攻击者获取一次验证码后可以注册多个账号
3. **频率限制不够严格**：基于 IP 的限制容易被代理池绕过

## 解决方案

### 核心思路

在发送邮箱验证码之前，强制要求用户完成滑动验证码验证，并确保每个邮箱验证码只能使用一次。

### 技术选型

- **后端**：`github.com/wenlng/go-captcha` - Go 语言的验证码生成库
- **前端**：`@wenlng/go-captcha-react` - React 组件库
- **验证方式**：滑动验证码（用户体验好，安全性高）

### 防护链设计

```
用户填写邮箱
    ↓
点击"获取验证码"
    ↓
显示滑动验证码弹窗
    ↓
用户拖动滑块到正确位置
    ↓
前端提交坐标到后端验证
    ↓
后端验证通过，生成一次性 token
    ↓
前端携带 token 请求发送邮件
    ↓
后端验证 token 有效性，发送邮件
    ↓
用户输入邮箱验证码注册
    ↓
注册成功后删除验证码（防止重复使用）
```

## 技术实现

### 后端实现

#### 1. 新增 CAPTCHA 接口

**GET `/api/captcha/get`** - 获取滑动验证码

请求参数：无

响应数据：
```json
{
  "success": true,
  "data": {
    "captcha_id": "uuid",
    "background_image": "base64_encoded_image",
    "slider_image": "base64_encoded_image",
    "slider_y": 100
  }
}
```

实现要点：
- 使用 go-captcha 生成随机背景图和滑块图
- 生成唯一的 captcha_id
- 将正确的滑块 X 坐标存储在 Redis/内存中（有效期 2 分钟）
- 返回图片数据和 captcha_id

**POST `/api/captcha/verify`** - 验证滑动结果

请求参数：
```json
{
  "captcha_id": "uuid",
  "x": 150,
  "y": 100
}
```

响应数据：
```json
{
  "success": true,
  "data": {
    "captcha_token": "one_time_token"
  }
}
```

实现要点：
- 从存储中获取正确的 X 坐标
- 验证用户提交的坐标是否在误差范围内（±5px）
- 验证成功后生成一次性 token（有效期 2 分钟）
- 删除 captcha_id 对应的数据（防止重复验证）
- 存储 token 及其状态（未使用）

#### 2. 修改邮箱验证码发送接口

**GET `/api/verification`** - 发送邮箱验证码

修改要点：
- 添加查询参数 `captcha_token`
- 验证 captcha_token 是否存在且有效
- 验证 token 是否已被使用
- 验证通过后标记 token 为已使用
- 发送邮件验证码

代码位置：`controller/misc.go:205` 的 `SendEmailVerification` 函数

#### 3. 修改验证码验证逻辑

**修改 `common/verification.go`**

添加函数：
```go
// VerifyAndDeleteCode 验证验证码并立即删除（一次性使用）
func VerifyAndDeleteCode(key string, code string, purpose string) bool {
    verificationMutex.Lock()
    defer verificationMutex.Unlock()

    value, okay := verificationMap[purpose+key]
    now := time.Now()
    if !okay || int(now.Sub(value.time).Seconds()) >= VerificationValidMinutes*60 {
        return false
    }

    // 验证成功后立即删除
    delete(verificationMap, purpose+key)
    return code == value.code
}
```

修改注册接口：
- 在 `controller/user.go:184` 将 `VerifyCodeWithKey` 改为 `VerifyAndDeleteCode`
- 确保每个验证码只能使用一次

#### 4. 新增文件结构

```
controller/
  ├── captcha.go          # CAPTCHA 相关接口
common/
  ├── captcha.go          # CAPTCHA 生成和验证逻辑
middleware/
  ├── captcha.go          # CAPTCHA 中间件（可选）
```

#### 5. 依赖管理

在 `go.mod` 中添加：
```
github.com/wenlng/go-captcha v1.2.5
```

### 前端实现

#### 1. 安装依赖

```bash
npm install @wenlng/go-captcha-react
```

#### 2. 创建 CAPTCHA 组件

**新建 `web/src/components/common/CaptchaModal.jsx`**

功能：
- 封装滑动验证码组件
- 处理验证码获取、显示、验证流程
- 提供回调函数通知父组件验证结果

接口：
```jsx
<CaptchaModal
  visible={showCaptcha}
  onSuccess={(token) => {
    // 验证成功，获得 token
    sendVerificationCode(token);
  }}
  onCancel={() => {
    setShowCaptcha(false);
  }}
/>
```

#### 3. 修改注册表单

**修改 `web/src/components/auth/RegisterForm.jsx`**

改动点：
1. 引入 CaptchaModal 组件
2. 修改"获取验证码"按钮的点击事件
3. 点击时先显示 CAPTCHA 弹窗
4. CAPTCHA 验证成功后再调用发送验证码接口

代码示例：
```jsx
const handleGetVerificationCode = () => {
  // 显示 CAPTCHA 弹窗
  setShowCaptcha(true);
};

const handleCaptchaSuccess = async (captchaToken) => {
  setShowCaptcha(false);
  setVerificationCodeLoading(true);

  try {
    // 携带 captcha_token 发送验证码
    const res = await API.get(
      `/api/verification?email=${inputs.email}&captcha_token=${captchaToken}`
    );

    if (res.data.success) {
      showSuccess('验证码发送成功');
      setDisableButton(true);
      // 开始倒计时
    } else {
      showError(res.data.message);
    }
  } catch (error) {
    showError('发送验证码失败');
  } finally {
    setVerificationCodeLoading(false);
  }
};
```

#### 4. 修改密码重置表单

类似地修改密码重置相关的表单，集成 CAPTCHA 验证。

### 配置管理

#### 1. 系统配置项

在系统设置中添加以下配置：

```go
// common/constants.go
var (
    CaptchaEnabled     bool   // 是否启用滑动验证码
    CaptchaMode        string // 验证码模式：slide/rotate/click
    CaptchaDifficulty  string // 难度：easy/medium/hard
)
```

#### 2. 配置初始化

在系统启动时从数据库或环境变量加载配置：

```go
func InitCaptchaConfig() {
    CaptchaEnabled = GetOptionBool("CaptchaEnabled", true)
    CaptchaMode = GetOptionString("CaptchaMode", "slide")
    CaptchaDifficulty = GetOptionString("CaptchaDifficulty", "medium")
}
```

#### 3. 前端配置获取

在 `/api/status` 接口中返回 CAPTCHA 配置：

```go
data := gin.H{
    // ... 其他配置
    "captcha_enabled": common.CaptchaEnabled,
    "captcha_mode": common.CaptchaMode,
}
```

前端根据配置决定是否显示 CAPTCHA。

### 数据存储

#### 1. CAPTCHA 数据存储

**优先使用 Redis**（如果已启用）：
```go
// 存储 CAPTCHA 答案
key := "captcha:answer:" + captchaID
rdb.Set(ctx, key, correctX, 2*time.Minute)

// 存储验证 token
key := "captcha:token:" + token
rdb.HSet(ctx, key, map[string]interface{}{
    "verified": true,
    "used": false,
    "expire_at": time.Now().Add(2*time.Minute).Unix(),
})
rdb.Expire(ctx, key, 2*time.Minute)
```

**降级到内存存储**（如果 Redis 未启用）：
```go
type captchaData struct {
    correctX  int
    createdAt time.Time
}

type captchaToken struct {
    verified bool
    used     bool
    expireAt time.Time
}

var captchaMap = make(map[string]captchaData)
var tokenMap = make(map[string]captchaToken)
var captchaMutex sync.RWMutex
```

#### 2. 清理策略

- CAPTCHA 答案：2 分钟后自动过期
- 验证 token：2 分钟后自动过期
- 定期清理过期数据（如果使用内存存储）

### 错误处理

#### 1. 后端错误码

```go
const (
    ErrCaptchaNotFound     = "CAPTCHA_NOT_FOUND"      // 验证码不存在或已过期
    ErrCaptchaVerifyFailed = "CAPTCHA_VERIFY_FAILED"  // 验证失败
    ErrCaptchaTokenInvalid = "CAPTCHA_TOKEN_INVALID"  // Token 无效
    ErrCaptchaTokenUsed    = "CAPTCHA_TOKEN_USED"     // Token 已使用
)
```

#### 2. 前端错误处理

- CAPTCHA 验证失败：提示用户重试，自动刷新验证码
- Token 过期：提示用户重新验证
- 网络错误：显示友好的错误提示，允许重试

### 兼容性处理

#### 1. 与 Turnstile 的关系

- 可以同时启用 CAPTCHA 和 Turnstile（双重保护）
- 可以只启用其中一个
- 建议：启用 CAPTCHA 后可以禁用 Turnstile，减少用户操作步骤

#### 2. 渐进式部署

1. 第一阶段：添加 CAPTCHA 功能，默认禁用
2. 第二阶段：在测试环境验证功能
3. 第三阶段：生产环境启用，观察效果
4. 第四阶段：根据效果调整配置（难度、模式等）

## 安全性分析

### 防护效果

1. **防止自动化脚本**：滑动验证码需要人工操作，自动化脚本难以绕过
2. **防止验证码重用**：一次性 token 和一次性验证码确保无法重复使用
3. **防止 API 直接调用**：所有验证都在服务端进行，无法绕过

### 攻击成本分析

- **单次攻击成本**：攻击者需要人工完成每次 CAPTCHA 验证
- **批量攻击成本**：即使使用打码平台，每次验证也需要成本和时间
- **绕过难度**：需要破解滑动验证码算法，难度较高

### 潜在风险

1. **打码平台**：攻击者可能使用人工打码平台绕过验证
   - 缓解措施：增加验证难度，提高攻击成本

2. **分布式攻击**：使用大量 IP 绕过频率限制
   - 缓解措施：结合邮箱域名黑名单、行为分析等多重防护

3. **用户体验**：增加了用户注册的步骤
   - 缓解措施：优化 UI/UX，使验证过程流畅自然

## 性能考虑

### 资源消耗

- **CPU**：生成验证码图片需要一定的 CPU 资源
- **内存**：存储验证码数据和 token（如果使用内存存储）
- **网络**：传输图片数据（base64 编码后约 50-100KB）

### 优化建议

1. **图片缓存**：预生成一批验证码图片，减少实时生成压力
2. **图片压缩**：使用 JPEG 格式，调整质量参数
3. **CDN 加速**：如果图片较大，考虑使用 CDN
4. **Redis 存储**：多实例部署时必须使用 Redis，避免数据不一致

## 测试计划

### 功能测试

1. CAPTCHA 生成和显示
2. 滑动验证成功/失败场景
3. Token 生成和验证
4. 邮箱验证码发送（携带 token）
5. 验证码一次性使用
6. 过期时间验证

### 安全测试

1. 尝试绕过 CAPTCHA 直接发送验证码
2. 尝试重复使用 token
3. 尝试重复使用邮箱验证码
4. 尝试伪造 token
5. 并发请求测试

### 性能测试

1. 验证码生成性能
2. 并发验证性能
3. 内存占用情况
4. Redis 存储性能

## 部署注意事项

### 环境要求

- Go 版本：1.18+
- Node.js 版本：16+
- Redis（推荐）：用于多实例部署

### 部署步骤

1. 更新后端依赖：`go mod tidy`
2. 更新前端依赖：`npm install`
3. 配置 CAPTCHA 相关参数
4. 重启服务
5. 验证功能正常

### 回滚方案

如果出现问题，可以通过配置快速禁用 CAPTCHA：
```go
CaptchaEnabled = false
```

系统会自动降级到原有的 Turnstile 验证。

## 后续优化

### 短期优化

1. 添加 CAPTCHA 验证失败次数限制
2. 添加 CAPTCHA 刷新次数限制
3. 优化图片生成性能

### 长期优化

1. 支持多种验证码类型（旋转、点选等）
2. 基于用户行为的风险评分
3. 机器学习识别异常注册行为
4. 集成第三方验证服务（如 reCAPTCHA v3）

## 总结

本方案通过集成 go-captcha 滑动验证码，在邮箱验证码发送环节增加了人机验证，并通过一次性 token 和一次性验证码机制，有效防止了恶意批量注册。方案在安全性和用户体验之间取得了良好的平衡，同时保持了系统的可扩展性和可维护性。
