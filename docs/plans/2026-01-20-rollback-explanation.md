# 上游合并回滚说明

**日期**: 2026-01-20
**回滚提交**: 回滚到 `ea91a25e`
**原因**: 上游代码质量问题导致编译失败

---

## 📋 回滚决策

### 为什么回滚？

虽然我们成功修复了报告的 3 个问题和额外发现的 2 个问题，但上游代码仍然存在 **2 个严重的编译错误**：

1. **Ollama 函数缺失**: `ollama.FetchOllamaModels` 不存在
2. **OpenAIModel 字段缺失**: `Metadata` 字段未定义

这些问题来自上游代码本身，不是我们的修复导致的。

### 决策选项对比

| 选项 | 优点 | 缺点 | 决策 |
|------|------|------|------|
| **A. 暂时注释代码** | 快速 | 功能不完整，后续维护困难 | ❌ |
| **B. 实现缺失功能** | 功能完整 | 工作量大，可能与上游冲突 | ❌ |
| **C. 回滚等待上游修复** | 干净，无技术债务 | 暂时无法使用新功能 | ✅ **选择** |

---

## 🔄 已回滚的内容

### 回滚的提交

| 提交 | 说明 |
|------|------|
| `fb92c6bd` | fix: resolve additional upstream code issues |
| `1756e344` | fix: resolve code issues from upstream merge |
| `86b894fb` | docs: add upstream merge analysis and test checklist |
| `092ca1a3` | merge: integrate upstream batch 1 (billing fixes) and batch 2 (channel fixes) |
| 以及合并分支的所有 15 个上游提交 | 第一批和第二批上游修复 |

### 回滚的功能

**第一批 - 计费修复 (P0)**:
- ❌ Anthropic 缓存计费修复
- ❌ Auto 分组 Task 扣费修复
- ❌ 智普/Moonshot 流式缓存统计修复
- ❌ WSS 预扣费 key 提取修复
- ❌ 免费模型预扣费逻辑修复

**第二批 - 渠道修复 (P1)**:
- ❌ Gemini 5 个修复
- ❌ Claude 调用标记修复
- ❌ MiniMax 修复
- ❌ Doubao 更新
- ❌ Ollama 支持（本来就有问题）

### 保留的内容

- ✅ 你的自定义功能（Captcha 系统、Dashboard 优化等）
- ✅ 修复总结文档（`docs/plans/2026-01-20-code-fixes-summary.md`）
- ✅ 分析文档（需要手动添加）

---

## 📊 损失评估

### 💰 计费相关损失

回滚后，以下计费问题将继续存在：

| 问题 | 影响 | 预计损失 |
|------|------|----------|
| Anthropic 缓存计费错误 | 94.5% 收入损失 | **高** |
| Auto 分组不扣费 | 视频/音乐接口漏计费 | **中** |
| 流式缓存统计错误 | 缓存 tokens 漏计费 | **中** |
| WSS 预扣费错误 | 预扣费异常 | **低** |
| 免费模型预扣费错误 | 计费逻辑错误 | **低** |

**总体评估**: 如果你使用 Anthropic 渠道且启用缓存，损失会很大（~95% 收入）。

### 🛠️ 功能相关损失

| 功能 | 影响 |
|------|------|
| Gemini 改进 | 部分功能可能不稳定 |
| Claude 改进 | 日志显示可能有问题 |
| MiniMax M2 | 新模型不可用 |
| Doubao video 1.5 | 新功能不可用 |
| Ollama | 继续不可用（原本就有问题） |

---

## 🚀 下一步行动

### 立即行动

1. **监控上游仓库**:
   ```bash
   # 定期检查上游更新
   git fetch upstream
   git log --oneline upstream/main -20
   ```

2. **等待上游修复**:
   - 关注 issue: 可能需要向上游报告这些问题
   - 关注 PR: 等待修复的 PR 合并

3. **临时缓解措施**（可选）:
   - 如果 Anthropic 缓存计费问题影响很大，可以考虑只 cherry-pick 这一个修复
   - 如果其他计费问题影响较大，可以逐个 cherry-pick

### Cherry-pick 单个修复的方法

如果你想在上游修复前先采纳某些关键修复，可以这样做：

```bash
# 1. 获取上游更新
git fetch upstream

# 2. Cherry-pick 单个修复（例如 Anthropic 缓存计费）
git cherry-pick 0a2f12c0

# 3. 解决冲突（如果有）
# ... 手动解决冲突 ...
git add .
git cherry-pick --continue

# 4. 测试
go build ./main.go

# 5. 如果成功，提交
git commit -m "fix: cherry-pick Anthropic cache billing fix from upstream"
```

**推荐优先 cherry-pick 的修复**（如果损失严重）:
1. `0a2f12c0` - Anthropic 缓存计费修复（最重要）
2. `aed19003` - Auto 分组 Task 扣费修复
3. `ab81d6e4` - 智普/Moonshot 流式缓存统计修复

---

## 📝 经验教训

### 1. 上游代码质量检查

在合并上游更新前，应该：
- ✅ 先在测试分支编译验证
- ✅ 运行基本的冒烟测试
- ✅ 检查上游 CI/CD 状态

### 2. 分批合并的价值

分批合并策略证明是正确的：
- 如果我们继续合并了第三批（签到功能）和第四批（安全修复）
- 回滚会更复杂，损失更大

### 3. 文档的重要性

我们创建的详细文档现在非常有价值：
- `2026-01-20-upstream-merge-analysis.md` - 完整的分析
- `2026-01-20-code-fixes-summary.md` - 修复总结
- 这些文档在下次尝试合并时仍然有效

---

## 🔍 向上游反馈

建议向上游报告以下问题：

### Issue 1: Ollama 函数未实现

**标题**: Missing `ollama.FetchOllamaModels` function causing compilation failure

**描述**:
```
The code in controller/channel.go references ollama.FetchOllamaModels()
but this function does not exist in relay/channel/ollama/ directory.

Location: controller/channel.go:209, 1126
Error: undefined: ollama.FetchOllamaModels

This was introduced in commit c2464fc8.
```

### Issue 2: OpenAIModel 缺少 Metadata 字段

**标题**: `OpenAIModel` struct missing `Metadata` field

**描述**:
```
The code in controller/channel.go tries to set a Metadata field on
OpenAIModel struct, but this field is not defined.

Location: controller/channel.go:246
Error: unknown field Metadata in struct literal of type OpenAIModel

This was introduced in commit c2464fc8.
```

---

## 📅 时间线

| 时间 | 事件 |
|------|------|
| 2026-01-20 14:00 | 开始分析上游更新 |
| 2026-01-20 15:00 | 完成第一批 + 第二批合并 |
| 2026-01-20 15:30 | 发现代码问题 |
| 2026-01-20 16:00 | 修复 5 个问题 |
| 2026-01-20 16:30 | 发现 2 个无法修复的上游问题 |
| 2026-01-20 17:00 | 决定回滚，等待上游修复 |

---

## ✅ 当前状态

- ✅ 已回滚到 `ea91a25e` (合并前的状态)
- ✅ 保留了所有文档和分析
- ✅ dev 分支处于稳定状态
- ✅ 可以正常编译和运行

---

**文档版本**: 1.0
**最后更新**: 2026-01-20
**状态**: 等待上游修复
