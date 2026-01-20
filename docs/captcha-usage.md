# 滑动验证码功能使用指南

## 功能概述

本系统集成了基于 go-captcha 的滑动验证码功能，用于防止恶意批量注册和自动化攻击。滑动验证码在以下场景中启用：

- 用户注册时发送邮箱验证码
- 密码重置时发送重置邮件

### 核心特性

- **滑动验证**: 用户需要拖动滑块到正确位置完成人机验证
- **一次性 Token**: 验证成功后生成的 token 只能使用一次
- **一次性验证码**: 邮箱验证码在使用后立即失效，防止重复使用
- **自动过期**: 验证码和 token 均有 2 分钟的有效期
- **内存存储**: 使用高效的内存存储，支持自动清理过期数据

## 配置方法

### 后端配置

#### 1. 环境变量

在 `.env` 文件或系统环境变量中配置：

```bash
# 启用/禁用滑动验证码（默认启用）
CAPTCHA_ENABLED=true
```

#### 2. 代码配置

在 `common/constants.go` 中可以修改默认配置：

```go
var (
    CaptchaEnabled = true  // 是否启用滑动验证码
)
```

#### 3. 系统初始化

确保在 `main.go` 中已调用 CAPTCHA 初始化函数：

```go
// 初始化 CAPTCHA
if err := common.InitCaptcha(); err != nil {
    common.FatalLog(fmt.Sprintf("failed to initialize captcha: %v", err))
}
```

### 前端配置

前端会自动从 `/api/status` 接口获取 CAPTCHA 配置，无需手动配置。

## API 接口说明

### 1. 获取滑动验证码

**端点**: `GET /api/captcha/get`

**描述**: 获取一个新的滑动验证码，包含背景图片、滑块图片和滑块 Y 坐标。

**请求参数**: 无

**响应示例**:

```json
{
  "success": true,
  "message": "",
  "data": {
    "captcha_id": "550e8400-e29b-41d4-a716-446655440000",
    "background_image": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA...",
    "slider_image": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA...",
    "slider_y": 123
  }
}
```

**字段说明**:

- `captcha_id`: 验证码唯一标识符（UUID）
- `background_image`: 背景图片（base64 编码的 PNG 图片）
- `slider_image`: 滑块图片（base64 编码的 PNG 图片）
- `slider_y`: 滑块的 Y 坐标（整数）

### 2. 验证滑动结果

**端点**: `POST /api/captcha/verify`

**描述**: 验证用户滑动的 X 坐标是否正确，成功后返回一次性 token。

**请求参数**:

```json
{
  "captcha_id": "550e8400-e29b-41d4-a716-446655440000",
  "x": 150
}
```

**参数说明**:

- `captcha_id`: 从获取接口返回的验证码 ID
- `x`: 用户滑动的 X 坐标（整数）

**成功响应**:

```json
{
  "success": true,
  "message": "验证成功",
  "data": {
    "captcha_token": "7c9e6679-7425-40de-944b-e07fc1f90ae7"
  }
}
```

**失败响应**:

```json
{
  "success": false,
  "message": "验证失败，请重试"
}
```

**注意事项**:

- 每个 `captcha_id` 只能验证一次
- 验证允许 ±5px 的误差范围
- 验证码有效期为 2 分钟

### 3. 发送邮箱验证码（需要 CAPTCHA Token）

**端点**: `GET /api/verification`

**描述**: 使用 CAPTCHA token 发送邮箱验证码。

**请求参数**:

- `email`: 邮箱地址（必填）
- `captcha_token`: CAPTCHA 验证 token（必填）

**请求示例**:

```bash
GET /api/verification?email=user@example.com&captcha_token=7c9e6679-7425-40de-944b-e07fc1f90ae7
```

**成功响应**:

```json
{
  "success": true,
  "message": "验证码发送成功，有效期为10分钟，请注意查收"
}
```

**失败响应**:

```json
{
  "success": false,
  "message": "验证码token无效或已过期，请重新验证"
}
```

**注意事项**:

- Token 只能使用一次
- Token 有效期为 2 分钟
- 邮箱验证码有效期为 10 分钟
- 邮箱验证码只能使用一次

### 4. 密码重置（需要 CAPTCHA Token）

**端点**: `POST /api/user/reset`

**描述**: 发送密码重置邮件，需要先完成 CAPTCHA 验证。

**请求参数**:

```json
{
  "email": "user@example.com",
  "captcha_token": "7c9e6679-7425-40de-944b-e07fc1f90ae7"
}
```

**响应格式**: 与邮箱验证码接口相同

## 使用流程

### 用户注册流程

```
1. 用户填写注册信息（用户名、邮箱、密码）
   ↓
2. 用户点击"获取验证码"按钮
   ↓
3. 前端调用 GET /api/captcha/get 获取验证码
   ↓
4. 显示滑动验证码弹窗
   ↓
5. 用户拖动滑块到正确位置
   ↓
6. 前端调用 POST /api/captcha/verify 验证坐标
   ↓
7. 验证成功，获得 captcha_token
   ↓
8. 前端调用 GET /api/verification?email=xxx&captcha_token=xxx
   ↓
9. 后端验证 token，发送邮箱验证码
   ↓
10. 用户输入收到的验证码
   ↓
11. 提交注册表单
   ↓
12. 后端验证邮箱验证码（一次性使用）
   ↓
13. 注册成功
```

### 密码重置流程

```
1. 用户访问密码重置页面
   ↓
2. 用户输入邮箱地址
   ↓
3. 用户点击"提交"按钮
   ↓
4. 前端调用 GET /api/captcha/get 获取验证码
   ↓
5. 显示滑动验证码弹窗
   ↓
6. 用户拖动滑块到正确位置
   ↓
7. 前端调用 POST /api/captcha/verify 验证坐标
   ↓
8. 验证成功，获得 captcha_token
   ↓
9. 前端调用 POST /api/user/reset 发送重置邮件
   ↓
10. 用户收到重置邮件，点击链接
   ↓
11. 完成密码重置
```

## 前端集成示例

### 使用 CaptchaModal 组件

```jsx
import React, { useState } from 'react';
import CaptchaModal from '../common/CaptchaModal';
import { API, showSuccess, showError } from '../../helpers';

const MyComponent = () => {
  const [showCaptcha, setShowCaptcha] = useState(false);
  const [email, setEmail] = useState('');

  // 点击获取验证码按钮
  const handleGetCode = () => {
    if (!email) {
      showError('请先输入邮箱地址');
      return;
    }
    setShowCaptcha(true);
  };

  // CAPTCHA 验证成功回调
  const handleCaptchaSuccess = async (token) => {
    setShowCaptcha(false);

    try {
      const res = await API.get(
        `/api/verification?email=${email}&captcha_token=${token}`
      );

      if (res.data.success) {
        showSuccess('验证码发送成功');
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError('发送验证码失败');
    }
  };

  return (
    <div>
      <input
        type="email"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        placeholder="请输入邮箱"
      />
      <button onClick={handleGetCode}>获取验证码</button>

      <CaptchaModal
        visible={showCaptcha}
        onSuccess={handleCaptchaSuccess}
        onCancel={() => setShowCaptcha(false)}
      />
    </div>
  );
};

export default MyComponent;
```

## 故障排查

### 问题 1: 验证码生成失败

**症状**: 调用 `/api/captcha/get` 返回错误或 500 状态码

**可能原因**:

1. CAPTCHA 未正确初始化
2. go-captcha 依赖未正确安装
3. 资源文件加载失败

**解决方法**:

1. 检查服务器启动日志，确认 `InitCaptcha()` 是否成功执行
2. 运行 `go mod tidy` 确保依赖已安装
3. 检查 go-captcha-assets 资源包是否正确安装
4. 查看详细错误日志：`tail -f logs/error.log`

### 问题 2: 验证总是失败

**症状**: 拖动滑块到正确位置，但验证仍然失败

**可能原因**:

1. 前端发送的 X 坐标计算错误
2. 容差设置过小
3. 验证码已过期（超过 2 分钟）

**解决方法**:

1. 在浏览器开发者工具中检查发送的 X 坐标值
2. 确认容差设置为 ±5px（在 `common/captcha.go` 中）
3. 确保在 2 分钟内完成验证
4. 检查后端日志查看具体错误信息

### 问题 3: Token 无效或已过期

**症状**: 使用 token 发送邮件时提示 token 无效

**可能原因**:

1. Token 已被使用（一次性 token）
2. Token 已过期（超过 2 分钟）
3. Token 存储失败
4. 服务器重启导致内存数据丢失

**解决方法**:

1. 确保 token 在 2 分钟内使用
2. 确认 token 未被重复使用
3. 检查内存存储是否正常工作
4. 如果是多实例部署，考虑使用 Redis 存储（未来版本支持）
5. 重新完成 CAPTCHA 验证获取新 token

### 问题 4: 邮箱验证码无法使用

**症状**: 输入正确的验证码但提示验证失败

**可能原因**:

1. 验证码已被使用（一次性验证码）
2. 验证码已过期（超过 10 分钟）
3. 验证码输入错误

**解决方法**:

1. 确认验证码在 10 分钟内使用
2. 确认验证码未被重复使用
3. 仔细核对验证码（区分大小写）
4. 重新获取新的验证码

### 问题 5: 前端弹窗不显示

**症状**: 点击"获取验证码"按钮，CAPTCHA 弹窗不显示

**可能原因**:

1. CaptchaModal 组件未正确导入
2. 状态管理错误
3. 前端依赖未安装

**解决方法**:

1. 检查 CaptchaModal 组件是否正确导入
2. 检查 `showCaptcha` 状态是否正确设置
3. 运行 `npm install` 确保依赖已安装
4. 检查浏览器控制台是否有错误信息
5. 确认 `@wenlng/go-captcha-react` 包已安装

### 问题 6: 图片加载失败

**症状**: CAPTCHA 弹窗显示，但图片不显示或显示错误

**可能原因**:

1. Base64 图片数据损坏
2. 图片格式不支持
3. 网络传输错误

**解决方法**:

1. 检查 API 响应中的图片数据是否完整
2. 确认图片数据以 `data:image/png;base64,` 开头
3. 检查网络请求是否成功
4. 尝试刷新验证码

### 问题 7: 内存占用过高

**症状**: 服务器内存占用持续增长

**可能原因**:

1. 过期数据未及时清理
2. 大量验证码请求导致数据积累
3. 内存泄漏

**解决方法**:

1. 检查清理机制是否正常工作
2. 调整清理频率（在 `common/captcha_store.go` 中）
3. 监控验证码和 token 的数量
4. 考虑添加最大存储数量限制
5. 如果是生产环境，考虑使用 Redis 存储

### 问题 8: 并发请求导致错误

**症状**: 高并发情况下出现验证错误或数据不一致

**可能原因**:

1. 锁竞争导致性能问题
2. 数据竞态条件
3. 多实例部署时数据不同步

**解决方法**:

1. 检查锁的使用是否正确
2. 确认读写锁（RWMutex）正确使用
3. 对于多实例部署，必须使用 Redis 存储（未来版本支持）
4. 进行压力测试，找出性能瓶颈

## 安全建议

### 1. 防止暴力破解

- 限制同一 IP 的验证码获取频率
- 限制验证失败次数
- 记录异常行为并进行分析

### 2. 防止 Token 泄露

- 使用 HTTPS 传输
- Token 只能使用一次
- Token 有效期尽量短（当前为 2 分钟）

### 3. 防止验证码重用

- 验证码验证后立即删除
- 邮箱验证码使用后立即删除
- 定期清理过期数据

### 4. 监控和日志

- 记录所有验证码生成和验证操作
- 监控异常验证行为
- 定期分析日志，发现攻击模式

## 性能优化建议

### 1. 图片优化

- 调整图片质量参数，减小文件大小
- 考虑使用 JPEG 格式替代 PNG
- 预生成一批验证码图片，减少实时生成压力

### 2. 存储优化

- 定期清理过期数据
- 设置最大存储数量限制
- 考虑使用 Redis 替代内存存储（多实例部署必须）

### 3. 并发优化

- 使用读写锁（RWMutex）提高并发性能
- 减少锁的持有时间
- 考虑使用无锁数据结构

### 4. 缓存优化

- 缓存验证码图片资源
- 使用 CDN 加速图片传输
- 启用浏览器缓存

## 未来改进方向

1. **Redis 支持**: 支持 Redis 存储，适用于多实例部署
2. **多种验证码类型**: 支持旋转验证码、点选验证码等
3. **难度配置**: 支持动态调整验证码难度
4. **行为分析**: 基于用户行为的风险评分
5. **统计报表**: 验证码使用情况统计和分析
6. **国际化**: 支持多语言提示信息

## 相关文档

- [CAPTCHA 集成设计方案](/docs/plans/2026-01-19-captcha-integration-design.md)
- [CAPTCHA 实施计划](/docs/plans/2026-01-19-captcha-integration-implementation.md)
- [CAPTCHA API 测试命令](/docs/captcha-test-commands.md)

## 技术支持

如遇到问题，请：

1. 查看本文档的故障排查部分
2. 检查服务器日志文件
3. 查看浏览器控制台错误信息
4. 提交 Issue 到项目仓库
