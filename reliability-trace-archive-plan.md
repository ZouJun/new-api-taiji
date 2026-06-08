# AWS Claude Timeout, Trace-Id, and Archive Implementation Plan

## 0. 这份文档是干什么的

这是一份给当前项目维护者看的“大白话实施方案”。

目标只有 3 个：

1. 说清楚现在请求上游 AWS Claude 时，到底有没有超时机制，现状是怎么实现的。
2. 设计一套完整的“请求/响应归档方案”，把客户请求内容、返回给客户的内容，按配置选择落本地或上传 Azure Blob。
3. 设计一套客户 `Trace-Id` 贯穿全链路的方案，做到“进来能拿到，链路里能传下去，日志里能看到，数据库里能查到”。

这份文档默认的大前提是：

- 系统要扛住 `8000-15000 RPM`
- 设计优先级是：`稳定 > 容错 > 性能 > 可追溯`
- 文档要让刚接手项目的人也能看懂

注意：这份文档先讲“怎么做”，不等于这些代码已经全部实现。

---

## 1. 先说结论

### 1.1 第 1 点结论：当前 AWS Claude 调用“有超时机制”，但还不够理想

现状里，AWS Claude 相关调用已经有两层超时保护：

1. **HTTP Client Timeout**
2. **AWS SDK 调用 Context Timeout**

当前代码落点：

- `/Users/zf/Project/new-api/service/http_client.go`
- `/Users/zf/Project/new-api/relay/channel/aws/relay-aws.go`
- `/Users/zf/Project/new-api/common/constants.go`

但现在有两个明显问题：

1. **超时是全局的 `common.RelayTimeout`，不是按 channel 动态控制**
2. **AWS SDK 的 context 现在是从 `context.Background()` 起的，不是从当前请求的 `c.Request.Context()` 往下传**

这两个问题会带来几个风险：

- 某个渠道想单独收紧或放宽超时时间，做不到
- 客户端已经断开连接时，上游调用不一定能第一时间感知
- 后续做链路级 Trace、归档、熔断时，context 信息不完整

所以第 1 点的改造方向很明确：

- 在 `channel.setting` 里增加超时配置
- 当 `channel.setting` 有配置时，必须真正生效
- AWS SDK 超时 context 改成从 `c.Request.Context()` 继承

### 1.2 第 2 点结论：请求/响应归档不能直接同步写 Azure，必须做异步隔离

8000-15000 RPM 不是小流量。

如果每个请求都在主链路里同步写 Azure Blob，会带来三个直接后果：

1. 客户响应延迟变高
2. Azure 抖动时，会拖垮主业务链路
3. 流式响应更容易被拖慢甚至打乱

所以归档方案必须坚持：

- **默认异步**
- **主链路成功不依赖归档成功**
- **本地 / Azure 只通过配置切换**
- **不改现有表逻辑来决定归档落哪里**

### 1.3 第 3 点结论：客户 Trace-Id 应该作为“外部关联 ID”，和系统自己的 requestId 分开

这个项目已经有系统自己生成的 `requestId`。

它和客户传进来的 `Trace-Id` 不是一个东西：

- `requestId`：系统自己生成，可信，适合内部排障
- `Trace-Id`：客户传入，不可信，需要清洗，适合跨系统对账和排查

所以正确做法不是二选一，而是两个都保留：

- `requestId` 继续保留
- 新增客户 `Trace-Id` 贯穿上下文、日志、`logs.other`、归档文件命名

---

## 2. 现状说明：第 1 点现在到底有没有超时机制

## 2.1 全局超时配置在哪里

当前全局超时变量在：

- `/Users/zf/Project/new-api/common/constants.go`

里面有：

- `RelayTimeout int // unit is second`

这个值现在是项目里很多上游调用共享的全局超时秒数。

## 2.2 普通 HTTP Client 是怎么用这个超时的

当前 HTTP Client 初始化在：

- `/Users/zf/Project/new-api/service/http_client.go`

现状逻辑可以概括成一句话：

- 如果 `common.RelayTimeout != 0`，那么 `http.Client.Timeout = RelayTimeout`

也就是说：

- 所有复用默认 HTTP Client 的链路，会被这个全局超时限制
- 代理 Client 也是同样逻辑

这属于**网络层总超时**。

它的特点是：

- 配得简单
- 对普通 HTTP 请求有效
- 但它是“整个 HTTP 请求生命周期超时”
- 它不够细，也不够灵活

## 2.3 AWS Claude 额外还有一层 SDK 调用超时

AWS Claude 调用在：

- `/Users/zf/Project/new-api/relay/channel/aws/relay-aws.go`

关键逻辑是：

- `newAwsInvokeContext()`
- `awsHandler()`
- `awsStreamHandler()`

现状逻辑可以翻译成人话：

1. 调 AWS 之前，先调用 `newAwsInvokeContext()`
2. 如果 `common.RelayTimeout <= 0`，就返回一个不带超时的 context
3. 如果 `common.RelayTimeout > 0`，就 `context.WithTimeout(..., RelayTimeout 秒)`
4. 后面把这个 context 传给：
   - `InvokeModel(...)`
   - `InvokeModelWithResponseStream(...)`

所以答案是：

**有超时机制，而且不止一层。**

## 2.4 为什么说“有，但还不够好”

因为它现在有 4 个不理想点。

### 2.4.1 不是按 channel 动态配置

用户已经明确要求：

- **要针对 channel 表动态设置超时时间**
- **只要 `channel_settings` 有设置，就需要有超时机制**

但当前实现只有全局 `common.RelayTimeout`，所以不满足这个要求。

### 2.4.2 context 起点不对

现在 `newAwsInvokeContext()` 是从 `context.Background()` 起新的 context。

这会导致：

- 客户端断开连接了，AWS 调用未必马上被取消
- Gin request 上下文里已有的信息，不能自然往下传
- 以后想把 Trace、链路信息、取消信号串起来，会很别扭

更合理的做法应该是：

- 用 `c.Request.Context()` 作为父 context

这样：

- 客户端取消时，上游请求能更快感知
- 整条调用链的上下文能串起来

### 2.4.3 流式超时和非流式超时要分清语义

非流式请求里，超时含义比较直观：

- 总调用时间超过阈值，就超时失败

但流式请求会复杂一些，因为流式常常持续更久。

所以这里要提前约定清楚：

- 当前第一版，先把超时当成**整个上游调用的总预算**
- 后续如果业务需要，再区分：
  - 首包超时
  - 流空闲超时
  - 整体总超时

当前需求没有强制要求拆成三种，所以第一版不要把复杂度做太高。

### 2.4.4 两层超时叠加，必须定义优先级

现在已经有：

1. HTTP Client Timeout
2. AWS SDK Context Timeout

如果以后再加 channel 级超时，就会出现“到底谁说了算”的问题。

所以必须把优先级先定死，避免维护者以后看不懂。

---

## 3. 第 1 点具体实施方案：AWS Claude 超时机制怎么改

## 3.1 配置放哪里

不新增 `channels.timeout` 物理列。

原因很简单：

- 用户已经认可走 `channel_settings`
- 这类渠道个性化配置，本来就适合放 `setting` JSON
- 避免额外跨数据库迁移复杂度

落点：

- `/Users/zf/Project/new-api/dto/channel_settings.go`
- `/Users/zf/Project/new-api/model/channel.go`

建议在 `dto.ChannelSettings` 里新增：

- `RelayTimeoutSeconds *int  'json:"relay_timeout_seconds,omitempty"'`

这里要用 `*int`，不用 `int`，原因很重要：

- `nil` = 没配置
- `0` = 明确配置为 0

虽然最终策略里我们仍然建议：

- `<= 0` 视为“不启用显式超时”

但用指针可以明确区分“用户没填”和“用户明确填了值”。

## 3.2 超时值怎么解析

建议统一做一个“有效超时解析函数”，比如：

- `ResolveRelayTimeoutSeconds(info *relaycommon.RelayInfo) int`

优先级固定为：

1. `channel.setting.relay_timeout_seconds`
2. `common.RelayTimeout`
3. `0` 表示不显式设超时

翻译成人话就是：

- 渠道自己配了，就听渠道的
- 渠道没配，退回全局配置
- 全局也没配，就不额外包 timeout

这样好处是：

- 不破坏现有全局行为
- 支持精细到 channel
- 新人一眼就知道规则

## 3.3 具体改哪些代码点

### 改动点 A：把 channel setting 带到 relay info

现状里 `RelayInfo` 已经带有：

- `ChannelSetting dto.ChannelSettings`

所以这部分主路径已经有基础了，重点是保证渠道选中后，setting 能正常解析出来并传到 relay 层。

主要关注：

- `/Users/zf/Project/new-api/middleware/distributor.go`
- `/Users/zf/Project/new-api/relay/common/relay_info.go`
- `/Users/zf/Project/new-api/model/channel.go`

### 改动点 B：改造 AWS invoke context

当前 `newAwsInvokeContext()` 建议改成类似下面的职责：

1. 接收 `*gin.Context` 和 `*relaycommon.RelayInfo`
2. 先取 `baseCtx := c.Request.Context()`
3. 根据解析后的超时值决定是否 `WithTimeout`
4. 返回新的 `ctx, cancel`

也就是从：

- “只看全局值”

改成：

- “优先看 channel setting，再回退全局值，并且从 request context 继承”

### 改动点 C：统一日志打印有效超时

建议在 AWS 发起调用前打一次 debug/info 级日志，打印：

- channelId
- requestId
- customer trace id（如果有）
- effective timeout
- is stream

这样线上排查时，很容易回答：

- 为什么这个请求 30 秒超时
- 为什么另一个渠道是 120 秒

## 3.4 为什么不建议直接改全局 HTTP Client 为“每个请求一个超时”

因为当前默认 HTTP Client 是共享复用的。

如果把 `http.Client.Timeout` 设计成“每个请求动态变”，你会遇到两个问题：

1. 共享 client 不适合每次临时改结构体字段
2. 连接池复用逻辑会变复杂，容易引入并发风险

所以更稳妥的做法是：

- 共享 HTTP Client 继续保留全局默认超时能力
- channel 级精细超时主要通过**请求级 context timeout**控制

这就是“粗粒度全局默认 + 细粒度请求级覆盖”。

对 8000-15000 RPM 来说，这样更稳。

## 3.5 超时失败时应该怎么表现

统一原则：

- 对客户返回明确超时错误
- 日志里能看出是哪个 channel、哪个 requestId、哪个 Trace-Id、超时值是多少
- `logs.other` 里保留超时相关元数据

建议在错误元数据里增加类似字段：

- `timeout_seconds`
- `timeout_source`：`channel_setting` 或 `global`
- `timeout_stage`：`upstream_invoke`

这样后面查日志表时，不需要翻代码就知道是怎么超时的。

## 3.6 流式请求怎么处理

第一版先遵循一个简单规则：

- **只要 `channel_settings` 配了超时，流式和非流式都必须使用这个超时预算**

也就是说：

- 流式请求不是“无限流”
- 到了预算上限，就应该结束并报超时

原因：

1. 用户已经明确说“只要 `channel_settings` 有设置，就需要有超时机制”
2. 对高并发系统来说，无上限流比“超时过早”更危险
3. 第一版先保证系统有边界，再谈更复杂的流式空闲策略

## 3.7 第 1 点实施后的收益

实施后，超时行为会变成：

- 每个 channel 都可以有自己的超时预算
- 没配的 channel 继续走全局默认
- 客户断开连接时，上游调用更容易及时取消
- 错误日志里能明确追到超时配置来源

这满足“稳定、容错、性能、能溯源”的大前提。

---

## 4. 第 2 点具体实施方案：请求/响应归档怎么做

## 4.1 先讲清楚目标

需求不是简单的“把日志写一下”，而是：

- 记录客户发来的请求内容
- 记录最后返回给客户的内容
- 同时支持：
  - 非流式
  - 流式
- 可配置：
  - 存本地
  - 上传 Azure Blob

而且必须注意：

- **不能为了归档把主链路拖慢**
- **不能因为 Azure 挂了就让客户请求失败**
- **不能把大 payload 直接塞进数据库**

## 4.2 为什么不能把完整 payload 存数据库

因为数据库里的 `logs` 表更适合存“元数据”，不适合存“完整大对象”。

如果把完整请求体、完整响应体塞到 DB，会有这些问题：

1. 表行会变得很大
2. 写入压力明显增大
3. 查询日志会变慢
4. 保留周期不好做
5. 流式内容更难处理

所以正确做法是：

- **DB 只存索引和元数据**
- **完整 payload 存文件系统或 Blob**

## 4.3 总体架构：主链路采集，异步归档

建议架构分 4 层：

1. **采集层**
2. **归档任务层**
3. **存储后端层**
4. **日志回写层**

翻译成人话：

### 第 1 层：采集层

主链路只负责把本次请求/响应采到内存或临时对象里，不做重存储、不做重上传。

### 第 2 层：归档任务层

主链路结束后，把一个“归档任务”丢进有界队列。

### 第 3 层：存储后端层

后台 worker 从队列拿任务，按配置写：

- 本地目录
- Azure Blob

### 第 4 层：日志回写层

把归档结果摘要写进 `logs.other`，比如：

- archive backend
- object path
- hash
- size
- status
- error

这样做的核心价值是：

- 主链路轻
- 存储故障和客户响应隔离
- 归档失败也能追到原因

## 4.4 配置原则：只通过配置切换本地 / Azure

用户已经明确要求：

- **不用调整代码现有的表逻辑**
- **只需要根据配置选择存本地或上传 Azure Blob**

所以归档选择必须来自配置，而不是表。

建议引入配置项：

- `ARCHIVE_ENABLED=true|false`
- `ARCHIVE_BACKEND=local|azure`
- `ARCHIVE_LOCAL_DIR=/data/new-api/archive`
- `ARCHIVE_QUEUE_SIZE=...`
- `ARCHIVE_WORKER_COUNT=...`
- `ARCHIVE_MAX_BODY_BYTES=...`
- `ARCHIVE_GZIP_ENABLED=true|false`
- `ARCHIVE_AZURE_ACCOUNT_NAME=...`
- `ARCHIVE_AZURE_ACCOUNT_KEY=...`
- `ARCHIVE_AZURE_CONTAINER=...`
- `ARCHIVE_AZURE_ENDPOINT=...`
- `ARCHIVE_FAIL_ON_STORE_ERROR=false`

其中：

- `ARCHIVE_FAIL_ON_STORE_ERROR` 默认必须是 `false`

意思是：

- 归档失败默认不影响客户成功响应

这点对高并发稳定性非常关键。

## 4.5 文件命名怎么设计

用户要求：

- 文件名称需包含 `requestId + 客户的traceId`

建议对象路径格式：

`archive/YYYY/MM/DD/{channelId}/{requestId}__{traceIdSafe}/`

例如：

`archive/2026/06/08/123/req_abc123__trace_u_9988/`

目录下放 3 个主要文件：

1. `request.json.gz`
2. `response.json.gz` 或 `response.sse.gz`
3. `manifest.json`

如果是错误响应，也可以是：

- `response.error.json.gz`

### 为什么是“每次请求一个目录”

因为这样最直观，也最好排查：

- 一次请求一个文件夹
- 文件夹名里就能看见 requestId 和 traceId
- 新人不用额外学复杂索引规则

### 为什么不建议把 request 和 response 拼成一个大文件

因为拆开以后更利于：

- 单独看请求
- 单独看响应
- 单独做 hash 和大小统计
- 后续只补传缺失文件

## 4.6 traceId 怎么进文件名

客户传入的 Trace-Id 不能原样进文件名，必须先清洗。

建议规则：

- 只保留：`A-Z a-z 0-9 . _ -`
- 去掉空格、换行、控制字符、斜杠等危险字符
- 长度限制为 64 或 128
- 为空时使用 `no-trace`

例如：

- 原始值：`user/abc 123\n`
- 清洗后：`userabc123`

这样做的原因：

1. 防止目录穿越
2. 防止 Blob/Object name 混乱
3. 防止日志污染

## 4.7 manifest.json 里放什么

`manifest.json` 不放完整 payload，只放说明书式元数据。

建议字段：

- `request_id`
- `customer_trace_id`
- `customer_trace_id_raw_present`：是否原始头里有值
- `channel_id`
- `channel_name`
- `provider`
- `model`
- `is_stream`
- `status_code`
- `archive_backend`
- `request_file`
- `response_file`
- `request_sha256`
- `response_sha256`
- `request_bytes`
- `response_bytes`
- `gzip_enabled`
- `created_at`
- `finished_at`
- `duration_ms`
- `archive_status`
- `archive_error`

为什么要 manifest：

- 查问题时先看 manifest，不用先解压大文件
- DB 里只要存 manifest 路径，就能快速跳过去
- 以后做离线工具也方便

## 4.8 为什么推荐 gzip，而不是 zip 包

推荐：

- 每个文件单独 `.gz`

不推荐：

- 热路径里做跨请求 zip 批量打包

原因很实际：

### 单文件 gzip 的好处

1. 压缩率已经够用
2. 实现简单
3. 单次请求独立，不互相等待
4. 某一个文件坏了，不影响其他请求

### 热路径批量 zip 的坏处

1. 要等凑批次
2. 失败影响面更大
3. 排查不直观
4. 流式响应更难及时归档

所以结论很明确：

- **在线链路不做跨请求批量 zip**
- **每个对象单独 gzip**

后续如果真的要做冷数据归档压缩，应该走离线任务，不放在主链路里。

## 4.9 非流式请求怎么采集

当前项目已经有不错的基础：

- `/Users/zf/Project/new-api/common/gin.go`
- `/Users/zf/Project/new-api/service/http.go`

### 请求体采集

请求体建议复用现有 `BodyStorage` 能力。

意思是：

- 不要重复读 `c.Request.Body`
- 不要自己再搞一套“读完再塞回去”的脆弱逻辑

正确做法：

1. 从已有 `BodyStorage` 拿字节
2. 计算 request hash
3. 记录 request bytes
4. 写进归档任务对象

### 响应体采集

非流式响应通常是完整 body，可以在已有复制逻辑旁边挂一个 recorder。

核心要求：

- 不改客户看到的响应内容
- 不多发字节
- 不少发字节

简单说就是：

- 客户收到什么，归档就记录什么

## 4.10 流式请求怎么采集

流式是这个需求里最敏感的地方。

原则只有一个：

- **记录流，不要影响流**

建议做法：

1. 在流式写出路径上挂一个“旁路 recorder”
2. 每写给客户一段，就顺手把同样的字节写入 recorder buffer
3. recorder 只做轻量内存累计，不做同步上传
4. 流结束后，把累计内容封装成归档任务

要点：

- 不要在每个 chunk 到来时同步写 Azure
- 不要在每个 chunk 到来时频繁落盘
- 不要改变 flush 节奏

第一版归档的核心是“尽量还原客户实际收到的内容”，所以流式建议归档：

- 原始 SSE 文本流

文件名：

- `response.sse.gz`

这样排查最直接，后续人工打开也最容易理解。

## 4.11 大流量下怎么防止归档拖垮系统

这是整套方案最关键的部分。

一定要有下面 5 个保护：

### 保护 1：有界队列

不能无限往内存里塞归档任务。

建议：

- 用固定长度 channel 或任务队列
- 例如 `ARCHIVE_QUEUE_SIZE=5000` 这种可配置值

队列满了以后，默认策略：

- 丢弃当前归档任务
- 打日志
- 在 `logs.other` 写 `archive_dropped=true`

为什么是丢归档而不是阻塞主链路？

因为主业务优先级更高。

### 保护 2：固定 worker 数

建议后台用固定数量 worker，比如：

- `ARCHIVE_WORKER_COUNT=8`
- 或按 CPU / IO 能力配置

不要每个请求都起 goroutine 去写 Azure。

那样短时间高峰下会把：

- 内存打爆
- 网络打满
- Blob SDK 连接压垮

### 保护 3：大小上限

必须限制单次归档最大字节数。

建议配置：

- `ARCHIVE_MAX_BODY_BYTES`

超过上限时：

- 截断归档
- 在 manifest 和 `logs.other` 里记录 `truncated=true`

这样可以防止极端大响应把归档系统拖死。

### 保护 4：失败重试，但次数有限

对本地写盘和 Azure 上传都可以做有限重试，例如：

- 最多 2-3 次
- 指数退避

但不能无限重试。

否则会出现：

- 失败任务越积越多
- 队列被坏任务占满

### 保护 5：明确降级策略

归档系统出问题时，主链路必须还能跑。

默认降级顺序建议是：

1. Azure 写失败
2. 记录错误元数据
3. 请求照常结束
4. 后台监控报警

如果以后业务上真的要求“归档必须成功才算成功”，可以单独加开关，但默认不能这样设计。

## 4.12 本地存储方案怎么做

本地存储适合：

- 开发环境
- 单机排查
- 短期临时审计

写入方式：

- 按上面的目录结构落到 `ARCHIVE_LOCAL_DIR`

例如：

`/data/new-api/archive/2026/06/08/123/req_abc123__trace_u_9988/request.json.gz`

优点：

- 最简单
- 排查快
- 没有外部网络依赖

风险：

- 本地盘容量有限
- 多节点部署时，文件散在各节点上

所以多节点生产环境更建议 Azure Blob。

## 4.13 Azure Blob 方案怎么做

Azure Blob 适合：

- 多节点
- 长期留存
- 集中审计

建议封装一个统一 `Store` 接口，例如：

- `Put(ctx, objectPath string, contentType string, body []byte, metadata map[string]string) error`

再实现两套后端：

1. `local store`
2. `azure blob store`

这样业务层不需要关心到底写哪里。

Azure Blob 上传时建议做：

- 容器预先配置好
- 对象路径直接使用上文统一命名
- metadata 里补充：
  - request_id
  - trace_id
  - channel_id
  - model
  - is_stream

### 为什么不建议“先本地落盘，再批量异步上传 Azure”作为默认方案

这个方案听起来很安全，但会引入额外复杂度：

1. 多一层本地状态管理
2. 多节点文件清理更麻烦
3. 失败补偿逻辑更复杂

所以第一版建议更直接：

- backend=local 时，直接落本地
- backend=azure 时，直接异步上传 Azure

简单、清晰、可维护。

## 4.14 `logs.other` 里放什么

数据库里不要放完整 payload，只放“归档索引信息”。

建议结构大概是：

```json
{
  "customer_trace_id": "trace_u_9988",
  "archive": {
    "enabled": true,
    "backend": "azure",
    "status": "success",
    "request_object": "archive/2026/06/08/123/req_abc123__trace_u_9988/request.json.gz",
    "response_object": "archive/2026/06/08/123/req_abc123__trace_u_9988/response.sse.gz",
    "manifest_object": "archive/2026/06/08/123/req_abc123__trace_u_9988/manifest.json",
    "request_bytes": 1234,
    "response_bytes": 5678,
    "request_sha256": "xxx",
    "response_sha256": "yyy",
    "is_stream": true,
    "truncated": false,
    "dropped": false,
    "error": ""
  }
}
```

这样后面看日志表时，就能知道：

- 这条请求有没有归档
- 归档到哪里
- 大小多少
- 是否截断
- 是否丢弃
- 为什么失败

## 4.15 第 2 点实施后的收益

实施后，归档系统会具备这些特性：

- 本地 / Azure 可配置切换
- 不改现有表逻辑
- 流式和非流式都能归档
- 文件名里包含 `requestId + traceId`
- 主链路和归档链路隔离
- 高并发时有明确背压和降级策略

---

## 5. 第 3 点具体实施方案：Trace-Id 怎么贯穿链路

## 5.1 先讲清楚目标

用户要的是：

1. 从请求头获取客户 `Trace-Id`
2. 贯穿链路
3. 错误打印时如果能拿到，要打印出来
4. 同时存到 `logs.other`

这个需求看着不复杂，真正难点是：

- 不能污染现有 requestId 逻辑
- 不能把不可信头值原样乱用
- 不能靠每个业务点手写一遍日志拼接

所以必须做成“统一入口 + 统一上下文 + 统一日志增强 + 统一入库”。

## 5.2 请求头读哪个

建议：

- 主读：`Trace-Id`
- 兼容：`X-Trace-Id`

规则：

1. 有 `Trace-Id` 就优先用它
2. 没有再看 `X-Trace-Id`
3. 都没有就当空值

这样兼顾标准性和兼容性。

## 5.3 为什么必须先清洗再用

因为这是客户传进来的值，不可信。

如果不清洗，可能出问题的地方很多：

- 日志污染
- 文件名非法
- 控制字符干扰终端
- 过长字符串放大存储压力

所以建议统一清洗规则：

- 只保留 `[A-Za-z0-9._-]`
- 去掉空格、换行、制表、控制字符
- 超长截断到 64 或 128
- 清洗后为空则视为“没有 trace id”

## 5.4 Trace-Id 存哪里

建议同时存 2 个地方：

1. `gin.Context`
2. `c.Request.Context()`

原因：

### 放 gin.Context

方便现在大量现有代码直接 `c.GetString(...)` 取值。

### 放 request context

方便那些只拿 `context.Context` 的底层函数也能拿到。

例如：

- logger
- service
- future async task metadata builder

## 5.5 中间件应该放在哪

建议新增一个 middleware，位置放在：

- `RequestId` 中间件之后

顺序原因很简单：

1. 先生成系统自己的 `requestId`
2. 再解析客户 `Trace-Id`
3. 后续所有链路都同时拿得到这两个值

建议改动点：

- `/Users/zf/Project/new-api/middleware/request-id.go`
- 新增一个 trace middleware 文件，或者同目录补充新中间件

## 5.6 日志怎么打印

重点不是“去每个 `logger.LogError` 调用点手改字符串”。

那样工作量大，而且一定会漏。

正确做法是：

- 增强统一 logger

当前 logger 在：

- `/Users/zf/Project/new-api/logger/logger.go`

建议让统一日志输出从现在的：

- `时间 | requestId | msg`

变成：

- `时间 | requestId | traceId | msg`

如果没有 traceId，就打印空或 `-`。

这样任何走统一 logger 的地方，天然都带上客户 trace。

## 5.7 Gin 访问日志也要带上

当前 Gin access log 在：

- `/Users/zf/Project/new-api/middleware/logger.go`

建议一起增强，打印：

- route tag
- requestId
- customer traceId
- status
- latency
- client ip
- method
- path

这样排查 HTTP 请求时，不用再去翻普通日志和 DB 日志对齐。

## 5.8 数据库怎么存

用户已经明确说：

- 存到 `logs` 表的 `other` 字段中

所以这里**不新增列**，直接复用 `other`。

建议在以下两个入口统一追加：

- `/Users/zf/Project/new-api/model/log.go`
  - `RecordErrorLog(...)`
  - `RecordConsumeLog(...)`

做法不是让每个调用方都手工传 `customer_trace_id`，而是：

1. 在 `RecordErrorLog` / `RecordConsumeLog` 内部读取 `c` 上下文
2. 如果有 traceId，就自动合并进 `other`

建议字段名固定为：

- `customer_trace_id`

为什么字段名不要叫 `trace_id`？

因为这个项目里以后很容易出现：

- 系统 trace
- 上游 trace
- 客户 trace

写成 `customer_trace_id` 最清楚，不容易混。

## 5.9 错误日志里要怎么体现

建议错误日志至少做到两个层面都有 trace：

### 层面 1：文本日志

统一 logger 输出带：

- requestId
- customer traceId

### 层面 2：数据库日志

`logs.other` 带：

- `customer_trace_id`

这样无论是：

- 看终端日志
- 看文件日志
- 查数据库日志

都能追到同一个客户请求。

## 5.10 对归档命名也有帮助

第 2 点要求文件名里要包含：

- `requestId + traceId`

那第 3 点做好之后，归档模块就可以直接复用同一个 traceId 获取逻辑，不需要自己再从 header 读一遍。

这很重要，因为：

- 一个值只在一个地方解析
- 清洗规则也只有一份
- 不会出现日志里的 traceId 和文件名里的 traceId 不一致

## 5.11 第 3 点实施后的收益

实施后，链路表现会变成：

1. 客户带着 `Trace-Id` 进来
2. 中间件统一清洗并挂到上下文
3. 普通日志自动打印
4. Gin 访问日志自动打印
5. `RecordErrorLog` / `RecordConsumeLog` 自动入 `logs.other`
6. 后续归档命名和 manifest 也复用同一个值

最后就能真正做到：

- 从 HTTP 请求
- 到普通日志
- 到数据库 logs
- 到归档目录

整条链路都能顺着同一个 trace 找下去。

---

## 6. 三个点合起来后的完整链路

一个请求进来以后，理想链路应该是这样：

1. 系统生成自己的 `requestId`
2. 从 header 读取客户 `Trace-Id`
3. 清洗后挂到上下文
4. distributor 选中 channel，并解析 `channel.setting`
5. relay 层拿到 channel 级有效超时
6. 调 AWS Claude 时，从 `c.Request.Context()` 派生 timeout context
7. 请求和响应在主链路中被轻量采集
8. 客户响应先正常返回
9. 归档任务异步进入队列
10. worker 按配置写本地或 Azure Blob
11. `logs.other` 记录 trace、archive 元数据、失败原因
12. 后续排查时，可通过 `requestId` 或 `customer_trace_id` 串起所有证据

一句话总结：

- `requestId` 负责系统内部可信定位
- `Trace-Id` 负责客户跨系统关联
- `archive` 负责完整证据留存
- `timeout` 负责给上游调用加边界

这 3 点是互相配合的，不是 3 个孤立需求。

---

## 7. 具体建议改动文件清单

下面是建议改动的主要文件，不代表每个文件都一定要大改，但这是最核心的落点。

### 第 1 点：超时

- `/Users/zf/Project/new-api/dto/channel_settings.go`
  - 增加 `relay_timeout_seconds`
- `/Users/zf/Project/new-api/model/channel.go`
  - 确保 setting JSON 能正确读写新字段
- `/Users/zf/Project/new-api/relay/channel/aws/relay-aws.go`
  - 改造 `newAwsInvokeContext`
  - 使用 `c.Request.Context()`
  - 应用 channel 级有效超时
- `/Users/zf/Project/new-api/relay/common/relay_info.go`
  - 确保 relay 层可拿到 `ChannelSetting`

### 第 2 点：归档

- `/Users/zf/Project/new-api/common/gin.go`
  - 复用请求 body 存储能力
- `/Users/zf/Project/new-api/service/http.go`
  - 挂非流式响应 recorder
- `/Users/zf/Project/new-api/relay/channel/openai/relay-openai.go`
  - 流式 recorder 接入点之一
- `/Users/zf/Project/new-api/relay/channel/claude/relay-claude.go`
  - 流式 recorder 接入点之一
- `/Users/zf/Project/new-api/relay/channel/aws/relay-aws.go`
  - AWS 流式 recorder 接入点之一
- 新增建议目录：
  - `/Users/zf/Project/new-api/service/archive/`
  - `/Users/zf/Project/new-api/service/archive/store_local.go`
  - `/Users/zf/Project/new-api/service/archive/store_azure_blob.go`
  - `/Users/zf/Project/new-api/service/archive/queue.go`
  - `/Users/zf/Project/new-api/service/archive/manifest.go`

### 第 3 点：Trace-Id

- `/Users/zf/Project/new-api/common/constants.go`
  - 增加 trace 相关 key 常量
- `/Users/zf/Project/new-api/middleware/request-id.go`
  - 保持 requestId 逻辑，配合 trace middleware 顺序
- 新增建议文件：
  - `/Users/zf/Project/new-api/middleware/customer-trace.go`
- `/Users/zf/Project/new-api/middleware/logger.go`
  - Gin access log 带 traceId
- `/Users/zf/Project/new-api/logger/logger.go`
  - 统一 logger 输出带 traceId
- `/Users/zf/Project/new-api/model/log.go`
  - `RecordErrorLog` / `RecordConsumeLog` 自动把 `customer_trace_id` 写入 `other`

---

## 8. 风险点和处理建议

## 8.1 风险：Trace-Id 被客户塞入奇怪字符

处理：

- 必须统一清洗
- 统一长度限制
- 文件名和日志都只使用清洗后的值

## 8.2 风险：流式响应太大，归档占用内存

处理：

- 配 `ARCHIVE_MAX_BODY_BYTES`
- 超过即截断
- 记录 `truncated=true`

## 8.3 风险：Azure Blob 抖动

处理：

- 异步 worker
- 有界队列
- 有限重试
- 默认不影响客户成功响应

## 8.4 风险：队列被打满

处理：

- 直接丢归档任务
- 打日志
- `logs.other` 记录 `archive_dropped=true`

## 8.5 风险：流式请求超时后，调用方误以为是归档问题

处理：

- 超时和归档错误要分开记录
- `logs.other` 分开写：
  - `timeout_*`
  - `archive.*`

## 8.6 风险：新同学看不懂“到底值从哪来”

处理：

- 关键字段统一命名
- 优先级规则固定写入文档
- 日志里打印：
  - requestId
  - customer_trace_id
  - channelId
  - effective_timeout

---

## 9. 推荐实施顺序

如果实际开始编码，我建议按下面顺序做。

### 第一步：先做 Trace-Id

因为它侵入性小，但会给后面的超时排查和归档命名提供基础。

先做完后，日志排障能力会立刻提升。

### 第二步：做 channel 级超时

因为这属于“给上游请求加边界”，直接提升稳定性。

这一步完成后，就能更稳地控制 AWS Claude 这类慢链路。

### 第三步：做归档框架，但先上 local backend

先把：

- 采集
- manifest
- 队列
- worker
- 本地存储

跑通。

原因：

- 本地更容易调试
- 更容易验证文件命名、截断、压缩、日志回写

### 第四步：再接 Azure Blob backend

这样把复杂度拆开，排障也更容易。

---

## 10. 最后再用一句大白话总结

这 3 个点本质上是在补齐一条高并发代理系统最重要的基础链路：

- **超时**：保证上游请求有边界，不要拖死系统
- **Trace-Id**：保证每个请求出问题时能串起来查
- **归档**：保证出了争议或事故时，有完整证据可回溯

真正落地时最重要的原则只有 4 句：

1. 主链路优先，归档不要反过来绑架主链路
2. 客户传入的 Trace-Id 只能当“可用但不可信”的外部标识
3. channel 配了超时，就必须真正生效
4. 数据库存元数据，大对象存本地或 Blob

如果后面继续进入编码阶段，这份文档就可以直接作为实施蓝图。
