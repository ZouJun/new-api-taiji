# Phase 1-4 环境变量配置建议与取值顺序

本文整理最初需求中 Phase 1-4 涉及的配置项：客户 `Trace-Id`、超时控制、重试、请求/响应归档，以及 Azure Blob 归档。重点说明推荐配值、实际取值顺序和中文注释。

## Phase 1：Trace-Id 贯穿链路

### 配置结论

`Trace-Id` 当前没有环境变量开关，功能由路由中间件直接启用。

客户端请求头：

```http
Trace-Id: Abc123
```

中文注释：

```text
# Trace-Id 是客户侧传入的外部关联 ID，用于跨系统排查和对账。
# 只接受 Trace-Id 请求头，不接受 X-Trace-Id。
# 可以不传；不传时 trace_id 为空字符串。
# 传了但为空、重复、多值、包含空格、包含非字母数字字符、长度超过 64，都会被拒绝。
# Trace-Id 与系统生成的 request_id 是两个不同字段。
```

### 取值顺序

```text
1. 读取请求头 Trace-Id。
2. 如果缺失，trace_id=""，请求继续。
3. 如果存在，按规则校验。
4. 校验通过后写入 Gin context、request context、运行日志、消费日志/错误日志 other.trace_id、归档 manifest。
```

## Phase 2：超时控制与重试

### 推荐环境变量

生产独占部署建议：

```env
# 旧全局兜底超时。建议保持 0，避免误伤所有 provider；优先使用更细分的新变量。
RELAY_TIMEOUT=0

# 普通非流式请求默认总超时，单位秒。channel 未单独配置时生效。
RELAY_DEFAULT_NON_STREAM_TIMEOUT=120

# 流式请求默认首包超时，单位秒。只限制首包/首事件等待时间，首包到达后不再限制整条流总时长。
RELAY_DEFAULT_STREAM_FIRST_BYTE_TIMEOUT=120

# AWS Bedrock 非流式 InvokeModel 默认超时，单位秒。只作用于 AWS 非流式 SDK Invoke。
AWS_INVOKE_TIMEOUT_SECONDS=120

# AWS SDK 最大尝试次数。0 表示不覆盖 AWS SDK 默认策略。
AWS_SDK_MAX_ATTEMPTS=0

# HTTP 连接池空闲连接保留时间，单位秒。
RELAY_IDLE_CONN_TIMEOUT=90

# HTTP 连接池全局最大空闲连接数。
RELAY_MAX_IDLE_CONNS=500

# 每个上游 host 的最大空闲连接数。
RELAY_MAX_IDLE_CONNS_PER_HOST=100

# 流式扫描器空闲超时，单位秒。用于流已经建立后的流扫描等待，不等同于首包超时。
STREAMING_TIMEOUT=300
```

本地测试建议：

```env
# 本地测试建议保留全局兜底为 0，避免影响不相关用例。
RELAY_TIMEOUT=0

# 本地按渠道 setting 单独测超时；全局默认可先保持 0。
RELAY_DEFAULT_NON_STREAM_TIMEOUT=0
RELAY_DEFAULT_STREAM_FIRST_BYTE_TIMEOUT=0

# AWS 本地联调时可按需设置；非 AWS 测试保持 0。
AWS_INVOKE_TIMEOUT_SECONDS=0
AWS_SDK_MAX_ATTEMPTS=0
```

### channel.setting 推荐配置

单渠道精确控制时，优先写入 `channel.setting`：

```json
{
  "non_stream_timeout_seconds": 120,
  "stream_first_byte_timeout_seconds": 120,
  "aws_invoke_timeout_seconds": 120,
  "aws_sdk_max_attempts": 0
}
```

中文注释：

```text
# non_stream_timeout_seconds：普通非流式总超时，单位秒。
# stream_first_byte_timeout_seconds：流式首包超时，单位秒；不是整条流总超时。
# aws_invoke_timeout_seconds：AWS 非流式 InvokeModel 超时，单位秒。
# aws_sdk_max_attempts：AWS SDK 最大尝试次数；0 或不配置表示不覆盖 SDK 默认策略。
# channel.setting 中这些超时值必须大于 0；0 或负数会被校验拒绝。
```

### 超时取值顺序

普通非流式总超时：

```text
1. channel.setting.non_stream_timeout_seconds
2. RELAY_DEFAULT_NON_STREAM_TIMEOUT
3. RELAY_TIMEOUT
4. 都没有或都为 0，则不启用这层显式超时
```

流式首包超时：

```text
1. channel.setting.stream_first_byte_timeout_seconds
2. RELAY_DEFAULT_STREAM_FIRST_BYTE_TIMEOUT
3. RELAY_TIMEOUT
4. 都没有或都为 0，则不启用首包等待超时
```

AWS 非流式 Invoke 超时：

```text
1. channel.setting.aws_invoke_timeout_seconds
2. AWS_INVOKE_TIMEOUT_SECONDS
3. RELAY_DEFAULT_NON_STREAM_TIMEOUT
4. RELAY_TIMEOUT
5. 都没有或都为 0，则不启用这层显式超时
```

AWS SDK 最大尝试次数：

```text
1. channel.setting.aws_sdk_max_attempts
2. AWS_SDK_MAX_ATTEMPTS
3. 都没有或为 0，则不覆盖 AWS SDK 默认策略
```

### RetryTimes 说明

`RetryTimes` 当前不是环境变量，而是系统 option 配置。

推荐值：

```text
RetryTimes=1
```

中文注释：

```text
# RetryTimes=1 表示首次渠道失败后，最多再重试 1 次。
# 对于优先级渠道场景：先请求高优先级渠道，若首包超时/可重试错误，再请求下一个可用渠道。
# 流式首包超时必须保持原始 timeout 错误可识别，否则 shouldRetry 无法判断并重试。
```

取值顺序：

```text
1. 代码默认 common.RetryTimes=0。
2. InitOptionMap 初始化默认 OptionMap。
3. 数据库 options 表中的 RetryTimes 覆盖默认值。
4. 后台保存配置或定时同步后更新内存值。
```

## Phase 3：本地请求/响应归档

### 推荐环境变量

本地归档建议：

```env
# 是否启用请求/响应归档。默认 false；本地验证归档链路时设为 true。
ARCHIVE_ENABLED=true

# 归档后端。local 表示写本地文件。
ARCHIVE_BACKEND=local

# 本地归档根目录。建议生产使用独立挂载目录，本地可用 ./data/archive。
ARCHIVE_LOCAL_DIR=./data/archive

# 临时 spool 目录。为空时使用 ARCHIVE_LOCAL_DIR/spool。
ARCHIVE_SPOOL_DIR=

# 异步归档队列大小。队列满时跳过归档，不阻塞主请求。
ARCHIVE_QUEUE_SIZE=50000

# 归档 worker 数量。高 RPM 场景可按 CPU/磁盘吞吐调大。
ARCHIVE_WORKER_COUNT=32

# 单个请求体归档大小上限，单位 MB。超过上限跳过 request.data.gz。
ARCHIVE_MAX_REQUEST_MB=128

# 单个响应体归档大小上限，单位 MB。超过上限跳过 response.data.gz。
ARCHIVE_MAX_RESPONSE_MB=128

# 失败 spool 文件保留时间，单位小时。
ARCHIVE_SPOOL_TTL_HOURS=24
```

生产 local 后端建议：

```env
ARCHIVE_ENABLED=true
ARCHIVE_BACKEND=local
ARCHIVE_LOCAL_DIR=/data/new-api/archive
ARCHIVE_SPOOL_DIR=/data/new-api/archive/spool
ARCHIVE_QUEUE_SIZE=50000
ARCHIVE_WORKER_COUNT=32
ARCHIVE_MAX_REQUEST_MB=128
ARCHIVE_MAX_RESPONSE_MB=128
ARCHIVE_SPOOL_TTL_HOURS=24
```

中文注释：

```text
# 归档是异步链路，默认不影响客户请求成功。
# 队列满、对象过大、spool 写失败、后端写失败都会记录到 logs.other.archive 或运行日志。
# 归档对象以 request_id 为路径 key，trace_id 是 manifest/metadata 字段，不作为路径主 key。
```

### 取值顺序

```text
1. 进程启动时从环境变量读取 ARCHIVE_*。
2. 未设置时使用代码默认值。
3. 归档 Manager 初始化时把 common 中的配置转换为归档运行配置。
4. 当前 ARCHIVE_* 不走 options 表，修改环境变量后需要重启服务生效。
```

## Phase 4：Azure Blob 归档与生产化

### Azure Shared Key 推荐环境变量

```env
# 启用归档。
ARCHIVE_ENABLED=true

# 使用 Azure Blob 后端。
ARCHIVE_BACKEND=azure_blob

# Azure Blob 服务地址。
ARCHIVE_AZURE_ACCOUNT_URL=https://<account>.blob.core.windows.net/

# Azure Blob 容器名称。
ARCHIVE_AZURE_CONTAINER=<container>

# Azure Storage Account 名称。
ARCHIVE_AZURE_ACCOUNT_NAME=<account>

# Azure Storage Account Key。生产环境必须通过密钥管理注入，不要提交到代码仓库。
ARCHIVE_AZURE_ACCOUNT_KEY=<key>

# Azure 后端仍会先写本地 spool，再由 worker 上传。
ARCHIVE_SPOOL_DIR=/data/new-api/archive/spool

# 高 RPM 场景按机器资源调整。
ARCHIVE_QUEUE_SIZE=50000
ARCHIVE_WORKER_COUNT=32
ARCHIVE_MAX_REQUEST_MB=128
ARCHIVE_MAX_RESPONSE_MB=128
ARCHIVE_SPOOL_TTL_HOURS=24
```

### Azure SAS URL 推荐环境变量

```env
# 启用归档。
ARCHIVE_ENABLED=true

# 使用 Azure Blob 后端。
ARCHIVE_BACKEND=azure_blob

# 带 SAS 的 Azure Blob URL。使用 SAS 时可以不配置 ACCOUNT_NAME 和 ACCOUNT_KEY。
ARCHIVE_AZURE_ACCOUNT_URL=https://<account>.blob.core.windows.net/?<sas>

# Azure Blob 容器名称。
ARCHIVE_AZURE_CONTAINER=<container>

# 使用 SAS URL 时留空。
ARCHIVE_AZURE_ACCOUNT_NAME=
ARCHIVE_AZURE_ACCOUNT_KEY=

# spool 目录建议放在可靠磁盘。
ARCHIVE_SPOOL_DIR=/data/new-api/archive/spool
```

中文注释：

```text
# ARCHIVE_BACKEND=azure_blob 时，必须提供 Azure Blob 连接信息。
# 支持两种认证方式：Shared Key，或 ACCOUNT_URL 中携带 SAS。
# 归档失败不会让客户成功响应失败，但会记录 archive failed/skipped 状态。
# 生产必须监控 spool 目录大小、队列积压、worker 错误和 Azure 写入失败。
```

### Azure 取值顺序

```text
1. ARCHIVE_ENABLED 必须为 true，否则归档整体不启用。
2. ARCHIVE_BACKEND=azure_blob 时初始化 Azure 后端。
3. 优先使用 ARCHIVE_AZURE_ACCOUNT_URL。
4. 如果 URL 携带 SAS，可不配置 account name/key。
5. 如果 URL 不携带 SAS，则使用 ARCHIVE_AZURE_ACCOUNT_NAME + ARCHIVE_AZURE_ACCOUNT_KEY。
6. ARCHIVE_AZURE_CONTAINER 必须配置。
7. 任一关键配置缺失时，Azure 后端会失败，归档状态记录为 failed，但主请求不应被归档失败阻断。
```

## 总体推荐配置模板

### 本地测试模板

```env
# Phase 2：本地优先用 channel.setting 精确控制，环境变量保持不干扰。
RELAY_TIMEOUT=0
RELAY_DEFAULT_NON_STREAM_TIMEOUT=0
RELAY_DEFAULT_STREAM_FIRST_BYTE_TIMEOUT=0
AWS_INVOKE_TIMEOUT_SECONDS=0
AWS_SDK_MAX_ATTEMPTS=0

# Phase 3：需要验证归档时打开 local。
ARCHIVE_ENABLED=true
ARCHIVE_BACKEND=local
ARCHIVE_LOCAL_DIR=./data/archive
ARCHIVE_SPOOL_DIR=
ARCHIVE_QUEUE_SIZE=50000
ARCHIVE_WORKER_COUNT=8
ARCHIVE_MAX_REQUEST_MB=128
ARCHIVE_MAX_RESPONSE_MB=128
ARCHIVE_SPOOL_TTL_HOURS=24
```

### 生产模板：独占部署 + local 归档

```env
RELAY_TIMEOUT=0
RELAY_DEFAULT_NON_STREAM_TIMEOUT=120
RELAY_DEFAULT_STREAM_FIRST_BYTE_TIMEOUT=120
AWS_INVOKE_TIMEOUT_SECONDS=120
AWS_SDK_MAX_ATTEMPTS=0
RELAY_IDLE_CONN_TIMEOUT=90
RELAY_MAX_IDLE_CONNS=500
RELAY_MAX_IDLE_CONNS_PER_HOST=100
STREAMING_TIMEOUT=300

ARCHIVE_ENABLED=true
ARCHIVE_BACKEND=local
ARCHIVE_LOCAL_DIR=/data/new-api/archive
ARCHIVE_SPOOL_DIR=/data/new-api/archive/spool
ARCHIVE_QUEUE_SIZE=50000
ARCHIVE_WORKER_COUNT=32
ARCHIVE_MAX_REQUEST_MB=128
ARCHIVE_MAX_RESPONSE_MB=128
ARCHIVE_SPOOL_TTL_HOURS=24
```

### 生产模板：Azure Blob 归档

```env
RELAY_TIMEOUT=0
RELAY_DEFAULT_NON_STREAM_TIMEOUT=120
RELAY_DEFAULT_STREAM_FIRST_BYTE_TIMEOUT=120
AWS_INVOKE_TIMEOUT_SECONDS=120
AWS_SDK_MAX_ATTEMPTS=0
RELAY_IDLE_CONN_TIMEOUT=90
RELAY_MAX_IDLE_CONNS=500
RELAY_MAX_IDLE_CONNS_PER_HOST=100
STREAMING_TIMEOUT=300

ARCHIVE_ENABLED=true
ARCHIVE_BACKEND=azure_blob
ARCHIVE_AZURE_ACCOUNT_URL=https://<account>.blob.core.windows.net/
ARCHIVE_AZURE_CONTAINER=<container>
ARCHIVE_AZURE_ACCOUNT_NAME=<account>
ARCHIVE_AZURE_ACCOUNT_KEY=<key>
ARCHIVE_SPOOL_DIR=/data/new-api/archive/spool
ARCHIVE_QUEUE_SIZE=50000
ARCHIVE_WORKER_COUNT=32
ARCHIVE_MAX_REQUEST_MB=128
ARCHIVE_MAX_RESPONSE_MB=128
ARCHIVE_SPOOL_TTL_HOURS=24
```

## 快速核对清单

```text
# Trace-Id
客户端只传 Trace-Id，值为 1-64 位字母数字。

# 超时
优先使用 channel.setting 配单渠道超时；全局环境变量只作为兜底。

# 重试
RetryTimes=1 写入系统 option，不是环境变量。

# 归档
ARCHIVE_ENABLED=true 后才会启用归档中间件和 worker。

# Azure
ARCHIVE_BACKEND=azure_blob 时必须配置 container 和认证信息。
```
