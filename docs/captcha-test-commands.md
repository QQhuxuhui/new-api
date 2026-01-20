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
