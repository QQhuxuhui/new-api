# 上游更新合并分析

**日期**: 2026-01-20
**分析人**: Claude Code
**项目场景**: 商业运营
**优先级**: 计费准确性 > 用户留存 > 渠道稳定性 > 成本控制
**使用渠道**: OpenAI, Anthropic, Gemini, 国内渠道（智普/Moonshot/MiniMax/Doubao）

---

## 📊 上游更新概况

- **共同祖先**: `de93fa5f`
- **上游新增提交**: 216 个
- **上游版本**: v0.10.6-alpha.3
- **当前分支**: dev
- **主要差异**: 我们有独立的 captcha 系统和 UI 优化

---

## 🎯 采纳建议

### ✅ 第一批：必须立即合并（计费修复）- P0

这些直接影响收入，必须优先处理：

#### 1. Anthropic 缓存计费错误修复
- **提交**: `0a2f12c0`
- **影响**: 修复前导致 94.5% 收入损失（实测数据：从 ¥0.02 → ¥0.38）
- **场景**: Anthropic 渠道 + `/v1/chat/completions` + 缓存
- **风险**: 低，只影响 Anthropic 渠道
- **文件**: `relay/compatible_handler.go`
- **优先级**: 🔴 P0 - 立即采纳

#### 2. Auto 分组 Task 不扣费修复
- **提交**: `aed19003`
- **影响**: 使用 auto 分组调用 Task 接口时不记录日志、不扣费
- **场景**: `/v1/videos`, `/suno`, `/kling` 等接口
- **风险**: 低，只影响 Task Relay
- **文件**: `relay/relay_task.go`
- **优先级**: 🔴 P0 - 立即采纳

#### 3. 智普/Moonshot 流式缓存统计修复
- **提交**: `ab81d6e4`
- **影响**: stream=true 时无法获取 cachePrompt 统计数据
- **场景**: 智普、Moonshot 渠道流式调用
- **风险**: 低，只影响这两个渠道
- **文件**: `relay/channel/openai/relay-openai.go`
- **优先级**: 🔴 P0 - 立即采纳

#### 4. WSS 预扣费 key 提取错误修复
- **提交**: `14c58aea`
- **影响**: 支持小写 bearer + 修复 WSS 预扣费错误提取 key
- **风险**: 低
- **优先级**: 🔴 P0 - 立即采纳

#### 5. 免费模型预扣费逻辑修复
- **提交**: `45556c96`
- **影响**: 免费模型基于分组倍率的预扣费逻辑
- **风险**: 低
- **优先级**: 🔴 P0 - 立即采纳

**预计收益**: 修复这些问题可能挽回 **10-20%** 的漏计费收入。

---

### ✅ 第二批：强烈推荐合并（渠道修复）- P1

#### Gemini 相关修复（5个）

| 提交 | 说明 | 文件 |
|------|------|------|
| `138fcd23` | 修复 propertyNames 清理 | gemini adaptor |
| `4ed4a765` | 支持 snake_case 字段 | gemini adaptor |
| `5ed4583c` | 修复文件类型不支持 image/jpg | gemini adaptor |
| `ddb40b1a` | 修复 gemini request -> openai tool call | gemini adaptor |
| `c2464fc8` | 使用原生 v1beta/models 端点 | gemini adaptor |

**优先级**: 🟡 P1 - 强烈推荐（你使用 Gemini）

#### Claude 相关修复

| 提交 | 说明 |
|------|------|
| `1d8a11b3` | 修复 Claude 模型调用标记问题 |

**优先级**: 🟡 P1 - 强烈推荐（你使用 Claude）

#### 国内渠道相关

| 提交 | 说明 |
|------|------|
| `b51d1e2f` | 移除 Minimax 从 FETCHABLE channels |
| `c975b4cf` | 添加 MiniMax-M2 系列模型 |
| `a4f28f0b` | 添加 Doubao video 1.5 |
| `67ba913b` | 添加 Doubao /v1/responses 支持 |

**优先级**: 🟡 P1 - 强烈推荐（你使用国内渠道）

---

### ✅ 第三批：用户留存功能 - P1

#### 签到功能

| 提交 | 说明 | 价值 |
|------|------|------|
| `8abfbe37` | 签到功能 + 配额奖励 | 提升用户活跃度和留存 |
| `c33ac97c` | 集成 Turnstile 安全检查 | 防止签到滥用 |

**优先级**: 🟡 P1 - 强烈推荐（符合你的用户留存需求）

**注意**: 需要前后端配合，可能需要额外配置 Turnstile。

---

### ✅ 第四批：安全与稳定性 - P2

| 提交 | 说明 | 价值 |
|------|------|------|
| `e13459f3` | 添加正则屏蔽 API keys | 安全性 |
| `a195e888` | 修复跨年时间显示 | 用户体验 |
| `a8f7c061` | 批量添加 key 去重 | 稳定性 |
| `c682e413` | CrossGroupRetry 默认 false | 稳定性 |
| `04ea79c4` | 支持 HTTP_PROXY 环境变量 | 灵活性 |

**优先级**: 🟢 P2 - 推荐

---

### ⚠️ 可选功能（按需采纳）- P3

| 提交 | 说明 | 建议 |
|------|------|------|
| `e5cb9ac0` | Codex 渠道支持 | 如果需要 Codex |
| `725d61c5` | IoNet 集成 | 如果需要 IoNet |
| `62b796fa` | Chat2Response 功能 | 如果需要格式转换 |
| `688280b3` | 修复 chat2response 设置 UI | 配合上一个 |
| `817da8d7` | 参数操作增强 | 可选 |

**优先级**: 🔵 P3 - 可选

---

## 🚀 实施方案

### 方案 A: 分批合并（推荐）⭐

**优点**:
- 风险可控
- 便于测试和回滚
- 每批都能快速验证效果

**缺点**:
- 需要多次合并操作
- 周期较长

**实施步骤**:

```bash
# 第一周：计费修复（P0）
git checkout -b merge-upstream-batch1
git cherry-pick 0a2f12c0  # Anthropic 缓存计费
git cherry-pick aed19003  # Auto 分组扣费
git cherry-pick ab81d6e4  # 智普/Moonshot 缓存
git cherry-pick 14c58aea  # WSS 预扣费
git cherry-pick 45556c96  # 免费模型预扣费
# 测试 → 合并到 dev → 上线

# 第二周：渠道修复（P1）
git checkout -b merge-upstream-batch2
git cherry-pick 138fcd23 4ed4a765 5ed4583c ddb40b1a c2464fc8  # Gemini
git cherry-pick 1d8a11b3  # Claude
git cherry-pick b51d1e2f c975b4cf a4f28f0b 67ba913b  # 国内渠道
# 测试 → 合并到 dev → 上线

# 第三周：签到功能（P1）
git checkout -b merge-upstream-batch3
git cherry-pick 8abfbe37 c33ac97c  # 签到功能
# 测试 → 合并到 dev → 上线

# 第四周：安全稳定性（P2）
git checkout -b merge-upstream-batch4
git cherry-pick e13459f3 a195e888 a8f7c061 c682e413 04ea79c4
# 测试 → 合并到 dev → 上线
```

---

### 方案 B: 一次性合并

**优点**:
- 快速获得所有更新
- 操作简单

**缺点**:
- 风险较高
- 冲突较多
- 难以定位问题

**实施步骤**:

```bash
# 创建合并分支
git checkout -b merge-upstream-all
git fetch upstream
git merge upstream/main

# 解决冲突（主要是 captcha 相关）
# 全面测试
# 合并到 dev
```

**不推荐原因**: 216 个提交一次性合并，风险太高。

---

### 方案 C: Cherry-pick 关键修复

**优点**:
- 最灵活
- 只拿需要的

**缺点**:
- 可能有依赖问题
- 需要仔细分析依赖关系

**实施步骤**:

```bash
# 只 cherry-pick P0 和 P1 的提交
git checkout -b merge-upstream-critical
git cherry-pick <P0 commits>
git cherry-pick <P1 commits>
# 测试 → 合并
```

---

## ⚠️ 潜在冲突分析

### 1. Captcha 系统
- **你的改动**: 完整的 captcha 功能（20+ 提交）
- **上游改动**: 无
- **冲突风险**: ✅ 无冲突

### 2. Dashboard/Charts
- **你的改动**: Token 使用图表、仪表盘优化
- **上游改动**: 可能有 UI 修复
- **冲突风险**: ⚠️ 小冲突（UI 层面）
- **解决方案**: 保留你的改动，手动合并上游 UI 修复

### 3. Semi-UI CSS
- **你的改动**: `fix(web): avoid semi-ui css deep import`
- **上游改动**: 可能有相关改动
- **冲突风险**: ⚠️ 小冲突
- **解决方案**: 保留你的修复

### 4. 计费逻辑
- **你的改动**: 无重大改动
- **上游改动**: 多个计费修复
- **冲突风险**: ✅ 无冲突
- **建议**: 直接采纳上游修复

---

## 📋 测试清单

### 第一批（计费修复）测试

- [ ] Anthropic 渠道 + 缓存调用，检查计费是否正确
- [ ] Auto 分组 + Task 接口（/v1/videos），检查是否扣费和记录日志
- [ ] 智普/Moonshot 流式调用 + 缓存，检查统计数据
- [ ] WSS 接口预扣费测试
- [ ] 免费模型调用测试

### 第二批（渠道修复）测试

- [ ] Gemini 各种调用场景（tool call, 文件上传等）
- [ ] Claude 调用测试
- [ ] MiniMax-M2 模型测试
- [ ] Doubao video 1.5 测试

### 第三批（签到功能）测试

- [ ] 签到功能前端显示
- [ ] 签到奖励配额发放
- [ ] Turnstile 安全检查
- [ ] 防止重复签到

### 第四批（安全稳定性）测试

- [ ] API key 屏蔽测试
- [ ] 跨年时间显示测试
- [ ] 批量添加 key 去重测试
- [ ] HTTP_PROXY 环境变量测试

---

## 📊 预期收益

### 计费修复收益（第一批）
- **Anthropic 缓存计费**: 挽回 94.5% 收入损失
- **Auto 分组扣费**: 挽回视频/音乐接口漏计费
- **流式缓存统计**: 挽回缓存 tokens 漏计费
- **预计总收益**: 10-20% 收入提升

### 用户留存收益（第三批）
- **签到功能**: 预计提升 15-30% 用户活跃度
- **配额奖励**: 增加用户粘性

### 稳定性收益（第二批 + 第四批）
- **渠道修复**: 减少 API 调用失败率
- **安全增强**: 降低 API key 泄露风险

---

## 🎯 推荐方案

**最终推荐**: 方案 A（分批合并）

**理由**:
1. 你是商业运营，稳定性优先
2. 计费修复影响收入，必须快速验证
3. 分批合并便于回滚和定位问题
4. 每批都能快速看到效果

**时间线**:
- 第 1 周: 计费修复（P0）→ 预计挽回 10-20% 收入
- 第 2 周: 渠道修复（P1）→ 提升稳定性
- 第 3 周: 签到功能（P1）→ 提升用户留存
- 第 4 周: 安全稳定性（P2）→ 长期优化

---

## 📝 后续行动

1. **立即开始**: 第一批计费修复（P0）
2. **准备测试环境**: 确保有完整的测试覆盖
3. **监控指标**: 合并后密切监控计费数据
4. **用户沟通**: 签到功能上线前做好用户通知

---

## 📚 参考资料

- 上游仓库: https://github.com/Calcium-Ion/new-api
- 上游版本: v0.10.6-alpha.3
- 共同祖先: de93fa5f
- 新增提交数: 216

---

**文档版本**: 1.0
**最后更新**: 2026-01-20
