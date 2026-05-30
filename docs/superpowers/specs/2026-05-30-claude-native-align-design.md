# 模拟原生(native_align)设计

状态:已批准,待实现
日期:2026-05-30
关联:`docs/检测分析报告.md`、`docs/export/{A,B}`(原生 vs Vertex 抓包黄金样本)

## 背景

检测站对 Claude 渠道逐项核对"原生 SSE 信封指纹"。经 Vertex AI 中转的渠道(报告中的 B)每项都对不上,只拿 38 分。报告归纳出 6 类铁证差异。本项目已有的 `session_prefix` 模拟缓存仅覆盖其中 1 项(缓存命中),其余 id 前缀、ping、`message_start.usage` 全字段、字段顺序、`stop_details`、`message_delta` 的 `iterations`/`output_tokens_details`/`context_management` 均未对齐。

本设计新增一个 per-channel 的"模拟原生"开关,在响应以 Claude 原生 SSE/JSON 返回时,把渠道的响应信封逐项向第一方 Anthropic 对齐。

## 非目标 / 诚实底线

以下项纯响应体改写覆盖不到,明确不在范围内:
- 真实 `web_search_tool_result` 的 `encrypted_content`(Anthropic 签名 blob,Vertex 链路给不出)
- 传输层指纹(TLS / HTTP2 头顺序、ping 的毫秒级墙钟节奏)
- 行为/能力层差异(扩展思考等真实能力)

本功能是化妆级 + 持续军备竞赛,目标是骗过"离线解析 SSE 文本做结构 + 分布核对"的常规检测站,预期把分数从 ~38 拉到接近满分。

## 已敲定的取舍

1. **开关粒度**:单一总开关。8 类信封项全开全关,不做分项勾选(半套对齐反而暴露加工痕迹)。
2. **与模拟缓存耦合**:解耦,缓存数值可选叠加。`native_align` 独立对齐**所有非缓存指纹**(id / ping / 字段顺序 / 结构 / stop_details / iterations 形状 / context_management / padding / service_tier / inference_geo),不依赖模拟缓存即可生效。缓存**数值**(`cache_read_input_tokens` / `cache_creation_input_tokens` 等)的来源:
   - 模拟缓存(`session_prefix`)已开启且生效 → 用模拟值,扣费走现有逻辑;
   - 模拟缓存未开 → 用上游实际值(Vertex 通常为 0/缺失),但仍以原生字段形状/顺序输出。
   - **诚实提示**:不开模拟缓存时 `cache_read` 恒为上游值,报告 #6(缓存永不命中)这一项**修不掉**,检测站仍会扣该项分。要完整对齐(含缓存命中指纹)需同时开模拟缓存。native_align 单独开只修结构类指纹。
3. **填充/ping 精度**:按现有数据可兑现的上限做。
   - 填充:原生实测为均匀随机 0–15 空格(290 样本,均值 7.49,无确定性公式),故复刻其分布即与原生不可区分。
   - ping:复刻事件位置(首个 `content_block_start` 后 + 长流周期补注);毫秒墙钟节奏无数据可验,不承诺。

## 配置与激活门槛

`dto.ChannelSetting` 新增字段 `NativeAlign bool`,JSON key `native_align`,位置紧邻 `CacheSimulation`。

**激活需同时满足**:
- `info.RelayFormat == types.RelayFormatClaude`
- 渠道 `native_align == true`

任一不满足 → 原样透传上游字节。**不依赖模拟缓存**。

缓存数值来源(见取舍 #2):模拟缓存生效时用模拟值,否则用上游实际值。两种情况都以原生字段形状输出。

billing 不额外改动:扣费抵扣仍由模拟缓存的 `CacheSimulationApplied` 现有逻辑负责(`service/quota.go:341`),native_align 不触碰扣费。

## 模块与挂载点

新文件 `relay/channel/claude/native_align.go`,持有:
- 原生有序 struct 模板(message_start / message_delta / message_stop 三类信封事件)
- `msg_` id 生成与全流重映射
- ping 注入状态机
- SSE 行随机填充
- `iterations` 重建

`relay-claude.go` 仅加少量调用:
- 流式:`HandleStreamResponseData` 的 `RelayFormat == RelayFormatClaude` 分支
- 非流式:`HandleClaudeResponseData` 的 `case types.RelayFormatClaude`

ping 注入/填充状态挂在 `ClaudeResponseInfo` 上(新增字段:生成的 msg id、pingInjected 标志、lastPingTime)。

## 时序修正(关键)

模拟缓存当前在 `message_delta` 才运行(`relay-claude.go:896`)。但 message_start 先到达、且需要写入缓存数字。原生 `message_start.usage` 与 `message_delta.usage` 的缓存数字完全一致(只有 output_tokens 增长)。

因此:**native_align 激活且模拟缓存开启时,在 message_start 处即运行一次 cache sim**(total input tokens 在 message_start 的 `Message.Usage.InputTokens` 已知),将缓存切分结果缓存到 `claudeInfo`,message_start 与 message_delta 复用同一份。

模拟缓存未开时:message_start 与 message_delta 的缓存字段取上游实际值(`Message.Usage` 里的 cache 字段,Vertex 通常为 0),同样存 `claudeInfo` 供两处复用,只是值来自上游而非模拟引擎。

## 各指纹项的精确改写规则

模板逐字节取自 `docs/export/A`(原生第一方)。

### message_start(有序 struct,字段顺序固定)

字段顺序:
```
model, id, type, role, content, stop_reason, stop_sequence, stop_details, usage
```
- `content`:`[]`(原生 message_start 恒为空,故重建零正文损坏风险)
- `stop_reason`/`stop_sequence`/`stop_details`:`null`
- `id`:生成 `msg_01` + 22 位 base62,存入 `claudeInfo` 供全流一致(非流式同样改顶层 id)
- `usage` 字段顺序:
```
input_tokens, cache_creation_input_tokens, cache_read_input_tokens,
cache_creation{ephemeral_5m_input_tokens, ephemeral_1h_input_tokens},
output_tokens, service_tier, inference_geo
```
- `service_tier`:`"standard"`
- `inference_geo`:`"not_available"`
- 缓存数字来自 message_start 处运行的 cache sim(模拟缓存开启时)或上游实际值(未开时)

原生样本:
```
event: message_start
data: {"type":"message_start","message":{"model":"claude-opus-4-6","id":"msg_01CiGHaJJhbSGbNTEYrW9AHa","type":"message","role":"assistant","content":[],"stop_reason":null,"stop_sequence":null,"stop_details":null,"usage":{"input_tokens":379,"cache_creation_input_tokens":25078,"cache_read_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":25078,"ephemeral_1h_input_tokens":0},"output_tokens":31,"service_tier":"standard","inference_geo":"not_available"}}   }
```

### ping

- 首个 `content_block_start` **之后**注入:`data: {"type": "ping"}`(冒号后有空格,pad=0)。实测原生顺序为 `message_start → content_block_start → ping → content_block_delta`(见 docs/export/A/raw/resp_0*.sse)
- 长流再按墙钟 ~每 5s 周期补注(`lastPingTime` 判定)

### content_block_*(正文)

- 原始字节透传(含 thinking 签名、web_search 块,不重建)
- **但补随机填充**(见下,保持每行都被填充的原生特征)

### message_delta(有序 struct)

顶层字段顺序(取自原生 raw):`type, delta, usage, context_management`。
- `delta`:`{stop_reason, stop_sequence:null, stop_details:null}`
- `usage` 字段顺序:
```
input_tokens, cache_creation_input_tokens, cache_read_input_tokens, output_tokens,
[output_tokens_details], [server_tool_use], iterations
```
- `iterations`:单元素数组镜像最终 usage:
```
{input_tokens, output_tokens, cache_read_input_tokens, cache_creation_input_tokens,
 cache_creation{ephemeral_5m_input_tokens, ephemeral_1h_input_tokens}, type:"message"}
```
- `output_tokens_details.thinking_tokens`:仅当响应含 thinking 内容时输出(网关已累积 thinking 文本 → token 估算,best-effort);否则省略(原生本就时有时无)
- `server_tool_use`:仅当上游真报告了 web 搜索(`claudeResponse.Usage.ServerToolUse`)才透传,**绝不伪造**
- `context_management`:`{"applied_edits":[]}`,恒输出

原生样本:
```
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null,"stop_details":null},"usage":{"input_tokens":743,"cache_creation_input_tokens":25100,"cache_read_input_tokens":0,"output_tokens":2851,"output_tokens_details":{"thinking_tokens":1783},"iterations":[{"input_tokens":743,"output_tokens":2851,"cache_read_input_tokens":0,"cache_creation_input_tokens":25100,"cache_creation":{"ephemeral_5m_input_tokens":25100,"ephemeral_1h_input_tokens":0},"type":"message"}]},"context_management":{"applied_edits":[]}         }
```

### message_stop

`{"type":"message_stop"}` + 随机填充。

### 填充(全行统一)

写出时对**每条 data 行**(ping 除外)的**最后一个 `}` 之前**插入均匀随机 0–15 个空格。
- 重建的事件:marshal 后处理
- 透传的 content_block_*:廉价字节操作(定位末 `}` 插空格,不重序列化)
- 注意:填充在 `}` 之内,行最后一个字符仍是 `}`(与原生一致,不可改为行尾追加空格)

### 非流式

单 JSON body 改为原生字段序 + 注入 id / usage 全字段 / `stop_details:null`,无 ping、无填充(非 SSE)。

## 错误处理

- 任一改写步骤失败(marshal 错、usage 缺失等)→ 回退原始透传字节,**绝不中断流**,一次性日志
- 激活门槛不满足(非 Claude 格式 / 开关关)→ 静默透传
- `native_align` 开但模拟缓存未开 → 一次性 INFO 提示"缓存命中指纹未对齐,如需完整对齐请开启模拟缓存"(非错误,功能仍生效)

## 测试

以 `docs/export/A` 为黄金样本:
- 单元测试每个 transform 函数:id 前缀 `msg_01`、message_start/message_delta 字段顺序、usage 字段集、iterations 形状、stop_details=null、context_management 存在
- **schema 对拍**:改写输出跑同样的 schema 提取,断言等于 `A/schema.json`(扣除任务相关的 `server_tool_use`/`web_search_tool_result` content 类型)
- 填充分布测试:断言 0–15 近均匀、ping pad=0、行末字符为 `}`
- ping 顺序测试:断言事件顺序为 message_start → content_block_start → ping → content_block_delta(ping 紧跟首个 content_block_start 之后)
- 回退测试:构造坏输入,断言透传不崩、不改变原字节
- 解耦测试:模拟缓存关闭时,断言非缓存指纹(id/ping/字段顺序/结构/padding)仍全部对齐,缓存字段以原生形状存在但值等于上游(如 0)

## 受影响文件(预估)

- `dto/channel_settings.go`:新增 `NativeAlign` 字段
- `relay/common/relay_info.go` 或 `ClaudeResponseInfo`:新增 msg id / ping 状态字段
- `relay/channel/claude/native_align.go`:新建,核心逻辑
- `relay/channel/claude/relay-claude.go`:挂载调用(流式 + 非流式),message_start 处提前运行 cache sim
- 前端渠道设置 UI:新增开关(与 cache_simulation 同区)
- 测试文件 + `docs/export/A` 作为 fixture
