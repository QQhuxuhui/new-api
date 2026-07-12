# 我的套餐页重设计 · 设计文档

日期:2026-07-11
状态:待用户审阅

## 背景

`web/src/pages/MyPlans/index.jsx`(约 1260 行)是普通用户查看自己套餐的页面。现状问题:

1. **多套餐不可扫视**:除当前套餐外,所有套餐(含已失效的)都渲染成约 400px 高的全宽大卡片单列堆叠,持有 N 个套餐就要滚动 N 屏;失效套餐与可用套餐混排,只靠一个到期标签区分。
2. **排队套餐重复出现**:既作为大卡片出现在主列表,又出现在独立的"排队套餐"区块和「查看队列」弹窗里。
3. **切换按钮形同虚设**:「切换到此套餐」「锁定/解锁」按钮及后端接口(`POST /api/my_plans/switch`、`POST /api/my_plans/:id/lock|unlock`)已完整存在。套餐模板 `Plan.DefaultAllowSwitch` 自 2026-02 起创建默认已是允许(`gorm:"default:1"`,model/plan.go:33),但**存量** `user_plans.allow_user_switch` 大多为 0,普通用户在旧套餐上只能看到「暂不可手动切换」。
4. **按套餐禁止切换形同虚设**:切换权限校验是"当前**或**目标任一允许即放行"(service/plan_selector.go:324-335),一旦多数套餐放开,管理员对单个套餐的禁止无法被服务端强制。
5. **手动切换会被系统覆盖**:当前套餐 `auto_switch=1` 时,选择器会在下一次请求时自动升级到更高优先级套餐(plan_selector.go:161-178),用户的手动选择被静默覆盖。而 `auto_switch` 同时门控三种行为:①有额度时自动升级;②额度耗尽时的救援切换(plan_selector.go:117-159);③跨套餐渠道故障转移(plan_failover.go:283-286)。直接关掉它防①会连带失去②③。

## 目标

1. 让「设为当前」(切换)和「锁定」对普通用户真正可见可用,且管理员按套餐禁止切换能被服务端强制。
2. 手动切换的选择不被自动升级覆盖,同时保留额度耗尽救援与故障转移。
3. 重设计页面布局,几十个套餐也能快速扫清状态。

## 非目标(明确不做)

- 退款流程:重写时**删除**现有以 `false &&` 禁用的退款死代码及退款 Modal/handler(后端接口保留),后续启用时重加,不平移死代码。
- 批量锁定/解锁、用户自助调整队列顺序。
- 允许锁定"当前使用中"的套餐(后端继续拒绝;须先切走)。
- 除「后端改动」一节所列各项外,不改动任何扣费/选择器/故障转移逻辑。
- trial 种子模板的 `DefaultAllowSwitch=0` 保持不动(该模板默认停用)。

## 决策记录

| 决策 | 结论 |
|---|---|
| 切换权限 | 存量数据迁移放开 + 服务端校验改为**仅看目标套餐**,使管理员按套餐禁止真正生效 |
| 手动切换保护 | 新增 `pinned` 标记:手动切换只禁止"自动升级",保留耗尽救援与故障转移;不改 `auto_switch` 值 |
| 布局方案 | 方案 A「分组卡片墙」:当前套餐大卡 + 按状态分区的紧凑卡片网格,失效区默认折叠 |
| 锁定语义 | 保持现状:锁定 = 冻结额度,被扣费/自动切换/故障转移全部跳过 |
| 发布/回退 | 迁移不可回退(执行前建议备份 `user_plans`/`plans` 表);`pinned` 行为不加开关——用户可随时通过再次切换或「解除」按钮解除 |

## 一、后端改动

后端共四处改动,其余全部复用现有接口(列表、切换、锁定、解锁、auto_switch、quota-status、billing-status)。

### 1.1 存量切换权限迁移

- 新建套餐模板与购买播种(service/plan_delivery.go、AssignPlanToUser、AddPlanToQueue)默认已是允许,**无需改默认值**;本项只处理存量。
- **迁移方式**:启动时一次性执行,仿照 `model/main.go` 的 `clearMasqueradeLegacyFlags` 模式(options 表标记,如 `UserPlanAllowSwitchBackfilled`,查到即跳过,完成后写标记;挂在 migrateDB 启动钩子)。
  - `user_plans` SET `allow_user_switch=1` WHERE `status=1`(含排队中;status 2–6 已失效行不动);
  - `plans` SET `default_allow_switch=1` WHERE `type != 'trial'`(以 `PlanTypeTrial` 常量为准,排除试用类模板)。
  - **必须用一次性标记**而非数据自查式幂等(model/plan_migration.go 那种):回填后,管理员重新禁止产生的 0 与"未迁移"的 0 在数据上不可区分,条件式重跑会覆盖管理员决定。
- **已知取舍**:迁移会把历史上管理员显式禁止的套餐一并放开(数据上无法区分),管理员可事后通过 `PUT /api/user_plan/:id/permissions` 重新禁止。

### 1.2 切换权限校验改为仅看目标套餐

- `UserSwitchPlanByUserPlanId`(plan_selector.go:324-335)中"当前 OR 目标任一允许"的规则改为**仅校验目标套餐** `targetUserPlan.CanUserSwitch()`(含未锁定判断;拒绝排队目标等其余校验不变)。
- 效果:目标套餐被管理员禁止(`allow_user_switch=0`)时服务端真正拒绝;前端按目标 `can_switch=0` 置灰的提示与服务端行为一致。
- **顺带清理死代码**:`service.UserSwitchPlan`(plan_selector.go:254-292)无任何生产调用方(路由 `POST /api/my_plans/switch` 走的是 `controller.UserSwitchPlan → service.UserSwitchPlanByUserPlanId`),且内部调用已弃用的 `model.SwitchUserCurrentPlan`——实现时直接删除,避免 pinned/权限改动漏改这条僵尸路径。

### 1.3 手动指定(pinned)

- `user_plans` 新增列 `pinned`(int,0/1,`gorm:"default:0"`,AutoMigrate 自动加列,UserPlan 已在 model/main.go 注册)。
- **写入(`model.SwitchToUserPlan`,model/user_plan.go:602-657)**:增加参数 `setPinned bool`,同一事务内两处配合:
  - clear 步骤(:620-623,现状只清 `is_current` 一列)改为同时置 `pinned=0`;
  - 目标行 updates **无条件**写 `pinned = setPinned ? 1 : 0`(不能只在 true 时写,否则救援切换不会清标)。
  - **必须同事务**,不接受"切换成功后追加一次写"(存在切换成功但置标失败、下一请求即被升级覆盖的窗口,直接违反验收标准 2)。
- **调用方**:签名变更,全部调用点逐处更新;仅用户切换端点(plan_selector.go:338,`UserSwitchPlanByUserPlanId`)传 true,**其余一律 false**:plan_selector.go:104(初始选择)、:126/:142(耗尽救援)、:168(自动升级)、pre_consume_quota.go:162/:574(计费预扣重选/排队提升)、billing_priority.go:377、plan_failover.go:326、distributor.go:261/:475/:1186(故障转移)、controller/user_plan.go:237(管理员 force_switch 的 user_plan_id 分支)。
- **两条不经过 SwitchToUserPlan 的写路径同样要处理**:
  - `model.SwitchUserCurrentPlan`(model/user_plan.go:531-596,弃用但仍被管理员 force_switch 的 plan_id 兼容分支调用,controller/user_plan.go:240):clear 步骤同样清 `pinned`,目标行写 `pinned=0`(该路径永远非用户切换);
  - 队列激活 `activateNextQueuedPlanWithTx`(model/user_plan.go:1553-1605)有独立的激活 updates(:1580-1591),需显式写 `pinned=0`;`CompleteUserPlanIfDepleted` 的 demote updates(:1654-1661)顺带清 `pinned`,避免已完结套餐在详情弹窗里残留"手动指定"脏状态。
  - 以上叠加后,"任何一次非用户切换后该用户所有活跃行 `pinned=0`"才成立,同时阻断已确认的泄漏链(clear 不清标 → 残留 pinned 行被 `recalculateQueuePositionsWithTx` 重新编队 → 队列激活推成当前,pinned 静默生效)。
- **选择器**:自动升级分支(plan_selector.go:161-178)增加条件 `currentPlan.Pinned != 1`;耗尽救援分支(:117-159,含总额度与日限两种耗尽)与故障转移**不改**——pinned 套餐额度用尽仍按 `auto_switch` 语义自动切换救援(救援切换本身会清掉 pinned)。
- **解除指定**:`PUT /api/my_plans/:id/auto_switch` 请求 `enabled=true` 时同时清除该套餐 `pinned`(幂等)。**UI 可达性**:pinned 不改 `auto_switch` 的值(通常仍为开),仅靠"开启自动切换"无法解除——因此当前套餐大卡的「手动指定」标签旁提供**「解除」按钮**,点击即调用该接口(enabled=true);若该套餐自动切换原为关,解除会一并把它打开,语义即"恢复系统自动调度",tooltip 中说明。
- **缓存(已知陷阱)**:`UserPlanCacheEntry` 未包含新字段,必须同步在 model/user_plan_cache.go 的结构体与 `ToUserPlan`/`FromUserPlan` 中加 `Pinned`(两个缓存 key 共用该结构),否则热路径选择器读到的永远是 0。缓存失效复用 `SwitchToUserPlan` 既有的 `InvalidateUserPlanCache`。
- **DTO**:`UserPlanResponse` 增加 `pinned` 字段供前端展示。
- `auto_switch` 的值与语义完全不变(仍控制耗尽救援、故障转移,及未 pinned 时的自动升级)。
- 切换成功 Toast:「已切换到该套餐。系统不会自动更换你的选择;额度用尽或渠道故障时仍会自动处理。」

### 1.4 修复套餐模板创建的 GORM 默认值陷阱

`Plan.DefaultAllowSwitch` 带 `gorm:"default:1"`,创建模板时若管理员取消勾选(提交 0),GORM 零值省略会被数据库默认值回填成 1,导致"创建时禁止"静默失效。在创建路径显式写入该列(显式 Select 或指针),保证 0 能存进去。1.2 使"按套餐禁止"成为服务端强制手段后,这个陷阱必须堵上。

## 二、前端改动

### 2.1 页面信息架构(自上而下,即最终渲染顺序)

1. **页头**:标题 + 副标题 + 刷新按钮。**删除**现有 200px 渐变横幅(连同其外链纹理图)与三列速览统计卡(当前套餐/剩余总额度/套餐状态)——信息并入当前套餐大卡;页头文字从白色改为适配普通背景的常规配色。
2. **当前套餐大卡**(唯一保留全部细节的卡):名称、类型/优先级标签、`pinned=1` 时显示「手动指定」标签及旁边的**「解除」按钮**(tooltip 说明:系统不会自动升级更换;额度用尽或故障仍自动处理;点击「解除」恢复自动调度,会一并开启自动切换)、总额度进度条、今日额度进度条、限流提示 Banner、自动切换开关(**可操作的开关全页仅此一处**)。
3. **今日日卡池卡**(billing-status.daily_pool 存在时显示;内容保留现状,位置由现状"套餐列表之前"移到当前套餐大卡之后)。
4. **可用套餐**:紧凑卡网格,桌面三列 / 平板两列 / 手机一列。
5. **排队中**:紧凑卡,显示队列位置、"前面套餐用完后自动激活"、`estimated_activation_time>0` 时显示预计激活日期。未锁定的排队套餐显示在此;被锁定的排队套餐按 2.2 归入「已锁定」分区(叠加队列标签)。删除现有独立排队区块与「查看队列」弹窗,排队信息不再重复出现。
6. **已锁定**:紧凑卡,区分"你已锁定"(可解锁)与"管理员锁定"(显示原因,不可自行解锁)。
7. **已失效(默认折叠)**:折叠条显示分类计数(已过期/已停用/已用完/已作废/已回收),展开后灰显、无操作按钮。
8. **按量付费钱包卡**(保留现状,含充值入口与 recharge_disabled 隐藏逻辑)。
9. **页脚免责声明**(保留现状)。

空分区自动隐藏。完全无套餐时显示空状态并**新增**「去购买」按钮跳转 `/plans`(现状空态无任何 CTA)。

**各分区排序**:可用与已锁定按 `plan_priority DESC, id ASC`(与后端选择器一致,DTO 已含 plan_priority);排队中按 `queue_position ASC`;已失效按 `expires_at DESC`(最近失效在前,`expires_at=0` 排最后,同值按 `id DESC`)。

### 2.2 分组判定(前端,消费现有 DTO 字段 + 新增 pinned)

按以下优先级归组,命中即止:

1. `is_current=1` → 当前;
2. `status ∈ {2,3,4,5,6}` **或**(`status=1` 且 `expires_at>0` 且 `expires_at≤now`)→ 已失效。分类标签映射:2=已过期、3=已停用、4=已用完、5=已作废、6=已回收;`status=1` 但已到期的伪过期行(异步过期任务未跑到的窗口)计入"已过期";
3. `locked=1` → 已锁定(若同时 `queue_position>0`,卡上叠加「队列 #N」标签并提示"锁定期间不会被自动激活"——后端激活确实跳过锁定行);
4. `queue_position>0` 且 `started_at=0` → 排队中(现有前端只看 queue_position,此处按后端约定收紧);
5. 其余(status=1)→ 可用。

### 2.3 紧凑卡片结构

- 内容:套餐名(超长省略号)、类型标签、剩余额度进度条 + 数字、到期时间(临期警示色;未激活套餐标注"切换后开始计时")、操作区。
- 操作区按钮:
  - **设为当前**:`!is_current && locked=0 && 非排队 && status=1 && 未过期` 时显示;目标 `can_switch=0` 时置灰并提示「管理员已禁止切换」(1.2 改后与服务端行为一致)。带 Popconfirm,沿用现有文案(「确认切换到此套餐?/切换后将使用此套餐的额度和渠道配置」)。
  - **锁定**:在现有前端三条件(非当前、未锁定、未排队)基础上**新增** `status=1` 且未过期判断,与后端 `LockUserPlanIfEligible` 的原子条件对齐。带 Popconfirm,沿用现有文案。
  - **解锁**:仅"你已锁定"时显示;保持现状**无确认弹层**,点击即解锁。
- 点击卡片任意位置打开**详情弹窗**:完整额度数字、已用/剩余、优先级、有效期与起止时间、每日限额、锁定信息(锁定方、原因)、管理员备注、预计激活时间(排队套餐)、`pinned` 与 `auto_switch` 状态(**只读展示**,可操作开关仅在当前套餐大卡)。已失效卡同样可点开详情(弹窗内无操作项)。移动端使用 Semi Modal 自适应宽度(近全屏)。
- 服务端始终是最终校验方:任何操作失败,前端统一 Toast 后端返回的 message 并自动重拉列表,**不做错误文案字符串匹配**(锁定/解锁竞态返回固定中文「套餐状态已变更,请刷新后重试」,切换失败返回英文校验错误,文案不统一)。

### 2.4 组件拆分

现有 index.jsx 单文件约 1260 行且桌面/手机两套按钮块完全重复,随布局重写一并拆分:

```
web/src/pages/MyPlans/
  index.jsx              页面壳:数据加载、分组、分区编排
  components/
    CurrentPlanHero.jsx  当前套餐大卡
    PlanSection.jsx      分区标题 + 网格容器
    CompactPlanCard.jsx  紧凑卡(单套响应式实现,不再桌面/移动两份)
    PlanDetailModal.jsx  详情弹窗
    ExpiredPlansFold.jsx 已失效折叠区
    WalletCard.jsx       按量付费钱包卡(现有 JSX 平移)
    DailyPoolCard.jsx    日卡池卡(现有 JSX 平移)
```

- 样式沿用现有 Tailwind + Semi-UI 组合及 PlanPricing 页的卡片设计语言。
- i18n 沿用 react-i18next 中文字面量 key 模式;**新增文案需同步补充 `web/src/i18n/locales/` 下各语言文件**(en/ja/fr/ru 等)。
- 单位约定:`expires_at` 为毫秒时间戳,`daily_reset_time` 为秒时间戳。

## 三、数据流与错误处理

- 加载:沿用现有三接口(`GET /api/my_plans/`、`/quota-status`、`/billing-status`)。
- 操作后刷新:切换/锁定/解锁/auto_switch 成功后重拉三接口,但**不再整页 Spin**——发起操作的卡片按钮显示 loading,其余区域保持可见。
- 竞态处理:见 2.3 末条,统一 Toast 后端 message + 自动重拉。
- 缓存:lock/unlock/switch/auto_switch 现有路径均已内部调用 `InvalidateUserPlanCache`;`pinned` 写入在 `SwitchToUserPlan` 事务内,复用其失效逻辑;auto_switch 端点清 pinned 复用 `ToggleUserPlanAutoSwitch` 所在路径的失效逻辑。

## 四、测试

- **后端**(仿照 `service/plan_user_lock_test.go` 模式):
  - 用户切换成功 → 目标 `pinned=1`,原当前套餐 `pinned=0`;下一次 `SelectPlanForRequest` 在存在更高优先级可用套餐、`auto_switch=1` 时**不**自动升级;
  - 耗尽救援仍生效:pinned 套餐总额度用尽 → 完结并切换,新当前套餐 `pinned=0`;日限用尽(总额度仍有)→ 同样救援切换;
  - force_switch(user_plan_id 与 plan_id 两个分支)/ 自动切换 / 队列激活(`activateNextQueuedPlanWithTx`)后,被激活或被指定的当前套餐 `pinned=0`;
  - `PUT auto_switch enabled=true` 清除 pinned 且开启自动切换(幂等);
  - 权限:目标 `allow_user_switch=0` → 服务端拒绝(即使当前套餐允许);目标为 1 → 放行;
  - 迁移:options 标记存在时不重跑;status 2–6 行不被改动;`type='trial'` 模板不被改动;执行两次结果一致;
  - 缓存:`UserPlanCacheEntry` 往返(FromUserPlan→ToUserPlan)后 `Pinned` 保留。
- **前端**:`npm run build`(web/ 目录)通过;手动核对状态组合清单(当前/手动指定/可用/可用但禁切/排队/排队且锁定/用户锁定/管理员锁定/各类失效/status=1 但已过期/未激活/临期/超长名称/空分区/完全无套餐/移动端)。

## 五、验收标准

1. 普通用户在可用套餐上能看到并成功使用「设为当前」「锁定」;解锁只对自己锁定的套餐可用。
2. 手动切换后,系统不会将用户的选择**自动升级**为其他套餐;总额度或日限用尽时仍按 `auto_switch` 语义自动救援切换;渠道故障转移不受影响(救援/故障切换后手动指定自动解除)。
3. 1080p 桌面视口下,无需滚动即可看到当前套餐大卡与可用分区首行;可用分区紧凑卡密度不低于每屏 9 张(三列三行);失效套餐默认折叠不占视野。
4. 排队套餐在页面上只出现一处。
5. 管理员显式禁止切换的套餐:按钮置灰且有原因提示,服务端同样拒绝。
