# new-api 渠道级图片超分（RunPod Serverless）设计

日期：2026-08-17
状态：待评审
范围：new-api（收口转换层）+ RunPod Serverless worker（Real-ESRGAN）

## 1. 背景与目标

生图链路为 `客户 → sub2api → new-api → cliproxyapi / adobe2api / 第三方上游`，new-api
是所有生图渠道的收口。部分上游只能原生出 1K/2K，但业务要卖 2K/4K。目标：在 new-api
渠道配置上声明一条超分规则（如 `1K→4K`），使得：

- 选路时该渠道对超分可达的档位可见（"加入允许队列"）；
- 出站请求自动降档为渠道原生档位；
- 回程响应在 new-api 内调 RunPod Serverless（Real-ESRGAN）放大到目标尺寸后再返回；
- 下游 sub2api 的计费（按实际像素判档）与直连用户的计费（按请求尺寸预估）均保持正确。

商业形态：卖 4K 的价，付 1K 的上游成本 + 每张几厘的 GPU 费。

已确认的决策：

| 决策点 | 结论 |
|---|---|
| 插入层 | new-api（不动 sub2api / cliproxyapi） |
| 中间档派生 | 宽松：规则 `1K→4K` 使 2K 也可达（`(native_max, To]` 全部） |
| 端点范围 | 仅 `/v1/images/generations`（**勘误 2026-08-18**：edits 的 adaptor 转换路径不读 `request.Size`，降档无法抵达上游，实施期裁决 edits 整体退出本期超分、转后续迭代；原文含 edits）；chat 出图路径不做 |
| 每渠道规则数 | 至多一条，校验层强制 |
| GPU 平台 | RunPod Serverless（flex worker，scale-to-zero） |
| 模型 | Real-ESRGAN x4plus（A 类忠实放大，不重绘） |

## 2. 现状锚点（代码事实，实现时以此为准）

| 机制 | 位置 |
|---|---|
| 档位常量 / 比例尺寸表 / 选路分类 | `dto/image_size_tier.go`（`imageSizeTierRatioTable:33`、`ClassifyImageRoutingTier:148`） |
| 渠道能力声明 | `dto/channel_settings.go:107` `ImageSizes *ImageSizeCapability` |
| 选路时档位注入 | `middleware/distributor.go:1097`（`ContextKeyImageSizeTier`） |
| 选路过滤 | `model/channel_capability.go:101` `setting.ImageSizes.Allow(tier)` |
| 配置写入校验 | `model/channel.go:1001` `channelParams.ImageSizes.Validate()` |
| 图片 relay 唯一入口 | `relay/image_handler.go:23` `ImageHelper` |
| 预扣费（早于 ImageHelper，按原始请求） | `controller/relay.go:216` `ModelPriceHelper` |
| sub2api 按实际像素计费的门 | `openai_images_usage_simulation.go:607`（账号旗标 + 模型 + 请求形状三重门） |
| sub2api 非模拟路径读声明 size | `image_output_accounting.go` `addDataArray` 读 `item.size` |

sub2api 侧现状（生产库核验于 2026-08-17）：桥账号 `spark-image2`（id=1132，
`base_url=http://new-api:3000`）已开 `openai_images_usage_simulation` 与
`openai_images_highres`，且为唯一 active 的 highres 账号 —— sub2api 的 2K/4K 流量
全部经 new-api，无旁路。

## 3. 总体架构

```
客户请求 4K (3840x2160, gpt-image-2)
  │
  ▼ sub2api（账号 spark-image2，预扣按请求）
  ▼ new-api distributor
      · ClassifyImageRoutingTier → tier=4K
      · 判定"超分资格"（§6 形状谓词）→ eligible=true
      · 选路过滤：native{1K} ∪ 派生{2K,4K} → 渠道命中
  ▼ ImageHelper
      · 出站降档：size 3840x2160 → 1280x720（同比例 1K 原生尺寸）
      · 上游生成 1280x720
      · 回程拦截（DoResponse 之前）：
          b64 解码 → 上传对象存储 → RunPod /run(源图URL, 目标尺寸)
          → worker: Real-ESRGAN 4x → Lanczos 精确缩放 → 传回对象存储
          → new-api 下载结果 → 替换 b64_json → 改写声明 size 字段
      · DoResponse 照常（写客户、算 usage）
  ▼ sub2api 收到 3840x2160 像素的响应
      · usage simulation 解码实际像素 → 判 4K 档 → 客户按 4K 计费
```

## 4. 配置模型

`ImageSizeCapability` 增加可选字段，`allowed` 语义保持**纯原生**不变：

```jsonc
"image_sizes": {
  "allowed": ["1K"],                       // 不变：仅原生能力
  "upscale": { "from": "1K", "to": "4K" }  // 新增：至多一条
}
```

```go
// dto/image_size_tier.go
type ImageUpscaleRule struct {
    From string `json:"from"` // 必须 ∈ NormalizedAllowed()
    To   string `json:"to"`   // 必须 > max(NormalizedAllowed())
}

type ImageSizeCapability struct {
    Allowed []string          `json:"allowed,omitempty"`
    Upscale *ImageUpscaleRule `json:"upscale,omitempty"`
}
```

校验（挂进现有 `Validate()`，`model/channel.go:1001` 链自动生效）：

1. `Upscale != nil` 时要求 `Allowed` 非空且含合法档位 —— 空白名单是 fail-open
   全放行，叠加超分规则语义矛盾，直接拒绝写入；
2. `From`、`To` 必须是合法档位；`From ∈ Allowed`；`To` 严格高于 max(Allowed)；
   `To ∈ Allowed` 时拒绝（规则无意义）；
3. 结构上 `Upscale` 是单对象非数组，天然满足"每渠道至多一条"。

SQL 直改绕过校验的兜底：运行期解析失败/含非法档位的 `upscale` 视同不存在
（fail-open 到纯原生行为），与现有 `Allow()` 的 `anyValid` 兜底同款，绝不让
一条写废的规则把渠道从选路里隐身。

## 5. 选路派生（宽松）

`Allow(tier)` 保持原生语义不动，新增：

```go
// AllowWithUpscale: eligible 表示本请求具备超分资格（§6）。
// 派生可达集 = native ∪ (native_max, To]，仅在 eligible 时生效。
func (c *ImageSizeCapability) AllowWithUpscale(tier string, eligible bool) bool
```

- `middleware/distributor.go` 在设置 `ContextKeyImageSizeTier` 的同处，按 §6 谓词
  计算并注入 `ContextKeyImageUpscaleEligible`（新增 context key）；
- `model/channel_capability.go:101` 的过滤改调 `AllowWithUpscale(tier, eligible)`，
  `ChannelSelectFilter` 增加 `UpscaleEligible bool` 字段；
- 观测：命中派生档位（而非原生）通过的渠道，打新的 reject/accept 计数标签
  `image_size_via_upscale`，与现有 `markImageSizeRejected` 并列，保证运营侧能看到
  "这条渠道的 4K 是超分出来的"。

不满足资格的请求（stream 等）派生集为空，行为与今天完全一致。

## 6. 超分资格谓词（与 sub2api 模拟资格对齐）

sub2api 只有 usage simulation 路径才按实际像素计费；形状不满足时透传上游 usage
（1K 的量）→ 客户拿 4K 图按 1K 扣，漏账。因此 **new-api 侧的超分资格 ≡ sub2api
的可模拟形状**，在 distributor 判定一次，全链共用：

```
eligible :=
  路径 ∈ {generations, edits}
  && !stream && partial_images 未设
  && n == 1
  && 无 mask（edits）
  && background ∈ {"", "opaque"}
  && output_format ∈ {"", "png"}
  && response_format ∈ {"", "b64_json"}
  && output_compression 未设 && input_fidelity 未设
  && 模型 ∈ {gpt-image-2, gpt-image-2-2026-04-21}
```

（模型白名单与 `isSimulatableOpenAIImagesModel` 逐字对齐；两侧口径漂移的风险写进
§10 的防漂移注释要求。）

不满足 → 不派生、不降档、不超分，纯原生选路。

## 7. 出站降档改写

位置：`relay/image_handler.go` `ImageHelper`，`ModelMappedHelper` 之后、
`ConvertImageRequest`/passthrough 分支之前。

判定：`eligible && 请求档位 ∉ native allowed && 请求档位 ≤ Upscale.To`
→ 本次请求进入超分模式，记录到 relayInfo（新增 `ImageUpscalePlan` 结构：
目标宽高、降档后尺寸、规则来源渠道 id）。

尺寸映射（复用 `imageSizeTierRatioTable`，含转置表）：

1. 解析用户请求 size：
   - `"WxH"` 形式：目标 = 精确 `WxH`；比例 = 就近匹配表内比例（长边口径）；
   - `"4K"` 字面档位：比例取 `1:1`，目标 = 表内 `[1:1][4K]` = 2880x2880；
2. 出站 size = 表内 `[匹配比例][Upscale.From]`（如 16:9 1K = 1280x720）；
3. 表内匹配不到比例的表外尺寸：按长边等比落到 From 档（长边 ≤ From 档上限），
   目标仍为用户请求的精确 WxH。

两条改写路径都要覆盖：

- 常规路径：改 `request.Size`（深拷贝，31 行的 `common.DeepCopy` 产物），
  `ConvertImageRequest` 自然带出；
- passthrough 路径（49 行 `PassThroughBodyEnabled` 或全局开关）：对原始 body
  用 sjson 改写 `size` 字段后再入 buffer。

**不动 `imageReq` 原始对象** —— `controller/relay.go:216` 的 `ModelPriceHelper`
在 ImageHelper 之前已按原始请求算完预扣（直连用户按请求的 4K 计费，正确），
日志 `logContent` 也改为记录 `用户尺寸 + "（1K 超分）"` 而非降档后尺寸。

## 8. 回程超分

位置：`ImageHelper` 中 `httpResp` 状态检查之后（106 行）、`adaptor.DoResponse`
（108 行）**之前**。DoResponse 内部直接写客户端，之后没有拦截机会；在它之前把
`httpResp.Body` 整体读出、变换、再包回去，adaptor 与其后所有逻辑看到的就是一个
"原生 4K 上游响应"，零改动。

流程（新增 `service/image_upscale.go`）：

```
body 读出 → gjson 取 data[0].b64_json → 解码 PNG（校验尺寸=预期降档尺寸）
→ PUT 源图到对象存储（presigned）
→ RunPod POST /run {source_url, target_w, target_h, output:"png"}
→ 轮询 /status/{id}（间隔 1s，指数上限 5s）
→ 拿到 result.output_url → GET 下载结果 PNG（校验尺寸=目标）
→ sjson 写回：data[0].b64_json = 新图；改写声明 size：
    · 根级 size / output_format（如存在）→ 目标 "WxH" / "png"
    · data[0].size（如存在）→ 目标值
→ httpResp.Body = io.NopCloser(新 body)；删 Content-Length（交给 Go 重算）
```

改写声明 size 是硬要求：sub2api 非模拟兜底路径按响应**声明的** size 判档
（`addDataArray`），不改写会把 4K 图判成 1K 档。

超时预算：上游 1K 生成（5–20s）+ 超分（热 2–4s / 冷 +10–30s）+ 存储往返（2–8s），
总体优于原生 4K 生成（30–60s+）。RunPod 调用整体设 90s 硬超时（含轮询），由
`IMAGE_UPSCALE_TIMEOUT` 配置。

### 对象存储

走对象存储中转是硬约束：RunPod `/run` payload 上限 10MB，2K 源图 PNG 的 b64 即可
超限；4K 结果 PNG（2880² 约 15–25MB）双向都过不去。

- new-api 侧用 S3 兼容客户端（`aws-sdk-go-v2`，path-style），**同一套代码同时兼容
  阿里云 OSS 与 Cloudflare R2**，endpoint/bucket/AKSK 全部环境变量注入；
- 推荐 R2：new-api 在 OVH（加拿大）、RunPod worker 在美欧，R2 出流量免费且全球
  边缘；现有阿里云 OSS 作为可用的第二选择（跨境往返慢，功能等价）；
- 凭据最小化：专用子账号/Token，仅限单 bucket 读写；
- 对象 key：`upscale/{date}/{request_id}/{src|out}.png`，bucket 生命周期 1 天自动
  清理，presigned URL 有效期 15 分钟。

### RunPod worker

独立小仓库（`runpod-upscale-worker/`，随本 spec 实施新建）：

- 镜像：`runpod/pytorch` 基底 + `realesrgan`，**权重（RealESRGAN_x4plus.pth,
  64MB）烤进镜像**，不用 network volume（会把 endpoint 钉死单数据中心，丧失
  全球池弹性）；
- handler：模块顶层加载模型 + 启动时空推理预热 CUDA —— FlashBoot 是进程快照，
  只快照缩容瞬间已存在的状态，懒加载会让每次冷启动重付 10–30s；
- 推理：`tile=512, tile_pad=32, half=True`；4x 输出后 Lanczos 精确缩放到
  `target_w×target_h`（表内 4K 非整倍：1:1 为 2.8125x，16:9 恰为 3x）；
- 输入输出均走 presigned URL，payload 只含 URL 与目标尺寸；
- endpoint 配置：16GB 档（A4000/A4500，$0.58/h，源图 ≤2048 + tiling 够用）、
  `workers_min=0, workers_max=3, idle_timeout=5s`、FlashBoot 开。

## 9. 失败与降级

原则：**超分失败绝不吞掉一次已付费的上游生成**。

- RunPod 调用失败/超时（一次重试后）：返回上游原图不改任何字段，打
  `image_upscale_degraded` 日志与计数。计费自动自洽 —— sub2api 模拟路径解码
  **实际**像素（1K）→ 客户按 1K 计费，不多收；直连 new-api 的用户按请求 4K
  预扣但拿到 1K 图，属于降级损益，靠告警压发生率（见 §11），不做退款联动
  （第一期）；
- 结果校验失败（尺寸不符/解码失败）：同上降级；
- 对象存储不可用：进入超分模式前探测性失败即降级为"native-only 选路不变更"
  已来不及（已按降档生成），同样走原图返回；
- 上游本身失败：现行错误路径不变，不引入新行为；
- 配置解析失败：§4 的 fail-open，规则视同不存在。

## 10. 计费一致性（核验结论 + 防漂移要求）

| 计费面 | 机制 | 结论 |
|---|---|---|
| sub2api → 客户（主路） | 账号 1132 sim=on，解码实际像素判档 | 4K 正确计费；降级时自动按 1K |
| sub2api 兜底路径 | 读响应声明 size | §8 改写后正确 |
| sub2api 形状逃逸 | stream/n>1/mask/webp… 不可模拟 | §6 谓词使这类请求根本不超分 |
| new-api → 直连第三方 | `ModelPriceHelper` 早于降档、按原始请求 | 按用户请求的 4K 计费，正确 |
| gemini/大香蕉系 | 不在模拟模型白名单，平价按次 | 第一期不给这些渠道配超分规则 |

防漂移：§6 谓词函数的注释必须双向指认 sub2api
`openAIImagesRequestSimulatable` / `isSimulatableOpenAIImagesModel`，与
`image_size_tier.go` 头部"两侧必须同步"的既有约定同款。sub2api 侧任何放宽
模拟资格的改动都要同步放宽此谓词（放宽方向安全：多超分不漏账；反向收紧才危险）。

遗留提示（不在本设计范围）：sub2api 已知 bug"按次分组被 ImageUsageSimulated
强制 token 计费绕过分组 image_price"（未修）。超分不引入新问题，但 4K 的
token 量显著高于 1K，走该 bug 路径的分组账单会相应变大 —— 上线前确认受影响
分组的定价可接受。

## 11. 观测

- 日志：`image_upscale.start/done/degraded`，字段含渠道 id、规则、源/目标尺寸、
  RunPod job id、各阶段耗时（存储上/下行、排队、推理）；
- 渠道日志 `logContent`：`大小 3840x2160（1K 超分）, 品质 …`；
- 计数：`image_size_via_upscale`（选路命中派生档）、`image_upscale_degraded`
  （降级）——降级率 >5%（滑动 10 分钟）触发告警（接入现有 TG 告警通道）；
- RunPod 侧：worker 日志确认 FlashBoot 快照命中（冷启动应 <1s，持续 >10s 说明
  懒加载回归）。

## 12. 成本模型

- GPU：16GB flex $0.58/h；1K→4K 单张（热）≈ 3s 计费 ≈ $0.0005；含 idle 尾巴
  与冷启动摊销按 $0.001–0.002/张 预算；
- 存储/流量：R2 出流量免费，存储量（1 天 TTL）忽略不计；
- 对照：上游原生 4K 与 1K 的差价即毛利来源，GPU 成本低两个数量级。

## 13. 测试策略

**dto 单测**（`image_size_tier_upscale_test.go`）：
- 校验矩阵：合法/非法 from/to、空 allowed+upscale、to∈allowed、垃圾 JSON fail-open；
- `AllowWithUpscale`：宽松派生边界（native_max、to、to+1、eligible=false）；
- 尺寸映射：六比例 × 三档 × 转置、"4K" 字面、表外尺寸、精确目标。

**relay 单测**：
- 出站降档：常规路径 / passthrough 路径 / 原始 `imageReq` 未被污染；
- 回程改写：mock 存储 + mock RunPod，验证 b64 替换、声明 size 改写、
  Content-Length 清除、各失败分支降级返回原图。

**worker 侧**（worker 仓库内）：handler 本地跑 `python rp_handler.py --test_input`，
校验输出尺寸与格式；FlashBoot 预热路径靠部署后日志验收（§11）。

**集成验收**（灰度渠道）：克隆一条现有 1K 渠道 → 配 `1K→4K` → 低优先级 +
`weight=0` 手动打流 → 验证：客户拿到 2880x2880/3840x2160 PNG、sub2api 日志按
4K 档计费、new-api 渠道日志带超分标记、拔掉 RunPod key 验证降级路径。

## 14. 分期与明确不做

第一期交付：§4–§13 全部。

明确不做（本期）：
- chat completions 出图路径的超分（大香蕉系主流量）；
- stream / partial_images 的超分；
- gemini 系模型的超分规则与高清定价；
- 降级时对直连用户的自动退款；
- B 类扩散重绘（Magnific 风格加细节）——接口留白：`ImageUpscaleRule` 未来可加
  `engine` 字段，RunPod 侧另起 endpoint，本期不设计。

## 勘误（2026-08-18，实施期）

1. **edits 退出本期范围**：§1/§6 中 `/v1/images/edits` 的超分支持移入 §14"明确不做"。原因：edits 的两条 adaptor 转换路径（multipart 逐字复制与 JSON 逐字复制）均不读取降档后的 `request.Size`，降档尺寸无法抵达上游；资格谓词已按路径排除 edits，其行为回到纯原生。后续迭代需在 adaptor 层实现 size 改写后方可纳入。
2. **§8 论证修正**：sub2api 非模拟兜底路径实际只读 `data[].size` 项级声明，不读根级 `size`；无声明 size 时回落请求 size 判档，结论（改写后判档正确）不变，但"根级 size 兜底"论证前提有误。实现同时改写根级与项级，行为安全。
3. **总开关语义补强**：路由派生可达集同样受 `IMAGE_UPSCALE_ENABLED`（经 `GetImageUpscaler() != nil`）约束，配置了 upscale 规则但模块未启用的渠道不派生，防止"配置先于开关落地"的失配窗口。
4. **新增环境变量**：`IMAGE_UPSCALE_MAX_CONCURRENCY`（默认 4）——超分回程并发上限，防大 body 并发放大（本机曾有 global_oom 前科）。

## 增补二（2026-08-18）：edits 纳入超分

背景：4K 流量 95%+ 是 `/v1/images/edits`（48h 内 185 承接 122 单/3h），全部原生直通，超分毛利未覆盖。
当初排除 edits 的四项风险重估：①上游无视降档 size 出怪尺寸 → 已被 worker 双向重采样+精确目标
消解；②需侵入共享 adaptor → 改在 ImageHelper 层改写输入（JSON: `c.Set(common.KeyRequestBody)`
重写缓存体；multipart: 解析后改 `c.Request.MultipartForm.Value["size"]`），adaptor 逐字段复制机制
自然带出，公共代码零改动；③JSON mask 检测缺失 → 本次补齐；④测试面 → 照单全测。
196 上游 48h 内成功承接 824 单 edits，能力已证。

范围与口径：
- 放行条件 = 现谓词 + 路径扩展到 edits + **无 mask**（multipart 的 mask 文件检测已有，
  补 JSON body 的 `mask` 字段检测）。与 sub2api `HasMask` 拒模拟的口径对齐：带 mask 的
  请求 sub2api 不按像素计费，超分即漏账，继续走原生。
- passthrough + multipart 组合仍不超分（透传体无法安全重组，维持现状跳过）。
- 出站降档：非透传 JSON edits 重写缓存 body 的 size；非透传 multipart 改表单 size 值；
  透传 JSON 沿用 sjson。回程超分/规整逻辑不变。
