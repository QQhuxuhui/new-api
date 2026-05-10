# Implementation Tasks

按提交顺序排列。每个 section 对应一次独立可 review 的提交。

## 1. OSS SDK 依赖与配置项

- [ ] 1.1 在 `go.mod` 中添加 `github.com/aliyun/aliyun-oss-go-sdk` 并 `go mod tidy`
- [ ] 1.2 在 `common/constants.go` 添加 7 个变量:`OSSAccessKeyId` / `OSSAccessKeySecret` / `OSSEndpoint` / `OSSBucket` / `PosterImageUrl` / `PosterClickUrl` / `EnablePoster`(默认全空 / false)
- [ ] 1.3 在 `model/option.go` 的 OptionMap init 块注册这 7 个 key
- [ ] 1.4 在 `model/option.go` 的 updateOption switch 中添加 7 个 case 处理
- [ ] 1.5 单测:覆盖默认值与 update switch 路径

## 2. OSS 上传 service

- [ ] 2.1 新建 `service/oss_uploader.go`,导出 `UploadFileToOSS(reader io.Reader, filename, contentType string) (string, error)`
- [ ] 2.2 函数内部:校验 4 个 OSS config 非空 → 构建 oss.Client → PutObject 到 bucket → 返回 public URL `https://<bucket>.<endpoint>/<filename>`
- [ ] 2.3 配置缺失/SDK 错误返回**带前缀**的明确错误信息,便于前端展示
- [ ] 2.4 单测:用 mock 或捕获 client init 失败路径(无法 mock 真实 OSS 调用,但可校验配置缺失分支)

## 3. Secret 脱敏

- [ ] 3.1 在 `controller/option.go::GetOptions` 中对 `OSSAccessKeySecret` 做脱敏:非空则在 response 中替换为 `***`(原值不动 DB)
- [ ] 3.2 在 `model/option.go::updateOption` 的 `OSSAccessKeySecret` case 中检测:如果传入值就是字面量 `***`,跳过该字段不更新(让原值保留)
- [ ] 3.3 单测:覆盖脱敏读、占位写不覆盖、真实 secret 写覆盖三种路径
- [ ] 3.4 实现注意:占位检测用 `==` 严格比较 `***`,不要用 `Contains`(允许真实 secret 中含 `***` 子串)

## 4. 海报上传 + 公开读 API

- [ ] 4.1 新建 `controller/poster.go`,实现 `UploadPoster(c *gin.Context)`:
  - 校验 admin 权限(中间件已保证)
  - `c.FormFile("file")` 取 multipart 文件
  - 校验 size ≤ 5 MB,Content-Type 在白名单
  - 生成 `posters/poster_<uuid><ext>` 作为 OSS object key
  - 调 `service.UploadFileToOSS`
  - 返回 `{success: true, data: {url: <public_url>}}`
- [ ] 4.2 实现 `GetPoster(c *gin.Context)`:从 `common.OptionMap` 读 3 个字段并返回
- [ ] 4.3 在 `router/api-router.go` 注册:
  - `POST /api/option/poster/upload` 挂在 admin 路由组 + UploadRateLimit
  - `GET /api/poster` 挂在公开路由组(无 auth 要求,首页未登录也能访问)
- [ ] 4.4 单测:覆盖大小拒绝、mime 拒绝、OSS 未配置拒绝、成功路径(用 stub uploader)

## 5. 前端 PosterModal 组件

- [ ] 5.1 新建 `web/src/components/layout/PosterModal.jsx`
  - props:`visible`、`imageUrl`、`clickUrl`、`onClose`
  - 大图居中,关闭按钮右上角
  - 有 clickUrl 时整张图包 `<a target="_blank" rel="noopener noreferrer">`
  - `<img onError>` 静默隐藏图(避免 broken image 阻塞 modal)
  - 移动端响应式(max-width: 90vw)

## 6. 后台海报设置页

- [ ] 6.1 新建 `web/src/pages/Setting/Dashboard/SettingsPoster.jsx`
  - 4 个 OSS 凭证输入框(Secret 字段密码模式 + 占位符 `***` 已配置时)
  - 1 个文件上传组件 → 调 `/api/option/poster/upload` → 拿到 url 自动填入 PosterImageUrl 输入框
  - 2 个 URL 输入框:`PosterImageUrl` / `PosterClickUrl`
  - 1 个 Switch:`EnablePoster`
  - 实时预览框(展示当前 PosterImageUrl 的图片)
  - "保存"按钮 → 调现有 option PUT 批量保存
- [ ] 6.2 在 Setting Dashboard 父页面注册新 tab "海报弹窗"(与现有 tab 同级)

## 7. Home 页面优先级与频率控制

- [ ] 7.1 在 `web/src/pages/Home/index.jsx` 添加 `posterVisible / posterData` state
- [ ] 7.2 在 `useEffect` 中先 fetch `/api/poster`:
  - 若 `enabled && image_url` → 计算 `key = poster_seen_<md5(image_url).slice(0,8)>_<YYYYMMDD>`
  - localStorage 已有 key → 不弹海报,**也不**弹公告(当天结束)
  - localStorage 没有 key → 弹 PosterModal,关闭时 setItem
  - 否则 → 走现有 `checkNoticeAndShow` 公告路径
- [ ] 7.3 引入轻量 md5 实现(crypto-js 或自写 8 位哈希;前端无需安全级 hash,简单字符串哈希即可)

## 8. 文档与运维

- [ ] 8.1 在 `docs/` 创建 `docs/poster-popup/operations.md`,描述:
  - 必需的 OSS bucket 配置(开放读取 ACL)
  - 7 个 option key 含义
  - 凭证泄漏应急处理(轮换 AccessKey)
  - 海报弹窗与公告优先级说明
  - **AccessKeySecret 占位符约定**:不得把字面量 `***` 设为真实 Secret
    (后端用此值做"未修改"占位检测,会导致写回被忽略)

## 9. E2E 验证

- [ ] 9.1 端到端验证清单:
  - 配置 OSS → 上传海报 → URL 自动填入 → 保存 → 首页看到海报弹窗
  - 关闭海报 → 当天再访问不弹
  - 第二天访问 → 重新弹同一海报(localStorage key 包含日期)
  - 后台改图 → URL 变化 → 用户即使当天关过也会看到新海报
  - 关闭 EnablePoster → 海报不弹,回退到现有公告
  - 海报 URL 失效(404)→ modal 仍可关闭,localStorage 仍记录
  - clickUrl 为空 → 海报不可点
  - clickUrl 非空 → 点击新窗口打开
  - Secret 占位写回不覆盖原值
  - 上传超过 5 MB / mime 类型错 → 拒绝
