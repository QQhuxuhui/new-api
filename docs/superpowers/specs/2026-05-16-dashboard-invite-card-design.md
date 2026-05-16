# 数据看板「邀请奖励」卡片设计稿

**日期**：2026-05-16
**作者**：GGbond + Claude Opus 4.7
**状态**：设计完成，待实现

---

## 背景

近期上线的「邀请有奖 / 一级分销返佣」（commit 系列 `feat(aff): *`、`feat(inviter-reward): *`）目前仅在 `/console/topup`（钱包管理）页面右侧的 `InvitationCard` 中展示。看板（dashboard）作为登录后的首屏，没有任何邀请入口，导致功能被深度隐藏，用户感知低。

本设计在 dashboard 顶部新增一张「邀请奖励」卡片，与现有「订阅套餐」「账户钱包」两张卡片**并列**，作为高曝光入口，承担两个目标：

1. **数据可见**：展示邀请人数、返佣明细等关键指标
2. **营销转化**：用醒目文案传达"邀请有 token 返佣、token 自由"的卖点，引导用户复制邀请链接

---

## 范围

**包含**：
- `web/src/components/dashboard/StatsCards.jsx`：栅格改为 2:2:1（条件回退 2 列）
- 新建 `web/src/components/dashboard/AffiliateRewardCard.jsx`
- i18n 新增中英文键
- 接口复用：`GET /api/user/aff/summary`、`GET /api/user/aff`（均已存在）

**不包含**：
- 任何后端改动（接口已具备所需字段）
- `InvitationCard.jsx`（钱包页那张完整版）保留不动
- 移动端推送 / 站内信等营销机制
- 邀请详情新弹窗（点 "查看返佣明细" 跳现有 `/console/topup` 即可）

---

## 关键决策

| 维度 | 选择 | 备选 / 理由 |
|---|---|---|
| 栅格布局 | 2:2:1（subscription / account / invite）| 备选 1:1:1 三等分；2:2:1 保留主卡空间，邀请卡作为右侧窄列符合"右上角"诉求 |
| 内容方向 | 行动+紧凑型：双 KPI 大卡 + KV 明细 + 单主 CTA | 备选「营销大字 hero」「三 KPI 平铺」；窄列下"行动+紧凑"信息密度最佳 |
| 主 CTA 行为 | 原地复制邀请链接 + toast | 备选「跳转钱包页」「弹抽屉」；原地复制路径最短 |
| 空数据态 | 始终显示，0 数据走"邀请第一位好友"鼓励文案 | 起到对新用户的营销价值 |
| 冻结态 | 显示卡片 + 顶部 warning Banner + CTA 置灰 | 与现有 `InvitationCard` 一致 |
| `reward_percent === 0` | 父层不渲染整卡，栅格退回 2 列 | 管理员关闭返佣时静默消失，无空槽位 |
| 数据获取 | StatsCards 父组件 fetch，props 透传 | 子组件 fetch 会与 InvitationCard 重复请求；父组件更便于条件渲染 |
| 容器元素 | `<div>` 自定义而非 Semi `<Card>` | 避开全局 `.semi-card-body { padding: 10px !important }` 压制 |
| 颜色方案 | Semi 主题 CSS 变量（`--semi-color-success-*`） | 避免 Tailwind 默认色阶（已被 `theme.colors` 替换不生效）+ 自动适配深色模式 |

---

## 布局规格

### 第一行栅格

`web/src/components/dashboard/StatsCards.jsx:294` 处改造：

```
当前：
  <div className="grid gap-4 grid-cols-1 lg:grid-cols-2">
    {订阅卡}
    {账户卡}
  </div>

改为：
  <div className={`grid gap-4 grid-cols-1 ${showAffSlot ? 'lg:grid-cols-5' : 'lg:grid-cols-2'}`}>
    <div className={showAffSlot ? 'lg:col-span-2' : ''}>{订阅卡}</div>
    <div className={showAffSlot ? 'lg:col-span-2' : ''}>{账户卡}</div>
    {showAffSlot && (
      <div className="lg:col-span-1">
        <AffiliateRewardCard summary={affSummary} affLink={affLink} loading={affLoading} />
      </div>
    )}
  </div>
```

`showAffSlot` 计算：
- 加载中（`affLoading === true`）→ `true`（先占位防抖动）
- 加载完且 `summary.reward_percent > 0` → `true`
- 加载完且 `summary.reward_percent === 0` 或 `summary === null` → `false`

### 移动端（< lg 断点）

`grid-cols-1` 三卡纵向堆叠，顺序：订阅 → 账户 → 邀请。理由：移动端首屏优先看自己的额度，邀请放底部不占首屏注意力。

---

## 卡片视觉规格

### 容器

```css
background: linear-gradient(135deg,
              var(--semi-color-success-light-default),
              var(--semi-color-success-light-hover));
border-radius: 16px;        /* rounded-2xl */
padding: 16px 14px;
box-shadow: var(--semi-shadow-card);   /* hover 时 hover:shadow-lg */
position: relative;
transition: all 200ms;
```

容器是普通 `<div>`，避开 `.semi-card-body padding !important` 全局规则。

### 自上而下结构

**1. 标题行**（`flex justify-between items-center`）
- 左：`Gift` icon (lucide, 16px) + 文案 `t('邀请奖励')`，14px font-bold，`text-semi-color-text-0`
- 右：Semi `<Tag color="green" size="small">{reward_percent}% {t('返佣')}</Tag>`

**2. 主文案区**
- 主标语：`t('让 token 不再有预算')` — 13px font-bold，`text-semi-color-text-0`
- 副标语：`t('好友充值/月卡续费，每次都拿返佣')` — 11px，`text-semi-color-text-2`
- `aff_count === 0` 时副标语换 `t('从邀请第一位好友开始')`
- `aff_status === 'frozen'` 时整段替换为 Semi `<Banner type="warning" fullMode={false}>{t('您的分销资格已冻结，请联系客服')}</Banner>`

**3. 双 KPI 卡**（`grid grid-cols-2 gap-2`）
- 单卡：`background: rgba(255,255,255,0.55); border-radius: 10px; padding: 8px 6px; text-align: center`
  - 仅作用于背景，文字不受透明度影响
  - 深色模式下通过 `html.dark` 局部覆盖为 `rgba(0,0,0,0.25)` 以保持层级感（实施阶段确认色值）
- 左卡：大数字 `{aff_count}`（18px font-extrabold，`text-semi-color-text-0`）+ 小字 `t('已邀请')`（10px，`text-semi-color-text-2`）
- 右卡：大数字 `{renderQuota(aff_history_quota)}` + 小字 `t('累计返佣')`
  - `renderQuota` 按用户币种偏好显示，统一遵循全站规则

**4. KV 明细区**（两行 `flex justify-between`，11px）
- `t('冷却中')` ─ `${pending_amount_usd.toFixed(2)}`（粗体，`text-semi-color-text-0`）
- `t('本月新增')` ─ `${this_month_earned_usd.toFixed(2)}`（粗体）
- 数值始终用 USD 显示（明细面板与现 `InvitationCard` 一致）

**5. 主 CTA**
- Semi `<Button theme="solid" type="primary" block>`
- 文案：`{copyIcon} {t('复制邀请链接')}`
- `aff_count === 0` 时文案改 `t('立即邀请第一位好友')`
- 禁用条件：`affLink === ''` || `aff_status === 'frozen'`
- 点击逻辑：见下方"行为"小节
- `border-radius: 10px`（受 `.semi-button { border-radius: 10px !important }` 影响，与方案一致）

**6. 底部小链接**（居中文字按钮，11px）
- `t('查看返佣明细')` → `navigate('/console/topup')`

### 加载态

整张卡 4 行 `<Skeleton.Title>` / `<Skeleton.Paragraph>` 堆叠，模拟标题 / 文案 / 双 KPI / 按钮的高度。Skeleton 圆角与卡片一致。

### 深色模式

- 所有颜色走 `var(--semi-color-*)` token，深色模式自动反色
- 实施时需视觉自查：`html.dark` 状态下 `--semi-color-success-light-default` 是否仍呈"浅绿"质感
- 若不理想，在 `index.css` 加 `html.dark` 的局部 CSS 变量覆盖（实现阶段处理）

---

## 数据契约

### 父组件（StatsCards）持有

```jsx
const [affSummary, setAffSummary] = useState(null);
const [affLink, setAffLink] = useState('');
const [affLoading, setAffLoading] = useState(true);

useEffect(() => {
  let cancelled = false;
  Promise.all([
    API.get('/api/user/aff/summary'),
    API.get('/api/user/aff'),
  ])
    .then(([sumRes, codeRes]) => {
      if (cancelled) return;
      if (sumRes?.data?.success) setAffSummary(sumRes.data.data);
      if (codeRes?.data?.success) {
        setAffLink(`${window.location.origin}/register?aff=${codeRes.data.data}`);
      }
    })
    .catch(() => {})
    .finally(() => { if (!cancelled) setAffLoading(false); });
  return () => { cancelled = true; };
}, []);
```

接口失败完全静默：`affSummary === null` 且 `affLoading === false` → `showAffSlot === false` → 卡片不显示。

### `AffiliateRewardCard` props

```ts
interface Props {
  summary: {
    aff_count: number;
    aff_history_quota: number;        // raw quota
    pending_amount_usd: number;
    this_month_earned_usd: number;
    reward_percent: number;
    cooldown_days: number;
    aff_status: 'normal' | 'frozen';
  } | null;
  affLink: string;
  loading: boolean;
}
```

---

## 行为

### 复制邀请链接

```jsx
const handleCopy = async () => {
  try {
    await navigator.clipboard.writeText(affLink);
    showSuccess(t('邀请链接已复制'));
  } catch {
    // fallback: textarea + execCommand
    const ta = document.createElement('textarea');
    ta.value = affLink;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    try {
      document.execCommand('copy');
      showSuccess(t('邀请链接已复制'));
    } catch {
      showError(t('复制失败，请手动复制'));
    } finally {
      document.body.removeChild(ta);
    }
  }
};
```

### 跳转返佣明细

`navigate('/console/topup')`，落到现有完整 `InvitationCard` 区域。

---

## i18n 新增

中文（`web/src/i18n/locales/zh.json`）：
```
邀请奖励
返佣
让 token 不再有预算
好友充值/月卡续费，每次都拿返佣
从邀请第一位好友开始
已邀请
累计返佣
冷却中
本月新增
复制邀请链接
立即邀请第一位好友
邀请链接已复制
复制失败，请手动复制
查看返佣明细
您的分销资格已冻结，请联系客服
```

英文（`web/src/i18n/locales/en.json`）：
```
Invite & Earn
rebate
Unlimited tokens — on us
Earn every time your friends top-up or renew
Invite your first friend to start
Invited
Lifetime
Pending
This month
Copy invite link
Invite your first friend
Invite link copied
Copy failed, please copy manually
View details
Affiliate suspended — please contact support
```

---

## 全局样式适配

`web/tailwind.config.js:23-135` 的 `theme.colors` 是**替换**而非扩展，Tailwind 默认色阶（`bg-green-50`、`text-blue-600` 等）不生成 CSS。本卡片**严格不使用** Tailwind 默认调色板，所有色彩走：

- Semi tokens via Tailwind 类：`text-semi-color-success`、`text-semi-color-text-0`、`text-semi-color-text-2`、`bg-semi-color-bg-0`
- 或 inline style 引用 CSS 变量：`var(--semi-color-success-light-default)` 等

`web/src/index.css:815-829` 的 `.semi-card-body { padding: 10px !important }` 全局覆写无法被 Tailwind `p-*` 撤销。本卡片**不用 Semi `<Card>` 作容器**，改用普通 `<div>`，padding 自管。

---

## 实施清单（高层）

1. 新建 `web/src/components/dashboard/AffiliateRewardCard.jsx`
2. 改 `web/src/components/dashboard/StatsCards.jsx`：fetch、state、栅格条件渲染、引入新组件
3. 加 i18n 键到 `web/src/i18n/locales/zh.json` 和 `en.json`
4. 浅色 + 深色模式视觉自查（Chrome DevTools 切 `html.dark`）
5. `reward_percent === 0` / `aff_status === 'frozen'` / 空数据态手动验证
6. 移动端断点折叠顺序验证

---

## 不做 / 后续可考虑

- 不做：邀请详情新弹窗（跳转钱包页足够）
- 不做：邀请二维码（卡片空间不够，钱包页已有）
- 后续可考虑：admin 端配置「是否在 dashboard 展示邀请卡」的开关（目前用 `reward_percent === 0` 隐含表达）
- 后续可考虑：A/B 测试不同文案的复制转化率
