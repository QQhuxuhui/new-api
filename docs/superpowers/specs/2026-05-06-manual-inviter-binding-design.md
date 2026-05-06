# 手动绑定邀请关系（Manual Inviter Binding）设计文档

- 日期：2026-05-06
- 作者：GGbond
- 状态：设计已确认，待实现

## 背景与目标

平台目前支持用户通过邀请码（aff_code）注册时自动建立邀请关系：被邀请用户的 `users.inviter_id` 记录邀请人，邀请人获得 `AffCount`、`AffQuota` 等奖励。

实际运营中，代理用户介绍来的客户经常**未通过其邀请码完成注册**（手输地址、看错链接等），导致邀请关系丢失。需要管理员能够**手动**把用户 A 的邀请人指定为用户 B，作为代理激励的归属依据。

本次只解决"绑定关系"本身，不附带任何额度奖励发放。后续若有基于 `inviter_id` 的分润/统计逻辑，可独立实现。

## 范围与非目标

**范围**

- 管理员（含 root）可在用户管理页面把任意用户 A 的 `inviter_id` 修改为另一个用户 B
- `inviter_id = 0` 表示解除当前绑定
- 操作支持覆盖原邀请人，需二次确认
- 服务端做环路检测、权限层级校验、审计日志

**非目标**

- 不发放任何 `AffQuota`、`AffHistoryQuota`、`AffCount`、`QuotaForInvitee` 奖励
- 不做批量绑定（一次操作只针对一个 A）
- 不做邀请关系树状可视化
- 不修改前端用户表格的"邀请信息"列展示逻辑（已显示 `inviter_id`）

## 当前实现摘要

- 数据：`users.inviter_id INT INDEX`（`model/user.go:43`）；新用户注册时由 `User.Insert(inviterId)` 写入（`model/user.go:377-438`）
- 普通编辑接口 `PUT /api/user/`（`controller/user.go:610` `UpdateUser` → `model/user.go:482` `Edit()`）只白名单更新 `username/display_name/group/quota/remark/max_concurrency`，**不会**修改 `inviter_id`
- 管理员 admin 路由组：`router/api-router.go:126-151` `userRoute.Group("/")` + `AdminAuth`
- 用户搜索：`GET /api/user/search?keyword=xxx`（`controller/user.go:301` `SearchUsers`），admin 路由
- 审计日志：`RecordLog(userId, LogTypeManage, content)`（`model/log.go:49`），现有"管理员将用户额度从 X 修改为 Y" 即此模式

## 总体设计

新增一个独立 admin 端点，在事务中完成校验 + 写库 + 审计日志 + 缓存失效。前端在用户表格的"更多"菜单加入口，弹出 Modal 进行操作。

```
┌─────────────────────────┐    POST /api/user/manage/inviter
│  Admin UI (User Table)  │ ───────────────────────────────────►  ┌──────────────────────┐
│  More Menu → 设置邀请人  │    { user_id, inviter_id }            │  controller/user.go  │
│  └─ SetInviterModal     │                                       │  SetUserInviter      │
└─────────────────────────┘                                       └──────────┬───────────┘
                                                                             │
                                                            ┌────────────────┴───────────────┐
                                                            │  权限层级校验（复用 UpdateUser）│
                                                            │  ↓                              │
                                                            │  model.SetUserInviter (事务)    │
                                                            │  ├─ FOR UPDATE 锁 A             │
                                                            │  ├─ 校验 B 存在                 │
                                                            │  ├─ detectInviterCycle (≤50)    │
                                                            │  ├─ UPDATE inviter_id           │
                                                            │  ├─ RecordLog(LogTypeManage)    │
                                                            │  └─ invalidateUserCache(A)      │
                                                            └────────────────────────────────┘
```

## 后端设计

### 路由

`router/api-router.go` 内 `adminRoute` 组新增：

```go
adminRoute.POST("/manage/inviter", controller.SetUserInviter)
```

### Controller：`controller.SetUserInviter`

文件：`controller/user.go`

请求体：

```go
type SetUserInviterRequest struct {
    UserId    int `json:"user_id" binding:"required,min=1"`
    InviterId int `json:"inviter_id" binding:"min=0"` // 0 = 解绑
}
```

流程：

1. 解码 + 字段校验。`UserId == InviterId` 直接返回错误："不能将用户自己设为邀请人"
2. 加载 A：`model.GetUserById(req.UserId, false)`，不存在返回 404 风格错误
3. 权限层级校验（复用 `UpdateUser:636-642` 的逻辑）：
   - `myRole := c.GetInt("role")`
   - `if myRole <= A.Role && myRole != common.RoleRootUser` → 返回"无权操作权限等级 ≥ 自己的用户"
4. 调用 `model.SetUserInviter(req.UserId, req.InviterId, c.GetInt("id"))`
5. 返回 `{ success: true, data: { previous_inviter_id, new_inviter_id } }`

### Model：`model.SetUserInviter`

文件：`model/user.go`

签名：

```go
func SetUserInviter(userId, inviterId, operatorId int) (previous int, err error)
```

伪代码：

```go
tx := DB.Begin()
defer func() { if err != nil { tx.Rollback() } }()

var a User
if err = tx.Set("gorm:query_option", "FOR UPDATE").
    First(&a, userId).Error; err != nil {
    return 0, err
}
previous = a.InviterId

if previous == inviterId {
    return previous, tx.Commit().Error  // 幂等：无变更不写日志
}

var newInviterName string
if inviterId != 0 {
    var b User
    if err = tx.Select("id, username").First(&b, inviterId).Error; err != nil {
        return previous, fmt.Errorf("邀请人用户不存在: %w", err)
    }
    newInviterName = b.Username
    if err = detectInviterCycle(userId, inviterId, tx); err != nil {
        return previous, err
    }
}

if err = tx.Model(&User{}).Where("id = ?", userId).
    Update("inviter_id", inviterId).Error; err != nil {
    return previous, err
}

if err = tx.Commit().Error; err != nil {
    return previous, err
}

content := buildInviterChangeLog(operatorId, previous, inviterId, newInviterName)
RecordLog(userId, LogTypeManage, content)
_ = invalidateUserCache(userId)
return previous, nil
```

> `RecordLog` 走独立的 `LOG_DB` 连接（`model/log.go:83-95`），与主事务无关。在 `tx.Commit()` 之后调用，与现有"管理员修改额度"日志（`controller/user.go:659`）的写入时机保持一致。

### 环路检测：`detectInviterCycle`

文件：`model/user.go`

```go
func detectInviterCycle(targetUserId, newInviterId int, tx *gorm.DB) error {
    if newInviterId == 0 {
        return nil
    }
    if newInviterId == targetUserId {
        return errors.New("不能将用户自己设为邀请人")
    }
    visited := make(map[int]bool)
    cur := newInviterId
    for depth := 0; depth < 50; depth++ {
        if cur == 0 {
            return nil
        }
        if cur == targetUserId {
            return errors.New("检测到邀请关系环路：目标邀请人的上线链中已包含该用户")
        }
        if visited[cur] {
            return errors.New("检测到已存在的邀请关系环路（数据异常），请联系开发处理")
        }
        visited[cur] = true

        var next int
        if err := tx.Model(&User{}).Select("inviter_id").
            Where("id = ?", cur).Scan(&next).Error; err != nil {
            return err
        }
        cur = next
    }
    return errors.New("邀请关系链路过深（>50），疑似数据异常")
}
```

### 审计日志文案

`buildInviterChangeLog(operatorId, previous, newId, newName)`：

| 场景 | 文案 |
|---|---|
| `previous == 0, newId != 0` | `管理员（#{op}）将邀请人设为 用户 #{newId}（{newName}）` |
| `previous != 0, newId != 0, previous != newId` | `管理员（#{op}）将邀请人由 用户 #{previous} 修改为 用户 #{newId}（{newName}）` |
| `previous != 0, newId == 0` | `管理员（#{op}）解除了邀请人绑定（原邀请人 #{previous}）` |
| `previous == newId` | 不写日志（幂等） |

## 前端设计

### 入口：`web/src/components/table/users/UsersColumnDefs.jsx`

`moreMenu` 数组中插入一项（位置：`套餐管理` 之后、`重置 Passkey` 之前），并在前后加 `divider`：

```jsx
{ node: 'item', name: t('设置邀请人'), onClick: () => showSetInviterModal(record) },
```

回调由父组件（`web/src/pages/User/index.jsx` 或对应的 users 表格容器）通过 props 注入。

### Modal：`web/src/components/table/users/modals/SetInviterModal.jsx`

Props：`{ visible, user, onClose, onSuccess }`

布局：

1. 顶部信息块（只读）
   - `用户：#{user.id} {user.username} ({user.display_name})`
   - `当前邀请人：#{user.inviter_id}` 或 `无邀请人`（`inviter_id === 0`）
2. `Select`（Semi UI），`filter` + 远程搜索
   - `onSearch(keyword)` debounce 300ms → `GET /api/user/search?keyword={keyword}`
   - 选项渲染：`#{id} {username} ({display_name}) — {email}`
   - placeholder：`搜索用户（用户名/邮箱/ID）`
   - 帮助文字：`留空则解除当前邀请人绑定`
3. 底部按钮：`取消` / `确认`

行为：

- 点击 `确认`：
  - 若 `user.inviter_id !== 0` 且选中的新值 `!== user.inviter_id`（覆盖场景），先弹 `Modal.warning` 二次确认：`用户 #{A} 当前的邀请人是 #{X}，将被替换为 #{Y}。此操作会写入审计日志，不可撤销。是否继续？`
  - 调用 `setUserInviter(user.id, selectedInviterId)` → `POST /api/user/manage/inviter`
  - 成功：`Toast.success('设置成功')` → `onSuccess()` 关闭 Modal 并刷新表格
  - 失败：直接展示后端 `message`（环路、权限、用户不存在等）
- 自己绑自己（前端拦截）：`selectedInviterId === user.id` 时 `确认` 按钮禁用

### API 客户端

按现有 axios/fetch 封装风格在 `web/src/helpers/api.js`（或对应文件）新增：

```js
export const setUserInviter = (userId, inviterId) =>
  API.post('/api/user/manage/inviter', { user_id: userId, inviter_id: inviterId });
```

### i18n 新增 key

| zh | en |
|---|---|
| 设置邀请人 | Set Inviter |
| 当前邀请人 | Current Inviter |
| 无邀请人 | No Inviter（已存在）|
| 搜索用户（用户名/邮箱/ID） | Search user (username / email / ID) |
| 留空则解除当前邀请人绑定 | Leave empty to remove the current inviter binding |
| 用户 #{A} 当前的邀请人是 #{X}，将被替换为 #{Y}。此操作会写入审计日志，不可撤销。是否继续？ | User #{A} currently has inviter #{X}, which will be replaced with #{Y}. This action is logged and cannot be undone. Continue? |
| 检测到邀请关系环路 | Inviter relationship cycle detected |
| 设置成功 | Updated successfully（如已存在则复用）|

## 错误处理

| 场景 | HTTP/响应 | 文案 |
|---|---|---|
| `user_id == inviter_id` | `success: false` | `不能将用户自己设为邀请人` |
| A 不存在 | `success: false` | `用户不存在` |
| B 不存在 | `success: false` | `邀请人用户不存在` |
| 操作者权限 ≤ A 的角色（非 root） | `success: false` | `无权操作权限等级 ≥ 自己的用户` |
| 检测到环路 | `success: false` | `检测到邀请关系环路：目标邀请人的上线链中已包含该用户` |
| 数据异常环路（visited 命中） | `success: false` | `检测到已存在的邀请关系环路（数据异常），请联系开发处理` |
| 链路 > 50 层 | `success: false` | `邀请关系链路过深（>50），疑似数据异常` |
| DB 错误 | `common.ApiError(c, err)` | 透传错误 |

## 并发与一致性

- 在事务内对 A 行 `SELECT … FOR UPDATE`，避免并发改 A 的 `inviter_id` 时检测旧链而写新值
- 环路检测使用同一个事务句柄读取上线链，看到的是事务隔离级别下的一致快照
- B 不需要加锁：B 的 `inviter_id` 改动不会影响"以 B 为根的上线链是否包含 A"这一判定的正确性（如果在校验后 B 的链发生变更，最坏情况是后续操作中再次触发环路检测拒绝；不会产生死锁循环写入）
- 提交事务后调用 `invalidateUserCache(userId)` 让用户缓存失效

## 测试要点

后端单元测试（建议放在 `model/user_inviter_test.go` 新文件）：

1. 正常路径：A inviter_id 0 → B
2. 覆盖路径：A inviter_id X → Y，previous 返回正确
3. 解绑路径：A inviter_id X → 0
4. 幂等：A inviter_id X → X，无日志写入，无错误
5. 自绑：A → A，返回错误
6. B 不存在：返回错误
7. 环路 1 跳：B.inviter_id == A，拒绝
8. 环路 N 跳：A ← B ← C，把 C.inviter_id 设为 A，拒绝
9. 数据异常环路（手工构造现有环），visited 双保险触发
10. 链路 > 50 层 fail safe（构造 51 层链或者 mock）

Controller 层 e2e/集成测试：

11. 非 admin 调用：401/403
12. 普通管理员操作另一个普通管理员（同级）：拒绝
13. Root 操作管理员：通过

前端手工验证：

14. 操作菜单看到"设置邀请人"
15. 搜索框可工作（输入用户名/邮箱/ID）
16. 覆盖场景看到二次确认弹窗
17. 设置成功后表格 inviter_id 列即时刷新
18. 后端报错时 Toast 显示中文错误

## 实施清单

后端：

- [ ] `model/user.go`：新增 `SetUserInviter`、`detectInviterCycle`、`buildInviterChangeLog`
- [ ] `controller/user.go`：新增 `SetUserInviterRequest` 和 `SetUserInviter` handler
- [ ] `router/api-router.go`：注册 `POST /api/user/manage/inviter`
- [ ] `model/user_inviter_test.go`：新增单元测试

前端：

- [ ] `web/src/components/table/users/modals/SetInviterModal.jsx`：新建
- [ ] `web/src/components/table/users/UsersColumnDefs.jsx`：菜单项 + props 透传
- [ ] `web/src/pages/User/index.jsx`（或 users 表格容器）：注入 `showSetInviterModal` 回调，挂载 Modal
- [ ] `web/src/helpers/api.js`（或对应文件）：`setUserInviter` API 客户端
- [ ] `web/src/i18n/locales/zh.json` + `en.json`：新增文案

## 风险与回滚

- **风险**：环路检测如果实现错误可能误判正常链路。缓解：单元测试覆盖 1/N 跳、空链、自绑、深链所有边界
- **风险**：极端场景下事务提交成功但 `RecordLog`（异步走 `LOG_DB`）写日志失败，会出现 inviter_id 已变但日志缺失。缓解：与现有"管理员修改额度"路径一致，运维上可接受；如需强一致可后续改为同库写入
- **回滚**：纯增量改动，没有 DB schema 迁移；删除 4 个新增文件 + revert 3 个改动文件即可

## 参考实现位点

- `controller/user.go:610-666`：`UpdateUser` 的权限层级校验模式
- `model/user.go:340-375`：`TransferAffQuotaToQuota` 的事务 + FOR UPDATE 模式
- `controller/user.go:301-315`：`SearchUsers`（前端搜索接口）
- `web/src/components/table/users/UsersColumnDefs.jsx:235-263`：现有 `moreMenu` 结构
