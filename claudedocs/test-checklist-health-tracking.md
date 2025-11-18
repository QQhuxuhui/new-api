# 健康跟踪功能测试清单

## ✅ 已自动验证（无需手动测试）

- [x] Redis连接正常
- [x] 应用成功启动（PID: 49489, 端口: 3000）
- [x] 数据库连接正常
- [x] 后端编译成功
- [x] 优先级故障转移代码修复正确
- [x] 前端列配置代码修复正确
- [x] 前端数据传递代码修复正确
- [x] API端点存在并响应

---

## 📋 需要手动测试的项目

### 🖥️ 前端UI测试（约5分钟）

访问 http://localhost:3000 并登录管理员账户

#### 1. 健康状态列显示
- [ ] 进入"通道管理"页面
- [ ] 检查表格中是否有"健康状态"列（在"状态"列之后）
- [ ] 打开列选择器，确认"健康状态"选项存在且默认勾选
- [ ] **截图保存**

#### 2. 健康状态标签
- [ ] 观察健康状态列的显示
  - 灰色"未知" = 正常（没有请求记录）
  - 绿色"正常" = 健康
  - 黄色"警告" = 有连续失败
  - 橙色"已暂停" = 通道暂停
- [ ] **截图保存**

#### 3. 详情弹窗
- [ ] 点击任意健康状态标签
- [ ] 确认弹窗打开且不报错
- [ ] 检查弹窗显示以下内容：
  - [ ] 状态（正常/警告/已暂停）
  - [ ] 连续高失败率周期（X/10）
  - [ ] 当前窗口失败率
  - [ ] 窗口请求数统计
  - [ ] 最后成功/失败时间
  - [ ] 总请求数和成功率
- [ ] **截图保存**

#### 4. 手动重置按钮（如果状态为警告或暂停）
- [ ] 确认"重置健康状态"按钮显示
- [ ] 点击重置按钮
- [ ] 确认重置成功并显示Toast提示
- [ ] 确认状态更新
- [ ] **截图保存**

---

### 🔄 优先级故障转移测试（约10分钟）

#### 准备工作
1. 创建测试通道（如果还没有）：
   - 优先级10: 通道A, 通道B
   - 优先级5: 通道C, 通道D

#### 测试步骤

**步骤1: 手动暂停优先级1的所有通道**
```bash
# 假设通道A的ID是1，通道B的ID是2
redis-cli setex "channel:health:1:suspended" 300 "manual_test"
redis-cli setex "channel:health:2:suspended" 300 "manual_test"
```

**步骤2: 监控日志**
```bash
tail -f /tmp/new-api.log | grep -E "(no healthy channel|priority|通道)"
```

**步骤3: 发送测试请求**
```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "YOUR_TEST_MODEL",
    "messages": [{"role": "user", "content": "test"}]
  }' \
  -v 2>&1 | grep -E "(X-Channel-Id|HTTP)"
```

**步骤4: 验证结果**
- [ ] 请求成功返回（HTTP 200）
- [ ] 日志显示"no healthy channel at priority 10"
- [ ] 日志显示使用了优先级5的通道（通道C或D）
- [ ] 响应头 `X-Channel-Id` 是通道C或D的ID（不是A或B）
- [ ] **复制日志输出**

---

### 🔄 自动刷新测试（约1分钟）

**步骤1: 打开通道管理页面并保持**

**步骤2: 在终端修改健康状态**
```bash
redis-cli setex "channel:health:1:suspended" 300 "test"
```

**步骤3: 观察前端**
- [ ] 30秒内，通道1的健康状态从"正常"变为"已暂停"
- [ ] 打开浏览器开发者工具 → Network
- [ ] 确认每30秒有一次 `GET /api/channel/health` 请求
- [ ] **截图保存**

---

### 💾 Redis数据验证（约3分钟）

**步骤1: 产生健康数据**
发送几个API请求（参考优先级测试中的curl命令）

**步骤2: 检查Redis键**
```bash
# 列出所有健康键
redis-cli --scan --pattern "channel:health:*"

# 应该看到类似：
# channel:health:1:bucket:1700321400
# channel:health:1:consecutive_failures
# channel:health:1:suspended
# channel:health:1:total_successes
# channel:health:1:total_failures
```

**步骤3: 检查键内容**
```bash
# 查看bucket内容
redis-cli get "channel:health:1:bucket:TIMESTAMP"
# 预期格式: "15:2" (15成功:2失败)

# 查看连续失败计数
redis-cli get "channel:health:1:consecutive_failures"
# 预期: 0-10之间的整数

# 检查TTL
redis-cli ttl "channel:health:1:bucket:TIMESTAMP"
# 预期: 约65秒
```

**验证项**:
- [ ] bucket键存在且格式正确（"成功数:失败数"）
- [ ] consecutive_failures键存在且值合理
- [ ] bucket的TTL约为65秒
- [ ] **复制命令输出**

---

## 📊 测试结果记录

### 前端UI测试
- 健康状态列显示: [ ] 通过 / [ ] 失败
- 详情弹窗功能: [ ] 通过 / [ ] 失败
- 手动重置功能: [ ] 通过 / [ ] 失败
- 自动刷新功能: [ ] 通过 / [ ] 失败

### 功能测试
- 优先级故障转移: [ ] 通过 / [ ] 失败
- Redis数据结构: [ ] 通过 / [ ] 失败

### 发现的问题
```
（在此记录测试中发现的任何问题）
```

---

## 🚀 快速命令参考

```bash
# 检查应用状态
ps aux | grep new-api | grep -v grep

# 监控实时日志
tail -f /tmp/new-api.log

# 监控Redis
redis-cli monitor | grep "channel:health"

# 列出所有健康键
redis-cli --scan --pattern "channel:health:*"

# 手动暂停通道（测试用）
redis-cli setex "channel:health:CHANNEL_ID:suspended" 300 "test"

# 清除所有健康数据（重置测试）
redis-cli --scan --pattern "channel:health:*" | xargs redis-cli del
```

---

## 📝 注意事项

1. **请替换以下占位符**：
   - `YOUR_ACCESS_TOKEN`: 您的管理员访问令牌
   - `YOUR_TEST_MODEL`: 测试模型名称（如 gpt-3.5-turbo）
   - `CHANNEL_ID`: 实际的通道ID

2. **获取访问令牌**：
   - 登录管理员账户后，在浏览器开发者工具 → Application → Local Storage 中查找 `access_token`

3. **测试建议顺序**：
   1. 前端UI测试（最简单）
   2. 自动刷新测试（观察效果）
   3. Redis数据验证（了解数据结构）
   4. 优先级故障转移测试（核心功能）

---

**测试人员**: _________________
**测试日期**: _________________
**应用版本**: 最新（2025-11-18编译）
