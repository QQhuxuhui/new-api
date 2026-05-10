# Change: 添加首页海报弹窗(阿里云 OSS 图片 + 后台上传)

## Why

平台首页当前只有"系统公告"弹窗(`common.OptionMap["Notice"]` 字符串 +
`Home/index.jsx::checkNoticeAndShow` + `NoticeModal` markdown 渲染)。
运营场景上,markdown 公告的传播力远不如**图片海报**(节日宣传、新功能推广、
活动入口等),且海报通常需要一键跳转到外部链接(如公众号文章、活动落地页)。

现状梳理:
- 项目**没有任何 OSS 集成**(go.mod 无 `aliyun-oss-go-sdk`,代码无 OSS 调用)
- 项目**没有通用图片上传 API**(只有 LLM relay multipart 处理,不能复用)
- 现有公告机制完整保留,本 change 不动它

需求约束(已确认):
1. OSS 凭证存放在后台 Setting 页(option 表),运行时可调,与现有配置点一致
2. **新增"海报"独立类型,与公告并存**:有海报先弹海报,无海报再走现有公告
3. 弹出频率与现有公告一致 — 一天一次(`localStorage` 控制),且海报 URL 变化时
   视为新海报重新弹一次
4. 海报支持点击跳转到自定义链接(可选填,空时纯展示)

## What Changes

### 后端
- **ADDED**: 引入 `github.com/aliyun/aliyun-oss-go-sdk/oss` 依赖
- **ADDED**: 新增 7 个 option 配置:`OSSAccessKeyId` / `OSSAccessKeySecret` /
  `OSSEndpoint` / `OSSBucket` / `PosterImageUrl` / `PosterClickUrl` /
  `EnablePoster`,均通过现有 OptionMap + updateOption switch 接入
- **ADDED**: `service/oss_uploader.go` 封装 OSS client,`UploadFileToOSS(reader,
  filename, contentType) (publicUrl, err)` 方法
- **ADDED**: `controller/poster.go`:
  - `POST /api/option/poster/upload`(admin only,multipart)— 上传图片到 OSS,
    成功后返回 public URL,**不**自动覆盖 `PosterImageUrl`(让管理员预览后手动确认)
  - `GET /api/poster`(公开)— 返回 `{enabled, image_url, click_url}` 三字段
- **ADDED**: 路由注册 + `UploadRateLimit` 中间件保护(防刷)
- **ADDED**: `OSSAccessKeySecret` 在 admin 通用 GET option 接口里**脱敏返回**(非空时
  返回 `***`,只表示"已配置")
- **ADDED**: 上传校验:文件大小 ≤ 5 MB;mime 类型限制为 `image/jpeg /
  image/png / image/webp / image/gif`;文件名重写为 `poster_<uuid><ext>`
  防冲突

### 前端
- **ADDED**: `web/src/components/layout/PosterModal.jsx`:海报弹窗组件(大图 +
  关闭按钮 + 可选 `<a target="_blank">` 包裹整个图片支持点击跳转)
- **ADDED**: `web/src/pages/Setting/Dashboard/SettingsPoster.jsx`:后台海报设置页
  - OSS 凭证 4 字段(Secret 显示为 `***` 占位,留空保留原值)
  - "上传海报"按钮 → 调 `POST /api/option/poster/upload` → 拿到 OSS URL
    自动填入 `PosterImageUrl` 输入框(管理员可再调整)
  - `PosterImageUrl` / `PosterClickUrl` 两个文本框 + `EnablePoster` 开关
  - 实时预览(展示当前 `PosterImageUrl` 的图片)
- **ADDED**: 后台 Dashboard 设置组加 tab "海报弹窗"(与现有 "系统公告" tab 同级)
- **MODIFIED**: `web/src/pages/Home/index.jsx`:并存逻辑 — 优先 fetch
  `/api/poster`,有 `enabled && image_url` 则弹 `PosterModal`;否则走现有
  `checkNoticeAndShow` 弹公告。两者不会同时弹

### localStorage 频率控制
- 现有公告用 `notice_close_date` 当天关闭后不再弹
- 海报新增 `poster_seen_<hash8>_<YYYYMMDD>`,其中 `hash8` 是 `image_url` 的
  md5 前 8 位 — 海报图片变了 → key 变 → 视为新海报重弹

## Impact

- **Affected specs**: 新增 capability `poster-popup-system`(零现有冲突;不修改
  现有"系统公告"功能,只在 Home 入口加优先级判断)
- **Affected code**:
  - `go.mod` / `go.sum` — `aliyun-oss-go-sdk` 依赖
  - `common/constants.go` — 7 个新配置变量
  - `model/option.go` — OptionMap init + updateOption switch + Secret 脱敏返回
  - `service/oss_uploader.go` — 新文件
  - `controller/poster.go` — 新文件
  - `controller/option.go::GetOptions` — Secret 字段读时脱敏为 `***`;`updateOption` 检测占位 `***` 不覆盖原值
  - `router/api-router.go` — 2 条新路由
  - `web/src/components/layout/PosterModal.jsx` — 新文件
  - `web/src/pages/Home/index.jsx` — 加 fetch /api/poster + 优先级判断
  - `web/src/pages/Setting/Dashboard/SettingsPoster.jsx` — 新文件
  - 后台 Setting Dashboard 父页面 — 加 tab 入口
- **Database migration**: 无新表;option 表已有,新 key 通过 OptionMap 注册自动
  生效
- **Out of scope**:
  - 不做前端直传 OSS(STS 签名复杂度高,海报场景不需要)
  - 不做多张海报轮播(YAGNI;当前只支持 1 张)
  - 不做海报埋点统计(点击跳转计数);如需要可后期扩展
  - 不做海报与公告的"两者都弹"模式(决策已选互斥)
  - 不修改现有"系统公告"逻辑,仅在 Home 入口加优先级判断
