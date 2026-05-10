# Design: Poster Popup System

## Context

平台首页有现成的"系统公告"弹窗(markdown 渲染)。运营反馈 markdown 公告
传播力不足,希望补一个"海报弹窗":运营在后台上传一张图片(OSS),前端首次
访问时弹出大图,可选点击跳转到外部链接(如公众号文章、活动页)。

代码库现状(扫码确认):
- `Home/index.jsx:92-110` 负责现有公告弹出 + `localStorage.notice_close_date`
  控制频率
- `NoticeModal.jsx` 是 markdown 公告组件
- `controller/misc.go:151 GetNotice` 从 `OptionMap["Notice"]` 读字符串
- 项目**无 OSS 集成**(go.mod 无 `aliyun-oss-go-sdk`)
- 项目**无通用图片上传**

新功能必须从零集成 OSS SDK,但所有其他改动都基于现有模式(option 表 + 公开/admin
路由 + Setting 页面)。

## Goals / Non-Goals

### Goals
- 运营可在后台上传图片到阿里云 OSS,无需手工命令行操作
- 海报弹窗与现有公告并存,有海报先弹海报,无海报回退到公告
- 一天一次弹出节奏 + 海报变化时重新弹一次
- OSS 凭证不暴露给前端(后端代理上传)
- 出问题可一键关停(`EnablePoster=false`)

### Non-Goals
- 不做前端直传 OSS(STS 签名,过度工程)
- 不做多张海报轮播(YAGNI,v1 单张)
- 不做点击埋点统计(可后期单独 change)
- 不修改现有公告 Markdown 能力
- 不支持海报与公告同时弹出(用户决策互斥)

## Decisions

### Decision 1: 后端代理上传 vs 前端直传 OSS

**选择**:后端代理(管理员上传到后端,后端再用 SDK 上传到 OSS)。

**Why**:
- OSS Secret 不会经过前端,即使 admin 权限被绕过也不会泄漏凭证
- 海报通常 < 1 MB,后端转发的带宽成本可忽略
- STS 临时凭证模式需要 RAM 角色 + Policy 配置,运营接入成本高

**Trade-off**:海报上传走一次后端,带宽占用极少。可接受。

### Decision 2: OSS 凭证存放 — option 表 vs 环境变量

**选择**:option 表(后台 Setting 页可配)。

**Why**:与项目现有的 `WeChatServerToken` / `TurnstileSecretKey` 等敏感配置一致;
运行时可调,不需要重启。

**安全措施**:
- `OSSAccessKeySecret` 在 GET option 接口返回时脱敏为 `***`(非空时);
- 前端 SettingsPoster 检测占位值时,用户保存时如果 Secret 仍是占位 `***`
  则不更新该字段(避免覆盖回真实值);
- 上传 API 限制 admin 权限 + UploadRateLimit 中间件防刷

### Decision 3: 海报 vs 公告的并存优先级

**选择**:有海报且 `EnablePoster=true` 时弹海报;否则弹现有公告。两者**互斥**。

**Why**:
- 同时弹两个 modal UX 差
- 海报通常是主推内容,公告是兜底信息

**实现**:`Home/index.jsx` 先 fetch `/api/poster`,有则弹 `PosterModal`;
没有则走现有 `checkNoticeAndShow` 公告路径。

### Decision 4: 频率控制 key 设计

**选择**:`localStorage.poster_seen_<md5(image_url).slice(0,8)>_<YYYYMMDD>`。

**Why**:
- 现有公告用 `notice_close_date`(只看日期,公告内容变化不会重新弹)
- 海报场景下"换图重新弹"是核心需求 — 用 image_url 哈希前 8 位作为版本标记
- `<YYYYMMDD>` 让用户每天最多看一次同一海报

**Trade-off**:同一 image_url 改了内容(只重传不换 URL)用户当天不会再看到。
但 OSS 上传会自动加 UUID 后缀,文件名实际不会重用,这个边界基本不存在。

### Decision 5: 文件名 + Mime 类型限制

**选择**:
- 上传时重写文件名为 `poster_<uuid><ext>`(防冲突 + 防猜测 URL)
- mime 类型白名单:`image/jpeg / image/png / image/webp / image/gif`
- 大小 ≤ 5 MB(防大文件耗带宽 + 海报通常 < 1 MB,5 MB 足够)
- 不在 OSS 路径加额外目录(直接 bucket 根 / 或 `posters/` 前缀方便后期清理)

**Why**:UUID 防冲突;mime 白名单防恶意 SVG XSS;5 MB 兜底防滥用。

**OSS object key 格式**:`posters/poster_<uuid><ext>`(固定 `posters/` 前缀)。
- 防止 bucket 根目录混杂;
- 未来扩展(头像、其他上传)时分目录管理与生命周期清理更清晰;
- 单一固定前缀,避免实现期歧义。

### Decision 6: 上传成功后是否自动覆盖 PosterImageUrl

**选择**:**不自动覆盖**。上传 API 返回 OSS URL,前端 SettingsPoster 拿到后
自动填入输入框,**但管理员仍需点"保存"才生效**。

**Why**:
- 管理员可能想先预览图片再决定是否使用
- 防止误操作上传后立即生效

### Decision 7: 海报降级策略

**选择**:任何环节失败都静默降级到"无海报",回退到现有公告路径。

**Why**:海报是增量功能,不应该因为它的故障破坏现有公告体验。

具体降级点:
- OSS 配置不全或上传失败 → admin 上传时报错,但不影响线上(线上仍用旧 URL)
- 前端 fetch `/api/poster` 失败 → 静默不弹海报,走公告
- 海报图片在 OSS 被删 → `<img>` 加载失败,modal 仍能关闭,下次 fetch 时
  后台可以发现 URL 失效


## State Diagram

```
首页 mount
    ↓
fetch /api/poster
    ↓
EnablePoster=true && PosterImageUrl != "" ?
    │
    ├── 是 ──> 检查 localStorage.poster_seen_<hash8>_<today>
    │           │
    │           ├── 已有(今天看过)──> 不弹海报(也不弹公告,因为优先级)
    │           └── 没有 ──> 弹 PosterModal
    │                          ↓
    │                       用户关闭 ──> 写 poster_seen key
    │                       用户点击图片(若 click_url 非空)──> 新窗口跳转
    │
    └── 否 ──> 走现有 checkNoticeAndShow 公告逻辑
```

## Risks / Trade-offs

| 风险 | 缓解 |
|---|---|
| OSS 凭证泄漏 | 后台 Secret 脱敏 + admin 路由 + 代码不写日志 |
| 上传被滥用 | UploadRateLimit + admin 权限 + 大小/类型限制 |
| OSS 单点故障 | 海报功能降级,现有公告兜底 |
| 海报 URL 失效(被删) | 前端 `<img onerror>` 静默隐藏,不阻断弹窗关闭 |
| 同一 URL 换内容用户当天看不到新图 | 文档说明"换图请重新上传(URL 会变化)" |
| 跨域问题 | OSS bucket 公开读;<img> 不需要 CORS;<a> 跳转走新 tab |
| 海报压根不存在(未配置) | 前端默认走公告路径,与现状完全一致(零回归) |

## Migration Plan

无数据迁移:
- option 表已有,7 个新 key 通过 OptionMap init 自动注册;
- 旧用户 `localStorage` 已有 `notice_close_date` 不影响,海报独立 key;
- 现有公告 API 与逻辑完全不动。

## Observability

- OSS 上传失败 SysLog 写一条,带 admin id + 文件名 + 错误原因
- 前端 fetch /api/poster 失败 console.error,不写后端日志(避免被刷)
- 后台 SettingsPoster 页加"测试 OSS 连通性"按钮(可选,简化版可省略 — v1 不做)

## Open Questions

无。前期 4 轮决策已确认所有关键点。
