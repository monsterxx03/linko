# Anthropic to OpenAI 协议转换代理设计

## 1. 概述

### 1.1 功能目标

实现一个协议转换代理（Protocol Transformer），将 Anthropic API 格式的请求转换为 OpenAI 兼容格式，转发给上游 OpenAI 兼容服务，并将响应转换回 Anthropic 格式返回给客户端。

### 1.2 使用场景

- 访问不支持 Anthropic API 的上游服务（如 Ollama、LocalAI、第三方兼容服务）
- 在不支持 Anthropic 的基础设施上使用 Claude SDK

### 1.3 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        Linko                                    │
│                                                                  │
│  ┌──────────────┐    ┌─────────────────────────────────────┐   │
│  │ 客户端        │    │  Anthropic 格式                     │   │
│  │ (Anthropic   │───▶│ /v1/messages                        │   │
│  │  SDK)        │    │ {model, messages, system, tools}   │   │
│  └──────────────┘    └──────────────────┬──────────────────┘   │
│                                         │                      │
│                                         ▼                      │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │              AnthropicToOpenAI Transformer              │   │
│  │  • 请求转换: Anthropic → OpenAI 格式                   │   │
│  │  • 响应转换: OpenAI → Anthropic 格式                   │   │
│  │  • SSE 流转换: OpenAI SSE → Anthropic SSE             │   │
│  │  • Thinking 提取/注入                                   │   │
│  │  • Tool 调用格式转换                                    │   │
│  └──────────────────────────┬──────────────────────────────┘   │
│                             │                                   │
│                             ▼                                   │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │              上游 (OpenAI 兼容服务)                        │   │
│  │  POST http://upstream/v1/chat/completions               │   │
│  │  (Ollama / LocalAI / 第三方)                             │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## 2. 配置设计

### 2.1 配置结构

```go
// pkg/config/config.go

type AnthropicProxyConfig struct {
    Enabled      bool              `yaml:"enabled"`
    UpstreamURL  string            `yaml:"upstream_url"`   // 上游地址，如 "http://localhost:11434"
    APIKey       string            `yaml:"api_key"`        // 可选，上游 API Key
    ModelMapping map[string]string `yaml:"model_mapping"`  // 模型名称映射
    Timeout      time.Duration     `yaml:"timeout"`        // 请求超时，默认 120s
}
```

### 2.2 配置示例

```yaml
# config/proxy.yaml
anthropic_proxy:
  enabled: true
  upstream_url: "http://localhost:11434"
  api_key: ""
  timeout: 120s
  model_mapping:
    claude-3-sonnet-20240229: llama3
    claude-3-opus-20240229: llama3:70b
    claude-3-5-sonnet-20241022: qwen2.5
```

## 3. 核心组件设计

### 3.1 模块结构

```
pkg/mitm/llm/
├── transformer.go        # 核心转换逻辑
├── transformer_test.go  # 测试
├── converter/
│   ├── request.go       # 请求转换
│   ├── response.go      # 响应转换
│   ├── stream.go        # 流式转换
│   ├── tools.go         # 工具调用转换
│   └── thinking.go      # Thinking 转换
└── types.go             # 已有类型定义
```

### 3.2 核心接口

```go
// Transformer 接口
type Transformer interface {
    // TransformRequest 将 Anthropic 请求转换为 OpenAI 格式
    TransformRequest(req *AnthropicRequest) (*OpenAIRequest, error)

    // TransformResponse 将 OpenAI 响应转换为 Anthropic 格式
    TransformResponse(resp *OpenAIResponse) (*AnthropicResponse, error)

    // TransformStreamChunk 将 OpenAI SSE chunk 转换为 Anthropic SSE
    TransformStreamChunk(chunk *OpenAIStreamChunk) ([]AnthropicStreamEvent, error)
}

// Proxy 代理接口
type Proxy interface {
    // RoundTrip 执行完整的请求-响应转换
    RoundTrip(ctx context.Context, req *AnthropicRequest) (*AnthropicResponse, error)

    // RoundTripStream 流式版本
    RoundTripStream(ctx context.Context, req *AnthropicRequest) (<-chan *AnthropicStreamEvent, error)
}
```

## 4. 请求转换详解

### 4.1 字段映射

| Anthropic 字段 | OpenAI 字段 | 转换说明 |
|---------------|-------------|----------|
| `model` | `model` | 查表映射，无则透传 |
| `max_tokens` | `max_tokens` | 直接映射 |
| `messages` | `messages` | 需转换格式 |
| `system` | `messages[0].content` (role=system) | 转换为首条 system 消息 |
| `tools` | `tools` | 需转换嵌套结构 |
| `temperature` | `temperature` | 直接映射 |
| `top_p` | `top_p` | 直接映射 |
| `top_k` | - | OpenAI 不支持，忽略 |
| `stop_sequences` | `stop` | 转换字段名 |
| `metadata` | - | OpenAI 不支持，忽略 |

### 4.2 消息转换

#### 4.2.1 用户消息

```json
// Anthropic
{"role": "user", "content": "Hello"}

// OpenAI
{"role": "user", "content": "Hello"}
```

#### 4.2.2 助手消息（含 Thinking）

```json
// Anthropic 输入
{
  "role": "assistant",
  "content": [
    {"type": "thinking", "thinking": "让我思考...", "id": "toolu_1"},
    {"type": "text", "text": "我的回答"}
  ]
}

// OpenAI 输出
{
  "role": "assistant",
  "content": "[Thinking]\n让我思考...\n[/Thinking]\n我的回答"
}
```

#### 4.2.3 助手消息（含 Tool Calls）

```json
// Anthropic 输入
{
  "role": "assistant",
  "content": [
    {"type": "tool_use", "id": "toolu_1", "name": "get_weather", "input": {"city": "Beijing"}}
  ]
}

// OpenAI 输出
{
  "role": "assistant",
  "content": null,
  "tool_calls": [
    {"id": "toolu_1", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"Beijing\"}"}}
  ]
}
```

#### 4.2.4 Tool Result 消息

```json
// Anthropic 输入
{
  "role": "user",
  "content": [
    {"type": "tool_result", "tool_use_id": "toolu_1", "content": "天气晴朗"}
  ]
}

// OpenAI 输出
{
  "role": "user",
  "content": [
    {"type": "tool_result", "tool_call_id": "toolu_1", "content": "天气晴朗"}
  ]
}
```

### 4.3 System 消息转换

```json
// Anthropic
{"system": "You are a helpful assistant."}
// 或
{"system": [{"type": "text", "text": "You are..."}]}

// OpenAI - 转换为 messages 数组
{"messages": [
  {"role": "system", "content": "You are a helpful assistant."}
]}
```

### 4.4 Tools 转换

```json
// Anthropic
{
  "tools": [
    {
      "name": "get_weather",
      "description": "Get weather information",
      "input_schema": {
        "type": "object",
        "properties": {"city": {"type": "string"}},
        "required": ["city"]
      }
    }
  ]
}

// OpenAI
{
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get weather information",
        "parameters": {
          "type": "object",
          "properties": {"city": {"type": "string"}},
          "required": ["city"]
        }
      }
    }
  ]
}
```

## 5. 响应转换详解

### 5.1 非流式响应

```json
// OpenAI 响应
{
  "id": "chatcmpl-xxx",
  "model": "llama3",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "回答内容"
    },
    "finish_reason": "stop"
  }],
  "usage": {"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}
}

// Anthropic 响应
{
  "id": "msg_xxx",
  "type": "message",
  "role": "assistant",
  "content": [{"type": "text", "text": "回答内容"}],
  "model": "claude-3-sonnet-20240229",
  "stop_reason": "end_turn",
  "usage": {"input_tokens": 10, "output_tokens": 20}
}
```

### 5.2 含 Tool Calls 的响应

```json
// OpenAI 响应
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": null,
      "tool_calls": [
        {"id": "call_xxx", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"Beijing\"}"}}
      ]
    },
    "finish_reason": "tool_calls"
  }]
}

// Anthropic 响应
{
  "content": [
    {"type": "tool_use", "id": "call_xxx", "name": "get_weather", "input": {"city": "Beijing"}}
  ],
  "stop_reason": "tool_use"
}
```

### 5.3 含 Reasoning (o1 模型)

```json
// OpenAI 响应
{
  "choices": [{
    "message": {
      "content": "最终回答",
      "reasoning_content": "推理过程..."
    }
  }]
}

// Anthropic 响应
{
  "content": [
    {"type": "thinking", "thinking": "推理过程..."},
    {"type": "text", "text": "最终回答"}
  ]
}
```

### 5.4 Stop Reason 映射

| OpenAI | Anthropic | 说明 |
|--------|-----------|------|
| `stop` | `end_turn` | 正常结束 |
| `length` | `max_tokens` | 达到最大 token |
| `tool_calls` | `tool_use` | 触发工具调用 |
| `content_filter` | `stopping_reason` | 内容过滤 |

## 6. 流式响应 (SSE) 转换

### 6.1 OpenAI SSE 格式

```
event: message_start
data: {"type":"message_start","message":{"id":"xxx","role":"assistant","content":[]}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" World"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":10}}

event: message_stop
data: {"type":"message_stop"}
```

### 6.2 OpenAI SSE 格式

```
data: {"id":"chatcmpl-xxx","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}
data: {"id":"chatcmpl-xxx","choices":[{"index":0,"delta":{"content":" World"},"finish_reason":null}]}
data: {"id":"chatcmpl-xxx","choices":[{"index":0,"delta":{},"finish_reason":"stop","usage":{"prompt_tokens":10,"completion_tokens":10}}]}
data: [DONE]
```

### 6.3 流式转换逻辑

1. **累积 Buffer**: OpenAI 流可能分多个 chunk 到达，需要缓存累积
2. **事件检测**: 解析 `choices[].delta` 判断当前状态
3. **状态机**:
   ```
   Idle → message_start → content_block_start → content_block_delta* → content_block_stop → message_delta → message_stop
   ```
4. **Thinking 处理**: 如果检测到 `reasoning_content`，生成 `thinking_delta` 事件

### 6.4 流式 Tool Calls

```json
// OpenAI 流式 tool_call
{"choices":[{"delta":{"tool_calls":[{"id":"call_xxx","function":{"name":"get_weather"}}]}}]}
{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"{"}}]}}]}
{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"\"city\":"}}]}}]}
{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"\"Beijing\"}"}]}}]}
{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}

// 转换为 Anthropic
event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_xxx","name":"get_weather"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}

... (累积 arguments)

event: content_block_stop
data: {"type":"content_block_stop","index":0}
```

## 7. Thinking 多轮对话处理

### 7.1 场景说明

在 Anthropic 多轮对话中，客户端发送下一轮请求时，需要携带上一轮的完整响应（包括 thinking）：

```json
// 客户端的第二轮请求
{
  "messages": [
    {"role": "user", "content": "第二句话"},
    {
      "role": "assistant",
      "content": [
        {"type": "thinking", "thinking": "上一轮的思考内容", "id": "toolu_1"},
        {"type": "text", "text": "上一轮的回答"}
      ]
    }
  ]
}
```

### 7.2 转换策略

#### 7.2.1 提取 Thinking 到文本

```go
func extractThinkingFromContent(content []AnthropicContent) []AnthropicContent {
    for i, c := range content {
        if c.Thinking != "" {
            // 合并到 text 中
            content[i].Text = "[Thinking]\n" + c.Thinking + "\n[/Thinking]\n" + c.Text
            content[i].Thinking = ""  // 清空 thinking
        }
    }
    return content
}
```

#### 7.2.2 完整转换流程

```
Anthropic 请求
    ↓
遍历 messages
    ↓
对每个 assistant 消息:
    ├── 提取 thinking → 合并到 text
    ├── 转换 tool_use → tool_calls
    └── 转换 tool_result 字段名
    ↓
转换为 OpenAI 格式
    ↓
发送到上游
```

### 7.3 流式 Thinking 累积

在流式响应中，thinking 可能分多个 chunk 到达：

```
// OpenAI 流
{"choices":[{"delta":{"reasoning_content":"思考中..."}}]}
{"choices":[{"delta":{"reasoning_content":"继续思考..."}}]}
{"choices":[{"delta":{"content":"最终回答","reasoning_content":"完整思考"}}]}
```

需要累积 reasoning_content，然后在最后一个 chunk 时转换为 Anthropic thinking：

```go
func (t *Transformer) handleStreamThinking(chunk *OpenAIStreamChunk) []AnthropicStreamEvent {
    var events []AnthropicStreamEvent

    for _, choice := range chunk.Choices {
        if choice.Delta.ReasoningContent != "" {
            // 累积 thinking
            t.thinkingBuffer += choice.Delta.ReasoningContent

            // 发送 thinking delta
            events = append(events, AnthropicStreamEvent{
                Type: "content_block_delta",
                Delta: struct {
                    Type    string `json:"type"`
                    Thinking string `json:"thinking,omitempty"`
                }{
                    Type:    "thinking_delta",
                    Thinking: choice.Delta.ReasoningContent,
                },
            })
        }
    }

    return events
}
```

## 8. 错误处理

### 8.1 上游错误透传

当上游返回错误时，直接转换为 Anthropic 错误格式：

```json
// OpenAI 错误
{"error":{"type":"invalid_request_error","message":"Model not found"}}

// 转换为 Anthropic
{"type":"error","error":{"type":"invalid_request_error","message":"Model not found"}}
```

### 8.2 不支持特性

| 特性 | 处理方式 |
|------|----------|
| `top_k` | 忽略，透传其他参数 |
| Thinking 配置 | 忽略（由上游模型决定） |
| 多模态图片 | 返回错误（当前版本不支持） |
| 不支持的 tool | 透传，由上游决定是否报错 |

## 9. 集成设计

### 9.1 在 MITM 中的集成

```go
// pkg/mitm/llm_inspector.go

func (l *LLMInspector) inspectRequest(data []byte, requestID string) ([]byte, error) {
    // ... 现有逻辑 ...

    // 检测是否是代理请求
    if l.proxyConfig != nil && provider == anthropicProvider {
        // 执行转换代理
        transformedReq, err := l.transformer.TransformRequest(req)
        if err != nil {
            return nil, err
        }

        // 发送到上游
        resp, err := l.proxy.RoundTrip(ctx, transformedReq)
        if err != nil {
            return nil, err
        }

        // 转换响应
        anthropicResp, err := l.transformer.TransformResponse(resp)
        // ...
    }

    // 原有分析逻辑...
}
```

### 9.2 配置加载

```go
// 在配置加载时读取 anthropic_proxy 配置
func LoadConfig(path string) (*Config, error) {
    // ...
    if err := yaml.Unmarshal(data, &cfg.AnthropicProxy); err != nil {
        return nil, err
    }
    // ...
}
```

## 10. 测试计划

### 10.1 单元测试

- [ ] 请求转换：消息格式
- [ ] 请求转换：System 提取
- [ ] 请求转换：Tools 格式
- [ ] 请求转换：Thinking 提取
- [ ] 响应转换：基本文本
- [ ] 响应转换：Tool Calls
- [ ] 响应转换：Thinking/Reasoning
- [ ] Stop Reason 映射

### 10.2 流式测试

- [ ] 基本文本流
- [ ] Tool Call 流
- [ ] Thinking 流 (o1 模型)
- [ ] 混合流 (text + tool)

### 10.3 集成测试

- [ ] 连接 Ollama
- [ ] 多轮对话
- [ ] 错误处理

## 11. 风险与局限

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Tool 格式不兼容 | 上游可能不支持 | 透传上游错误消息 |
| Thinking 语义丢失 | 推理过程不可见 | 合并到 text 显示 |
| 模型能力差异 | 效果可能不同 | 用户需理解上游模型能力 |
| 流式状态同步 | 可能出现状态不一致 | 完善状态机逻辑 |

## 12. 实现优先级

1. **P0 (必须)**: 基本请求/响应转换，无流式
2. **P1 (必须)**: 流式文本响应
3. **P1 (必须)**: Tool 调用转换
4. **P2 (重要)**: Thinking / Reasoning 转换
5. **P2 (重要)**: 多轮对话支持
6. **P3 (优化)**: 错误消息优化
