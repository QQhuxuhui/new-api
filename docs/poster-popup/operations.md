# 海报弹窗(海报+OSS)运维手册

完整设计与需求见 `openspec/changes/add-poster-popup-system/`。

## 权限要求

海报相关的所有运营操作(Setting 页 + 上传 + GET options 含脱敏 secret)
要求 **root 角色**(`middleware.RootAuth()`)。这与现有 option 表的所有运营
操作权限一致(Notice、announcements、模型分组、汇率等同样需要 root)。

普通管理员(`admin` 但非 `root`)看不到设置页(Setting 入口 `isRoot()`
判断)、调用上传接口会被中间件拦截。

公开读接口 `GET /api/poster` 无需认证(给前端首页用)。

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
- 读写权限:**两种方式任选其一**
  - 方式 A(推荐,省心):创建 bucket 时直接选 **公共读** → 整个 bucket 公开读
  - 方式 B(更精细):创建 bucket 时保持默认 **私有** → **代码会自动给每个上传的海报 object 单独设置 ObjectACL=public-read**(`oss.ObjectACL(oss.ACLPublicRead)`),从 v271 起生效;只有海报路径下的对象公开,bucket 其他对象仍私有
- ⚠️ **如果你之前在 v267-v270 上传过海报,那些图片 object 默认是 private,前端会报 `You have no right to access this object`。修复方式:**
  - 选项 1:在 OSS 控制台找到 `posters/` 路径下的旧 object → 单独"设置文件 ACL → 公共读"
  - 选项 2:在后台**重新上传**海报(v271+ 上传的会自动 public-read)
  - 选项 3:把 bucket 整体 ACL 改为公共读
- 跨域:可选,只要图片用 `<img>` 标签加载就不需要 CORS

### 2. 创建 RAM 子账号 + 最小权限
推荐策略:授予 `oss:PutObject` + `oss:PutObjectAcl`。后者用于代码给每个海报对象设置 ACL=public-read。

```json
{
  "Version": "1",
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "oss:PutObject",
      "oss:PutObjectAcl"
    ],
    "Resource": "acs:oss:*:*:my-poster-bucket/posters/*"
  }]
}
```

如果方式 A(整个 bucket 公开读),`oss:PutObjectAcl` 也加上不亏,代码做的 ACL 设置是 idempotent 的。

如果不想给 PutObjectAcl,只能选方式 A(整 bucket 公开读),代码层 ObjectACL 会失败,但因为 bucket 已经公开读,效果一样能访问。

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

### Secret 脱敏与防误覆盖
- `GET /api/option/` 返回时,`OSSAccessKeySecret` **非空** → 替换为 `***`
- `PUT /api/option/` 收到 `OSSAccessKeySecret = "***"` → 不覆盖原值
- `PUT /api/option/` 收到 `OSSAccessKeySecret = ""`(空字符串)→ **也不覆盖**(防止管理员清空密码输入框时误清空 Secret)
- 比较用严格 `==`,不是 `Contains`,真实 secret 中含 `***` 子串可正常保存

⚠️ **运维警告**:
- **不要把字面量 `***` 设为真实 OSS Secret**(会被识别为占位符)
- **要"关闭"海报功能**:关闭 EnablePoster 即可,**不要**清空 Secret 输入框试图禁用 OSS — 后端会把空字符串当成"不修改"
- **真要重置 Secret**(罕见运维场景):后台 UI 不支持,请在 DB 中直接 `UPDATE options SET value='' WHERE \`key\`='OSSAccessKeySecret';` 然后调一次 `model.InitOptionMap()` 或重启服务

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
