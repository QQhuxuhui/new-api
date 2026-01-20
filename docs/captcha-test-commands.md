# CAPTCHA API 测试命令

本文档提供了测试滑动验证码集成功能的 curl 命令和测试步骤。

## 前置条件

1. 确保服务器已启动并运行在 `http://localhost:3000`（根据实际端口调整）
2. 确保 CAPTCHA 功能已启用（环境变量 `CAPTCHA_ENABLED=true`）

## 测试流程

### 1. 获取滑动验证码

**端点:** `GET /api/captcha/get`

**描述:** 获取一个新的滑动验证码，包含背景图片、滑块图片和滑块 Y 坐标。

**测试命令:**

```bash
curl -X GET "http://localhost:3000/api/captcha/get" \
  -H "Content-Type: application/json"
```

**预期响应:**

```json
{
  "success": true,
  "message": "",
  "data": {
    "captcha_id": "uuid-string",
    "background_image": "data:image/png;base64,...",
    "slider_image": "data:image/png;base64,...",
    "slider_y": 123
  }
}
```

**验证点:**
- HTTP 状态码应为 200
- `success` 字段应为 `true`
- `data.captcha_id` 应为有效的 UUID
- `data.background_image` 和 `data.slider_image` 应为 base64 编码的图片数据
- `data.slider_y` 应为整数

---

### 2. 验证滑动验证码

**端点:** `POST /api/captcha/verify`

**描述:** 验证用户滑动的 X 坐标是否正确，成功后返回一次性 token。

**测试命令:**

```bash
# 替换 CAPTCHA_ID 为步骤 1 返回的 captcha_id
# 替换 X_COORDINATE 为用户滑动的 X 坐标（测试时可以尝试不同的值）

curl -X POST "http://localhost:3000/api/captcha/verify" \
  -H "Content-Type: application/json" \
  -d '{
    "captcha_id": "CAPTCHA_ID",
    "x": X_COORDINATE
  }'
```

**成功响应示例:**

```json
{
  "success": true,
  "message": "验证成功",
  "data": {
    "token": "uuid-token-string"
  }
}
```

**失败响应示例:**

```json
{
  "success": false,
  "message": "验证失败，请重试"
}
```

**验证点:**
- 正确的 X 坐标（±5px 误差范围内）应返回成功
- 错误的 X 坐标应返回失败
- 同一个 `captcha_id` 只能验证一次（第二次验证应失败）
- 成功验证后应返回有效的 `token`

---

### 3. 使用 CAPTCHA Token 发送邮箱验证码

**端点:** `GET /api/verification`

**描述:** 使用步骤 2 获得的 token 发送邮箱验证码。

**测试命令:**

```bash
# 替换 YOUR_EMAIL 为测试邮箱
# 替换 CAPTCHA_TOKEN 为步骤 2 返回的 token

curl -X GET "http://localhost:3000/api/verification?email=YOUR_EMAIL&captcha_token=CAPTCHA_TOKEN" \
  -H "Content-Type: application/json"
```

**成功响应示例:**

```json
{
  "success": true,
  "message": "验证码发送成功，有效期为10分钟，请注意查收"
}
```

**失败响应示例（无 token）:**

```json
{
  "success": false,
  "message": "请先完成验证码验证"
}
```

**失败响应示例（token 无效或已使用）:**

```json
{
  "success": false,
  "message": "验证码token无效或已过期"
}
```

**验证点:**
- 有效的 token 应成功发送邮件
- 无 token 应返回错误提示
- 已使用的 token 应返回错误（token 只能使用一次）
- 过期的 token（2分钟后）应返回错误

---

## 完整测试脚本

以下是一个完整的测试脚本示例（需要 `jq` 工具解析 JSON）:

```bash
#!/bin/bash

BASE_URL="http://localhost:3000"
TEST_EMAIL="test@example.com"

echo "=== 测试 1: 获取验证码 ==="
CAPTCHA_RESPONSE=$(curl -s -X GET "$BASE_URL/api/captcha/get")
echo "$CAPTCHA_RESPONSE" | jq .

CAPTCHA_ID=$(echo "$CAPTCHA_RESPONSE" | jq -r '.data.captcha_id')
echo "Captcha ID: $CAPTCHA_ID"

echo ""
echo "=== 测试 2: 验证验证码（错误的 X 坐标）==="
curl -s -X POST "$BASE_URL/api/captcha/verify" \
  -H "Content-Type: application/json" \
  -d "{\"captcha_id\": \"$CAPTCHA_ID\", \"x\": 0}" | jq .

echo ""
echo "=== 测试 3: 重新获取验证码 ==="
CAPTCHA_RESPONSE=$(curl -s -X GET "$BASE_URL/api/captcha/get")
CAPTCHA_ID=$(echo "$CAPTCHA_RESPONSE" | jq -r '.data.captcha_id')
echo "New Captcha ID: $CAPTCHA_ID"

echo ""
echo "=== 测试 4: 验证验证码（需要手动输入正确的 X 坐标）==="
echo "请查看验证码图片并输入正确的 X 坐标:"
read -p "X 坐标: " X_COORD

VERIFY_RESPONSE=$(curl -s -X POST "$BASE_URL/api/captcha/verify" \
  -H "Content-Type: application/json" \
  -d "{\"captcha_id\": \"$CAPTCHA_ID\", \"x\": $X_COORD}")
echo "$VERIFY_RESPONSE" | jq .

TOKEN=$(echo "$VERIFY_RESPONSE" | jq -r '.data.token')
echo "Token: $TOKEN"

if [ "$TOKEN" != "null" ] && [ -n "$TOKEN" ]; then
  echo ""
  echo "=== 测试 5: 使用 token 发送邮箱验证码 ==="
  curl -s -X GET "$BASE_URL/api/verification?email=$TEST_EMAIL&captcha_token=$TOKEN" | jq .

  echo ""
  echo "=== 测试 6: 尝试重复使用 token（应该失败）==="
  curl -s -X GET "$BASE_URL/api/verification?email=$TEST_EMAIL&captcha_token=$TOKEN" | jq .
else
  echo "验证失败，无法继续测试"
fi
```

---

## 边界测试用例

### 1. 过期测试

```bash
# 获取验证码
CAPTCHA_RESPONSE=$(curl -s -X GET "http://localhost:3000/api/captcha/get")
CAPTCHA_ID=$(echo "$CAPTCHA_RESPONSE" | jq -r '.data.captcha_id')

# 等待 2 分钟后验证（应该失败）
sleep 120

curl -X POST "http://localhost:3000/api/captcha/verify" \
  -H "Content-Type: application/json" \
  -d "{\"captcha_id\": \"$CAPTCHA_ID\", \"x\": 100}"
```

### 2. 重复验证测试

```bash
# 获取验证码并验证
CAPTCHA_RESPONSE=$(curl -s -X GET "http://localhost:3000/api/captcha/get")
CAPTCHA_ID=$(echo "$CAPTCHA_RESPONSE" | jq -r '.data.captcha_id')

# 第一次验证
curl -X POST "http://localhost:3000/api/captcha/verify" \
  -H "Content-Type: application/json" \
  -d "{\"captcha_id\": \"$CAPTCHA_ID\", \"x\": 100}"

# 第二次验证（应该失败）
curl -X POST "http://localhost:3000/api/captcha/verify" \
  -H "Content-Type: application/json" \
  -d "{\"captcha_id\": \"$CAPTCHA_ID\", \"x\": 100}"
```

### 3. 无效参数测试

```bash
# 缺少 captcha_id
curl -X POST "http://localhost:3000/api/captcha/verify" \
  -H "Content-Type: application/json" \
  -d '{"x": 100}'

# 缺少 x 坐标
curl -X POST "http://localhost:3000/api/captcha/verify" \
  -H "Content-Type: application/json" \
  -d '{"captcha_id": "test-id"}'

# 无效的 captcha_id
curl -X POST "http://localhost:3000/api/captcha/verify" \
  -H "Content-Type: application/json" \
  -d '{"captcha_id": "invalid-id", "x": 100}'
```

### 4. Token 重复使用测试

```bash
# 获取并验证验证码
CAPTCHA_RESPONSE=$(curl -s -X GET "http://localhost:3000/api/captcha/get")
CAPTCHA_ID=$(echo "$CAPTCHA_RESPONSE" | jq -r '.data.captcha_id')

VERIFY_RESPONSE=$(curl -s -X POST "http://localhost:3000/api/captcha/verify" \
  -H "Content-Type: application/json" \
  -d "{\"captcha_id\": \"$CAPTCHA_ID\", \"x\": 100}")
TOKEN=$(echo "$VERIFY_RESPONSE" | jq -r '.data.token')

# 第一次使用 token
curl -X GET "http://localhost:3000/api/verification?email=test@example.com&captcha_token=$TOKEN"

# 第二次使用 token（应该失败）
curl -X GET "http://localhost:3000/api/verification?email=test@example.com&captcha_token=$TOKEN"
```

---

## 预期行为总结

1. **验证码生成**: 每次调用应返回新的唯一验证码
2. **验证码验证**:
   - 正确的 X 坐标（±5px）应验证成功
   - 每个验证码只能验证一次
   - 验证码 2 分钟后过期
3. **Token 生成**: 验证成功后生成唯一 token
4. **Token 使用**:
   - Token 只能使用一次
   - Token 2 分钟后过期
   - 发送邮件时必须提供有效 token（如果 CAPTCHA 启用）

---

## 注意事项

1. 本测试需要服务器运行并且 CAPTCHA 功能已启用
2. 测试邮箱需要配置有效的 SMTP 服务器
3. 实际的 X 坐标需要根据生成的验证码图片确定
4. 建议使用前端界面进行完整的用户体验测试
5. 生产环境测试时注意不要触发速率限制

---

## 故障排查

### 问题: 验证码生成失败

**可能原因:**
- CAPTCHA 未初始化
- go-captcha 依赖未正确安装

**解决方法:**
- 检查服务器日志
- 确认 `common.InitCaptcha()` 已在启动时调用

### 问题: 验证总是失败

**可能原因:**
- X 坐标计算错误
- 容差设置过小

**解决方法:**
- 检查前端发送的 X 坐标值
- 确认容差设置为 ±5px

### 问题: Token 无效

**可能原因:**
- Token 已使用
- Token 已过期
- Token 存储失败

**解决方法:**
- 检查 token 是否在 2 分钟内使用
- 确认 token 未被重复使用
- 检查内存存储是否正常工作

---

## 前端集成测试

本节描述如何在前端界面测试 CAPTCHA 集成功能。

### 测试环境准备

1. 确保后端服务已启动并运行
2. 确保前端开发服务器已启动（通常在 `http://localhost:3001`）
3. 确保 CAPTCHA 功能已启用（`CAPTCHA_ENABLED=true`）
4. 清除浏览器缓存和 localStorage，确保获取最新的系统状态

### 测试场景 1: 用户注册 - 邮箱验证码

**测试步骤:**

1. 打开浏览器，访问注册页面 `/register`
2. 填写用户名、密码和邮箱地址
3. 点击"获取验证码"按钮
4. **验证点:** 应弹出滑动验证码弹窗
5. 拖动滑块到正确位置完成验证
6. **验证点:** 验证成功后弹窗自动关闭
7. **验证点:** 显示"验证码发送成功"的提示消息
8. **验证点:** "获取验证码"按钮变为禁用状态，显示倒计时（30秒）
9. 检查邮箱，确认收到验证码邮件
10. 输入收到的验证码
11. 完成注册流程

**预期结果:**
- 滑动验证码弹窗正常显示，背景图和滑块图清晰可见
- 滑动操作流畅，验证成功后自动关闭
- 邮件发送成功，验证码正确
- 倒计时功能正常，30秒后按钮重新启用

**错误场景测试:**
- 滑动到错误位置：应显示"验证失败，请重试"，弹窗不关闭
- 点击取消按钮：弹窗关闭，不发送邮件
- 验证成功后等待超过2分钟再尝试使用同一 token：应提示 token 过期

### 测试场景 2: 密码重置 - 发送重置邮件

**测试步骤:**

1. 打开浏览器，访问密码重置页面 `/reset`
2. 输入已注册的邮箱地址
3. 点击"提交"按钮
4. **验证点:** 应弹出滑动验证码弹窗
5. 拖动滑块到正确位置完成验证
6. **验证点:** 验证成功后弹窗自动关闭
7. **验证点:** 显示"重置邮件发送成功，请检查邮箱！"的提示消息
8. **验证点:** "提交"按钮变为禁用状态，显示倒计时（30秒）
9. 检查邮箱，确认收到密码重置邮件
10. 点击邮件中的重置链接
11. 完成密码重置流程

**预期结果:**
- 滑动验证码弹窗正常显示和工作
- 密码重置邮件成功发送
- 重置链接有效且可以正常使用
- 倒计时功能正常

**错误场景测试:**
- 输入未注册的邮箱：应提示"该邮箱地址未注册"
- 滑动验证失败：应显示错误提示，不发送邮件
- 点击取消按钮：弹窗关闭，不发送邮件

### 测试场景 3: CAPTCHA 禁用状态

**测试步骤:**

1. 在后端配置中设置 `CAPTCHA_ENABLED=false`
2. 重启后端服务
3. 清除浏览器 localStorage 缓存
4. 刷新前端页面
5. 尝试注册或密码重置流程
6. **验证点:** 不应显示滑动验证码弹窗
7. **验证点:** 直接发送邮件，无需验证

**预期结果:**
- CAPTCHA 弹窗不显示
- 邮件发送功能正常工作
- 用户体验流畅，无额外验证步骤

### 测试场景 4: 多次验证测试

**测试步骤:**

1. 在注册页面点击"获取验证码"
2. 完成滑动验证，发送验证码
3. 等待30秒倒计时结束
4. 再次点击"获取验证码"
5. **验证点:** 应再次弹出新的滑动验证码
6. 完成第二次验证
7. **验证点:** 应成功发送第二封邮件

**预期结果:**
- 每次点击都生成新的验证码
- 每次验证都是独立的
- 倒计时正常工作，防止频繁请求

### 测试场景 5: 浏览器兼容性测试

**测试浏览器:**
- Chrome (最新版本)
- Firefox (最新版本)
- Safari (最新版本)
- Edge (最新版本)

**测试步骤:**
1. 在每个浏览器中打开注册页面
2. 执行完整的注册流程，包括 CAPTCHA 验证
3. **验证点:** 滑动验证码在所有浏览器中正常显示和工作
4. **验证点:** 拖动操作流畅，无卡顿
5. **验证点:** 弹窗样式正常，无布局错乱

**预期结果:**
- 所有主流浏览器都能正常显示和使用 CAPTCHA
- 无明显的兼容性问题
- 用户体验一致

### 测试场景 6: 移动端响应式测试

**测试设备/模式:**
- 手机浏览器（iOS Safari, Android Chrome）
- 浏览器开发者工具的移动设备模拟模式

**测试步骤:**
1. 在移动设备或模拟器中打开注册页面
2. 点击"获取验证码"
3. **验证点:** CAPTCHA 弹窗适配移动屏幕尺寸
4. **验证点:** 滑块可以通过触摸操作拖动
5. 完成验证流程

**预期结果:**
- 弹窗在小屏幕上正常显示，无溢出
- 触摸拖动操作流畅
- 按钮和文字大小适合移动设备
- 整体布局响应式良好

### 测试场景 7: 网络异常处理

**测试步骤:**

1. 打开浏览器开发者工具，切换到 Network 标签
2. 在注册页面点击"获取验证码"
3. 在 CAPTCHA 弹窗中，使用开发者工具模拟网络断开
4. 完成滑动验证
5. **验证点:** 应显示网络错误提示
6. 恢复网络连接
7. 重新尝试验证
8. **验证点:** 应正常工作

**预期结果:**
- 网络错误时显示友好的错误提示
- 错误后可以重试
- 不会导致页面崩溃或卡死

### 测试场景 8: 性能测试

**测试步骤:**

1. 打开浏览器开发者工具，切换到 Performance 标签
2. 开始录制性能
3. 点击"获取验证码"，完成整个 CAPTCHA 流程
4. 停止录制
5. **验证点:** 检查 CAPTCHA 弹窗加载时间
6. **验证点:** 检查图片加载时间
7. **验证点:** 检查拖动操作的响应时间

**预期结果:**
- CAPTCHA 弹窗加载时间 < 500ms
- 图片加载时间 < 1s
- 拖动操作响应时间 < 100ms
- 无明显的性能瓶颈

### 前端测试检查清单

使用以下检查清单确保所有功能正常：

- [ ] 注册页面 CAPTCHA 集成正常
- [ ] 密码重置页面 CAPTCHA 集成正常
- [ ] CAPTCHA 弹窗正常显示和关闭
- [ ] 滑动验证功能正常工作
- [ ] 验证成功后正确发送邮件
- [ ] 验证失败时显示正确的错误提示
- [ ] 倒计时功能正常工作
- [ ] 取消按钮正常工作
- [ ] CAPTCHA 禁用时不显示弹窗
- [ ] 多次验证功能正常
- [ ] 浏览器兼容性良好
- [ ] 移动端响应式正常
- [ ] 网络异常处理正确
- [ ] 性能表现良好
- [ ] 无控制台错误或警告

### 自动化测试建议

虽然本次测试主要是手动测试，但建议在未来添加以下自动化测试：

1. **E2E 测试（使用 Playwright 或 Cypress）:**
   - 自动化注册流程测试
   - 自动化密码重置流程测试
   - CAPTCHA 弹窗显示和关闭测试

2. **单元测试（使用 Jest + React Testing Library）:**
   - CaptchaModal 组件测试
   - useSecureVerification hook 测试
   - 状态管理测试

3. **集成测试:**
   - API 调用测试
   - Token 验证流程测试
   - 错误处理测试

### 测试报告模板

完成测试后，建议记录以下信息：

```
测试日期: YYYY-MM-DD
测试人员: [姓名]
测试环境: [浏览器版本, 操作系统]
CAPTCHA 状态: 启用/禁用

测试结果:
- 注册页面: ✓ 通过 / ✗ 失败
- 密码重置页面: ✓ 通过 / ✗ 失败
- 浏览器兼容性: ✓ 通过 / ✗ 失败
- 移动端响应式: ✓ 通过 / ✗ 失败
- 性能测试: ✓ 通过 / ✗ 失败

发现的问题:
1. [问题描述]
2. [问题描述]

建议:
1. [改进建议]
2. [改进建议]
```

---

## 总结

本文档提供了完整的 CAPTCHA 功能测试指南，包括：
- API 端点的 curl 命令测试
- 前端界面的手动测试步骤
- 各种测试场景和边界情况
- 浏览器兼容性和移动端测试
- 性能和网络异常测试

通过遵循这些测试步骤，可以确保 CAPTCHA 功能在各种场景下都能正常工作，为用户提供安全可靠的验证体验。
