# 海报弹窗(海报+OSS)运维手册

完整设计与需求见 `openspec/changes/add-poster-popup-system/`。

## 配置项(全部进 option 表,后台 Setting → 仪表盘设置 → 海报弹窗)

| Key | 类型 | 默认 | 说明 |
|---|---|---|---|
| `OSSAccessKeyId` | string | "" | 阿里云 RAM 子账号 AccessKey ID(最小权限:bucket 写) |
| `OSSAccessKeySecret` | string | "" | 阿里云 Secret;`GetOptions` 返回时脱敏为 `***`;字面量 `***` 写回时不覆盖 |
| `OSSEndpoint` | string | "" | 形如 `oss-cn-shanghai.aliyuncs.com`;允许带 `https://` 前缀(后端 strip) |
| `OSSBucket` | string | "" | bucket 名(不含 `oss://` 或 `https://` 前缀) |
| `PosterImageUrl` | string | "" | 海报图 URL;空时弹窗自动失效;上传成功后 URL 会自动填入 |
| `PosterClickUrl` | string | "" | 可选;非空时整张海报包 `<a target=_blank>` |
| `EnablePoster` | bool | false | 总开关;false 时立即回退到现有公告弹窗 |

## 上线步骤

### 1. 创建阿里云 OSS bucket
- 区域:任选(推荐与平台同区,降低延迟)
- 读写权限:**公共读**(海报需要前端浏览器直接访问)
- 跨域:可选,只要图片用 `<img>` 标签加载就不需要 CORS

### 2. 创建 RAM 子账号 + 最小权限
推荐策略:只授予该 bucket 的 `oss:PutObject` 权限。
```json
{
  "Version": "1",
  "Statement": [{
    "Effect": "Allow",
    "Action": "oss:PutObject",
    "Resource": "acs:oss:*:*:my-poster-bucket/*"
  }]
}
```

### 3. 后台填入凭证
后台 → 仪表盘设置 → 海报弹窗
- 填 4 个 OSS 凭证 + 点保存
- 上传海报图片(JPG/PNG/WebP/GIF, ≤ 5MB)
- 填可选的"点击跳转 URL"
- 打开 `EnablePoster` 开关
- 保存

### 4. 验证
- 清浏览器 localStorage(否则受当天频率限制)
- 进首页,应弹出海报
- 关闭后,当天再访问不弹

## 海报与公告优先级

```
首页 mount
   ↓
GET /api/poster
   ↓
EnablePoster=true && PosterImageUrl != "" ?
   │
   ├── 是 ── 检查 localStorage poster_seen_<hash8>_<YYYYMMDD>
   │           │
   │           ├── 已有 → 当天不再弹(也不弹公告)
   │           └── 没有 → 弹 PosterModal
   │
   └── 否 ── 走现有 /api/notice + NoticeModal 公告路径
```

**关键特性**:
- 同一海报一天只弹一次(`localStorage poster_seen_<hash>_<date>`)
- 海报 URL 变化 → hash 变 → 当天也会重新弹一次新海报
- 关海报 modal 后,当天不会再弹任何弹窗(海报和公告互斥)

## 安全机制

### Secret 脱敏
- `GET /api/option/` 返回时,`OSSAccessKeySecret` **非空** → 替换为 `***`
- `PUT /api/option/` 收到 `OSSAccessKeySecret = "***"` → 不覆盖原值
- 比较用严格 `==`,不是 `Contains`,真实 secret 中含 `***` 子串可正常保存

⚠️ **运维警告**:**不要把字面量 `***` 设为真实 OSS Secret**。系统会把它识别为占位符,导致写回被忽略。真实 Aliyun OSS Secret 通常 30+ 字符 base64,不会出现这个问题,但请知悉这个边界。

### 上传校验
- 文件大小 ≤ 5 MB
- Content-Type 白名单:`image/jpeg / image/png / image/webp / image/gif`
- **不接受** `image/svg+xml`(可能内嵌 script,XSS 风险)
- 文件名重写为 `posters/poster_<uuid><ext>`(防冲突 + 防猜测)
- admin 路由 + `UploadRateLimit` 中间件防刷

### OSS 凭证轮换
- 在阿里云 RAM 控制台创建新 AccessKey
- 后台 Setting 页填入新 AccessKeyId + AccessKeySecret(覆盖旧值)→ 保存
- 在 RAM 控制台禁用/删除旧 AccessKey

## 紧急关停

发现海报内容不当或 OSS 异常:
1. 后台关闭 `EnablePoster` 开关 → 立即回退到公告
2. 或清空 `PosterImageUrl` → 立即失效
3. 用户当天已弹的不会"撤回",但下次访问就不会再弹

## 公告功能不受影响

现有"系统公告"(`OptionMap["Notice"]` + `/api/notice` + `NoticeModal` + `localStorage.notice_close_date`)**完全不动**。海报关闭/失败时自动回退,行为与上线前 byte-identical。

## 已知限制

1. **不支持多张海报轮播**(v1 单张;按需求扩展)
2. **不做点击埋点统计**(可后续单独 change 加)
3. **不支持前端直传 OSS**(STS 临时凭证模式;海报场景不需要)
4. **不区分海报与公告同时弹**(产品决策互斥)
5. **同一 URL 重传图片改内容**用户当天看不到新图(URL 不变 hash 不变)— 实际上 OSS object key 含 UUID,重传会自动产生新 URL,这个问题基本不存在
