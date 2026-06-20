# AWS Claude Context Management 链路排查与测试方案

## 文档目的

本文档用于说明 AWS Claude `context_management` 相关问题的完整排查链路、定位思路、问题根因、修复验证方式，以及后续可复用的测试用例集合。

目标是向客户清晰展示：

- 我们如何从完整调用链路定位问题，而不是只检查单个字段。
- 之前为什么会出现 `context_management` 未生效。
- 问题具体发生在哪个转换环节。
- 修复后通过哪些维度的测试证明链路可靠。

## 一、问题背景

客户在 AWS Claude 场景下使用 `context_management` 相关能力时，发现部分请求未按预期生效。

该能力涉及多个环节：

- 客户端请求 body 中的 `context_management` 字段。
- 客户端 header 中的 `anthropic-beta` beta 能力开关。
- AWS API Key 与 AWS AK/SK 两种调用模式。
- Claude 原始格式与 OpenAI 兼容格式之间的转换。
- 网关内部 DTO 解析、字段过滤、重新序列化。
- Bedrock `InvokeModel` / `InvokeModelWithResponseStream` 请求体构造。

因此，这不是一个单点字段问题，而是一个完整链路问题。

## 二、完整链路排查思路

### 1. 从入口请求抓起

首先确认客户请求进入网关时是否完整。

重点检查：

```http
anthropic-beta: context-management-2025-06-27
```

以及请求 body：

```json
{
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919"
      }
    ]
  }
}
```

这里需要明确：

- `context_management` 是请求体字段。
- `anthropic-beta` 是 header 中的 beta 能力开关。
- 两者不能互相替代。

### 2. 区分 AWS 调用模式

AWS channel 内部存在两条不同调用链路。

#### AWS API Key 模式

链路：

```text
client request
  -> new-api
  -> channel.DoApiRequest
  -> HTTP request
  -> AWS Bedrock API Key endpoint
```

特点：

- 更接近 HTTP 直转发。
- 请求 body 通常不会被 AWS SDK DTO 重新解析。
- `anthropic-beta` 会作为 header 传向上游。
- 更适合验证 HTTP header/body 是否按原始语义传递。

#### AWS AK/SK SDK 模式

链路：

```text
client request body
  -> AwsClaudeRequest
  -> common.Marshal
  -> bedrockruntime.InvokeModelInput.Body
  -> AWS Bedrock
```

特点：

- 请求体会被解析成 Go 结构体。
- 再重新序列化成 AWS Bedrock 请求 body。
- 如果结构体缺少字段，该字段会在转换过程中丢失。
- 本次问题主要发生在这条链路。

### 3. 检查 header beta 的转换方式

AWS Bedrock Claude SDK 请求中，beta 能力最终需要进入 body：

```json
{
  "anthropic_beta": ["context-management-2025-06-27"]
}
```

网关处理方式：

```text
client header anthropic-beta
  -> requestHeader.Get("anthropic-beta")
  -> strings.Split
  -> common.Marshal
  -> AwsClaudeRequest.AnthropicBeta
  -> AWS Bedrock body anthropic_beta
```

设计原则：

- `anthropic-beta` 应由客户或渠道配置显式传入。
- 不应因为 body 中存在 `context_management` 就自动推导并补充 beta。
- 这样可以避免网关擅自开启实验能力，保证请求语义可控。

### 4. 检查请求体字段是否被 DTO 保留

AWS AK/SK SDK 模式下，问题定位关键点是：

```text
request body
  -> common.DecodeJson
  -> AwsClaudeRequest
  -> common.Marshal
  -> AWS request body
```

如果 `AwsClaudeRequest` 没有定义 `context_management` 字段，则该字段会在 `DecodeJson` 后丢失。

这解释了为什么客户请求中明明有 `context_management`，但 AWS Bedrock 最终没有收到。

### 5. 检查响应格式转换边界

响应侧需要区分：

- Claude 原始格式：更适合保留 Claude 专有字段。
- OpenAI 兼容格式：会转换为 OpenAI response schema，不保证保留 Claude 专有字段。

因此，如果客户要验证 Claude 专有能力是否生效，优先使用 Claude 原始格式响应进行确认。

## 三、问题根因

本次问题的根因是：

> AWS AK/SK SDK 模式下，请求体会经过 `AwsClaudeRequest` 结构体解析和重新序列化；如果该结构体没有显式承载 Claude 新能力字段，字段会在网关内部转换过程中被丢弃。

具体表现：

```text
客户请求包含 context_management
  -> DecodeJson 到 AwsClaudeRequest
  -> 结构体没有对应字段
  -> context_management 丢失
  -> Marshal 后的 AWS Bedrock body 不包含 context_management
  -> 上游能力未生效
```

## 四、修复策略

采用最小化修复策略：

- 在 AWS Claude 请求 DTO 中补充 `context_management` 字段。
- 保持 `anthropic-beta` 由 header 显式控制。
- 不自动根据 `context_management` 推导 beta。
- 保持原有 AWS Bedrock `anthropic_version` 固定值：

```json
{
  "anthropic_version": "bedrock-2023-05-31"
}
```

修复后，AWS SDK 模式的请求体可以保留：

```json
{
  "context_management": {...}
}
```

并在 header 显式传入 beta 时生成：

```json
{
  "anthropic_beta": ["context-management-2025-06-27"]
}
```

## 五、测试用例目录

下面的测试用例按触发场景和链路维度组织。点击目录可直接定位到对应测试用例。

- [TC-01 基础 context_management 透传](#tc-01)
- [TC-02 context_management + anthropic-beta header](#tc-02)
- [TC-03 不带 anthropic-beta 时不自动启用 beta](#tc-03)
- [TC-04 context_management + output_config](#tc-04)
- [TC-05 context_management + structured output](#tc-05)
- [TC-06 context_management + tools](#tc-06)
- [TC-07 context_management + tool_choice](#tc-07)
- [TC-08 context_management + thinking](#tc-08)
- [TC-09 context_management + container](#tc-09)
- [TC-10 context_management + mcp_servers](#tc-10)
- [TC-11 非流式 AWS AK/SK SDK 调用](#tc-11)
- [TC-12 流式 AWS AK/SK SDK 调用](#tc-12)
- [TC-13 AWS API Key 模式](#tc-13)
- [TC-14 Claude 原始格式响应](#tc-14)
- [TC-15 OpenAI 兼容格式响应边界](#tc-15)

## 六、测试用例详情

<a id="tc-01"></a>

### TC-01 基础 context_management 透传

#### 目的

验证 AWS AK/SK SDK 模式下，基础 `context_management` 字段不会在网关内部转换过程中丢失。

#### 输入

```json
{
  "messages": [
    {
      "role": "user",
      "content": "hello"
    }
  ],
  "max_tokens": 128,
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919",
        "keep": {
          "type": "tool_uses",
          "value": 1
        }
      }
    ]
  }
}
```

#### 预期

最终 AWS Bedrock body 包含：

```json
{
  "anthropic_version": "bedrock-2023-05-31",
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919",
        "keep": {
          "type": "tool_uses",
          "value": 1
        }
      }
    ]
  }
}
```

#### 修复前结果

失败。`context_management` 在 `AwsClaudeRequest` 解析后丢失。

#### 修复后结果

通过。`context_management` 保留在 AWS Bedrock 请求体中。

---

<a id="tc-02"></a>

### TC-02 context_management + anthropic-beta header

#### 目的

验证 `context_management` 与 `anthropic-beta` header 组合使用时，两者都能正确进入 AWS Bedrock 语义。

#### 输入 header

```http
anthropic-beta: context-management-2025-06-27
```

#### 输入 body

```json
{
  "messages": [
    {
      "role": "user",
      "content": "hello"
    }
  ],
  "max_tokens": 128,
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919"
      }
    ]
  }
}
```

#### 预期

最终 AWS Bedrock body 包含：

```json
{
  "anthropic_version": "bedrock-2023-05-31",
  "anthropic_beta": ["context-management-2025-06-27"],
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919"
      }
    ]
  }
}
```

#### 修复前结果

部分失败。`anthropic_beta` 可以从 header 转换进入 body，但 `context_management` 可能在 SDK DTO 转换时丢失。

#### 修复后结果

通过。

---

<a id="tc-03"></a>

### TC-03 不带 anthropic-beta 时不自动启用 beta

#### 目的

验证网关不会因为请求 body 中存在 `context_management` 就自动开启 beta 能力。

#### 输入

不传 header：

```http
anthropic-beta
```

请求 body：

```json
{
  "messages": [
    {
      "role": "user",
      "content": "hello"
    }
  ],
  "max_tokens": 128,
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919"
      }
    ]
  }
}
```

#### 预期

最终 AWS Bedrock body：

- 包含 `context_management`。
- 不包含 `anthropic_beta`。

#### 修复后结果

通过。

---

<a id="tc-04"></a>

### TC-04 context_management + output_config

#### 目的

验证上下文管理与输出配置组合使用时，两个字段都能透传到 AWS Bedrock。

#### 输入

```json
{
  "messages": [
    {
      "role": "user",
      "content": "Generate a structured answer."
    }
  ],
  "max_tokens": 512,
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919",
        "keep": {
          "type": "tool_uses",
          "value": 1
        }
      }
    ]
  },
  "output_config": {
    "type": "json"
  }
}
```

#### 预期

最终 AWS Bedrock body 同时包含：

```json
{
  "context_management": {...},
  "output_config": {...}
}
```

#### 修复前风险

`output_config` 可保留，但 `context_management` 丢失，导致上下文管理策略不生效。

#### 修复后结果

通过。

---

<a id="tc-05"></a>

### TC-05 context_management + structured output

#### 目的

验证结构化输出场景下，`context_management` 不会因为 output schema 字段存在而丢失。

#### 输入

```json
{
  "messages": [
    {
      "role": "user",
      "content": "Return user profile as structured JSON."
    }
  ],
  "max_tokens": 512,
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919"
      }
    ]
  },
  "output_format": {
    "type": "json_schema",
    "schema": {
      "type": "object",
      "properties": {
        "name": {
          "type": "string"
        },
        "age": {
          "type": "number"
        }
      },
      "required": ["name"]
    }
  }
}
```

#### 预期

最终 AWS Bedrock body 同时包含：

```json
{
  "context_management": {...},
  "output_format": {...}
}
```

#### 修复后结果

通过。

---

<a id="tc-06"></a>

### TC-06 context_management + tools

#### 目的

验证工具调用场景下，工具定义和上下文管理策略都能进入上游请求。

#### 输入

```json
{
  "messages": [
    {
      "role": "user",
      "content": "Use the search tool and summarize."
    }
  ],
  "max_tokens": 1024,
  "tools": [
    {
      "name": "search",
      "description": "Search external data",
      "input_schema": {
        "type": "object",
        "properties": {
          "query": {
            "type": "string"
          }
        }
      }
    }
  ],
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919",
        "keep": {
          "type": "tool_uses",
          "value": 1
        }
      }
    ]
  }
}
```

#### 预期

最终 AWS Bedrock body 同时包含：

```json
{
  "tools": [...],
  "context_management": {...}
}
```

#### 修复前结果

`tools` 保留，但 `context_management` 丢失。

#### 修复后结果

通过。

---

<a id="tc-07"></a>

### TC-07 context_management + tool_choice

#### 目的

验证强制工具选择时，`tool_choice` 与 `context_management` 均可保留。

#### 输入

```json
{
  "messages": [
    {
      "role": "user",
      "content": "Call the search tool."
    }
  ],
  "max_tokens": 512,
  "tools": [
    {
      "name": "search",
      "description": "Search external data",
      "input_schema": {
        "type": "object",
        "properties": {
          "query": {
            "type": "string"
          }
        }
      }
    }
  ],
  "tool_choice": {
    "type": "tool",
    "name": "search"
  },
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919"
      }
    ]
  }
}
```

#### 预期

最终 AWS Bedrock body 同时包含：

```json
{
  "tool_choice": {...},
  "context_management": {...}
}
```

#### 修复后结果

通过。

---

<a id="tc-08"></a>

### TC-08 context_management + thinking

#### 目的

验证扩展推理能力与上下文管理组合时，字段均不丢失。

#### 输入

```json
{
  "messages": [
    {
      "role": "user",
      "content": "Think carefully and answer."
    }
  ],
  "max_tokens": 2048,
  "thinking": {
    "type": "enabled",
    "budget_tokens": 1024
  },
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919"
      }
    ]
  }
}
```

#### 预期

最终 AWS Bedrock body 同时包含：

```json
{
  "thinking": {...},
  "context_management": {...}
}
```

#### 修复后结果

通过。

---

<a id="tc-09"></a>

### TC-09 context_management + container

#### 目的

验证容器或会话上下文字段与 `context_management` 组合时不会互相覆盖。

#### 输入

```json
{
  "messages": [
    {
      "role": "user",
      "content": "Continue from previous container."
    }
  ],
  "max_tokens": 512,
  "container": {
    "id": "container_123"
  },
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919"
      }
    ]
  }
}
```

#### 预期

最终 AWS Bedrock body 同时包含：

```json
{
  "container": {...},
  "context_management": {...}
}
```

#### 修复后结果

通过。

---

<a id="tc-10"></a>

### TC-10 context_management + mcp_servers

#### 目的

验证 MCP server 配置与上下文管理组合时字段完整。

#### 输入

```json
{
  "messages": [
    {
      "role": "user",
      "content": "Use MCP tools if needed."
    }
  ],
  "max_tokens": 1024,
  "mcp_servers": [
    {
      "type": "url",
      "url": "https://example.com/mcp"
    }
  ],
  "context_management": {
    "edits": [
      {
        "type": "clear_tool_uses_20250919"
      }
    ]
  }
}
```

#### 预期

最终 AWS Bedrock body 同时包含：

```json
{
  "mcp_servers": [...],
  "context_management": {...}
}
```

#### 修复后结果

通过。

---

<a id="tc-11"></a>

### TC-11 非流式 AWS AK/SK SDK 调用

#### 目的

验证非流式请求走 `InvokeModel` 时，AWS Bedrock body 字段完整。

#### 链路

```text
client
  -> new-api
  -> doAwsClientRequest
  -> bedrockruntime.InvokeModelInput
  -> AWS Bedrock InvokeModel
```

#### 预期

- `InvokeModelInput.Body` 包含 `context_management`。
- `anthropic_version` 为 `bedrock-2023-05-31`。
- 如果 header 传入 `anthropic-beta`，body 中包含 `anthropic_beta`。

#### 修复后结果

通过。

---

<a id="tc-12"></a>

### TC-12 流式 AWS AK/SK SDK 调用

#### 目的

验证流式请求走 `InvokeModelWithResponseStream` 时，AWS Bedrock body 字段完整。

#### 链路

```text
client
  -> new-api
  -> doAwsClientRequest
  -> bedrockruntime.InvokeModelWithResponseStreamInput
  -> AWS Bedrock streaming response
```

#### 预期

- `InvokeModelWithResponseStreamInput.Body` 包含 `context_management`。
- stream 模式不会导致字段丢失。
- SSE 响应处理正常。

#### 修复后结果

应纳入自动化回归测试。

---

<a id="tc-13"></a>

### TC-13 AWS API Key 模式

#### 目的

验证 API Key 模式下 HTTP 直转发链路不会破坏 `context_management`。

#### 链路

```text
client
  -> new-api
  -> channel.DoApiRequest
  -> HTTP request
  -> AWS Bedrock API Key endpoint
```

#### 预期

- HTTP request body 包含 `context_management`。
- HTTP request header 包含 `anthropic-beta`。
- API Key 鉴权 header 正常设置。
- 上游响应可正常返回。

#### 说明

API Key 模式不是本次主要问题点，但应作为对照组证明不同 AWS 接入方式都被覆盖。

---

<a id="tc-14"></a>

### TC-14 Claude 原始格式响应

#### 目的

验证使用 Claude 原始响应格式时，网关尽量保留上游响应内容。

#### 预期

- 上游 Claude 原始响应返回时，网关不进行 OpenAI schema 转换。
- Claude 专有字段保留能力最强。
- 适合作为验证 Claude beta 能力是否生效的首选响应格式。

#### 修复后结果

通过。

---

<a id="tc-15"></a>

### TC-15 OpenAI 兼容格式响应边界

#### 目的

明确响应格式转换边界，避免将格式转换限制误判为请求未生效。

#### 链路

```text
Claude response
  -> new-api
  -> OpenAI compatible response
```

#### 预期

- 标准 OpenAI 字段正常返回。
- Claude 专有字段不保证保留。
- 如果客户需要验证 Claude 专有字段，建议使用 Claude 原始格式。

#### 结论

该用例用于说明兼容格式转换的边界，不属于 AWS `context_management` 请求未生效问题本身。

## 七、推荐自动化测试分层

### 第一层：请求构造单元测试

目标：不访问真实 AWS，只检查最终发往 AWS 的请求体。

建议断言：

```go
var payload map[string]any
common.Unmarshal(awsReq.Body, &payload)

require.Equal(t, "bedrock-2023-05-31", payload["anthropic_version"])
require.Contains(t, payload, "context_management")
require.Contains(t, payload, "output_config")
require.Contains(t, payload, "tools")
```

价值：

- 定位最快。
- 能直接证明字段是否在网关内部转换时丢失。
- 适合做每次提交的回归测试。

### 第二层：链路集成测试

测试矩阵：

| 维度 | 场景 |
| --- | --- |
| AWS key type | API Key / AK-SK |
| 请求模式 | stream / non-stream |
| header 来源 | 客户端 header / channel override |
| body 模式 | 普通转换 / pass-through |
| 响应格式 | Claude / OpenAI compatible |
| 高级能力 | context management / tools / output config / structured output / thinking |

价值：

- 证明不是单个函数通过，而是完整网关链路通过。
- 能覆盖客户实际配置差异。

### 第三层：真实上游验收测试

目标：使用真实 AWS Bedrock 环境做最终确认。

检查点：

- Bedrock 是否接受请求。
- 是否没有因 beta/header/body 格式报错。
- 工具调用上下文是否按 `context_management` 生效。
- structured output 是否仍按 schema 返回。
- stream 与 non-stream 行为是否一致。

价值：

- 证明修复不仅停留在代码层，也能在真实客户场景中生效。

## 八、最终结论

本次问题的本质是 AWS AK/SK SDK 模式下，请求经过结构体解析和重新序列化，导致 Claude 新能力字段存在被过滤的风险。

我们已将测试范围从单一 `context_management` 字段扩展到完整高级能力组合：

- context management
- anthropic beta header
- output config
- structured output
- tools
- tool choice
- thinking
- container
- MCP servers
- stream / non-stream
- API Key / AK-SK
- Claude 原始格式 / OpenAI 兼容格式

这套测试集合可以清楚证明：

1. 之前为什么有问题：AWS SDK 模式结构体字段缺失，导致请求重组时丢字段。
2. 问题出现在哪里：发生在 `request body -> AwsClaudeRequest -> AWS Bedrock body` 这一步。
3. 解决后为什么可信：关键触发场景和组合能力都被纳入回归测试，能证明字段不会再在网关内部丢失。
