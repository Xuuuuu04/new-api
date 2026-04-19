# 技术方案：Responses(CodeX) → Anthropic 协议转换全量修复

**背景**：客户端以 OpenAI `/v1/responses` 格式请求经由 new-api 转发到 Anthropic 上游时，转换路径当前完全 broken（`ConvertOpenAIResponsesRequest` 直接返回 `not implemented`）。本方案覆盖全部 15 条审计 finding，并补充 2 条审计遗漏项，实现"零配置自动双向转换"。

**方案选型**：在现有 Claude Adaptor 内扩展，新建 `responses.go` 承载双向转换逻辑，避免 `relay-claude.go` 进一步膨胀（文件级最小改动优先）。不引入新的基础设施，不改动计费会计层，不跨模块重构。

---

## 1. In-scope（本次必做）

- [ ] `src/relay/channel/claude/adaptor.go`：实现 `ConvertOpenAIResponsesRequest`；`DoResponse` 增加 `RelayFormatOpenAIResponses` 分支（finding 1）
- [ ] `src/relay/channel/claude/relay-claude.go`：`HandleClaudeResponseData` switch 补 `RelayFormatOpenAIResponses` 分支（finding 2）；`HandleStreamResponseData` 补 `RelayFormatOpenAIResponses` 分支（finding 3）；修复 `fcIdx` 独立计数（finding 9）；修复 `signature_delta` 透传（finding 6）；修复 `tool_use` arguments 为空 continue 问题（finding 8）；修复 `message_start` 重复发 content 问题（finding 10）；`budget_tokens` 表重算（finding 5）；`HandleStreamFinalResponse` 补 `RelayFormatOpenAIResponses` 结尾逻辑
- [ ] 新建 `src/relay/channel/claude/responses.go`：放置 Responses 方向 helper：`RequestResponses2Claude`、`ClaudeResponse2Responses`（非流式）、`StreamClaude2Responses` + `StreamState` 结构体（finding 1/2/3）
- [ ] `src/dto/claude.go`：`ClaudeUsage` 补 `ThinkingOutputTokens int` 字段（finding 7）
- [ ] `src/dto/openai_response.go`：新增 `ResponsesUsage` 专用结构；`OpenAIResponsesResponse.Usage` 类型由 `*Usage` 改为 `*ResponsesUsage`；补 `OutputTokensDetails` 子结构含 `reasoning_tokens`（finding 7/12）；`ResponsesStreamResponse` 补 `SequenceNumber int` 字段（finding 13）
- [ ] `src/relay/reasonmap/reasonmap.go`：`ClaudeStopReasonToOpenAIFinishReason` 加 `pause_turn` → `"incomplete"` 映射（finding 11）
- [ ] `src/service/openaicompat/chat_to_responses.go`：`Summary` 由硬编码 `"detailed"` 改为按 `req.Include` 字段判断（finding 14）
- [ ] `src/relay/responses_handler.go`：`PreviousResponseID` 非空时返回 `HTTP 400` + 明确 message（finding 15）
- [ ] `src/relay/channel/claude/relay-claude.go` 图片 source 处理：当 URL 以 `http` 开头时，`ClaudeMessageSource.Type` 应设为 `"url"`，`Url` 字段直接填充，不走 base64 下载（finding 4）

## 2. Out-of-scope（本次明确不做）

- Chat Completions 已有转换器（`RequestOpenAI2ClaudeMessage` / `ResponseClaude2OpenAI`）的重构——现有路径工作正常，改动有回归风险
- 非 Anthropic 上游的 Responses 入口改动（OpenAI/Gemini/等 adaptor 的 `ConvertOpenAIResponsesRequest` 已各自实现）
- 鉴权/计费会计模型层改动（`postConsumeQuota` 流程不动）
- Anthropic Messages API 流式 `relay-claude.go` 的非 Responses 路径重构（只加分支，不动现有 if-else）
- `RelayModeResponsesCompact` 对 Claude 渠道的支持（当前 `responses_handler.go` 在 `RelayModeResponsesCompact` 时已拦截非 OpenAI/Codex 渠道，后续单独 Task）
- Thinking 功能的 Responses 协议侧 `reasoning` item 完整 summary 流式拼装（本次只保证 Responses 输出结构正确，thinking token 计入 `output_tokens_details.reasoning_tokens`，summary 文本透传基础实现）

---

## 2. 自动格式转换路由设计

### 入口识别（已就绪，[AUDITOR-CORRECTION]）

审计报告未明确，但路由已完整配置：
- `src/relay/constant/relay_mode.go:51`：`/v1/responses` → `RelayModeResponses`
- `src/relay/common/relay_info.go:349`：`RelayModeResponses` → `info.RelayFormat = types.RelayFormatOpenAIResponses`
- `src/relay/responses_handler.go:81`：调用 `adaptor.ConvertOpenAIResponsesRequest`

**结论**：路由注册已存在，唯一 broken 点是 Claude adaptor 里 `ConvertOpenAIResponsesRequest` 未实现。

### 出口识别（Claude Adaptor `DoResponse` 分支逻辑）

当前 `DoResponse`（`adaptor.go:97-103`）仅根据 `info.IsStream` 分发到 `ClaudeStreamHandler` / `ClaudeHandler`，两者内部再根据 `info.RelayFormat` 分支。需要在 `HandleClaudeResponseData` 和 `HandleStreamResponseData` 中各新增一个 `case types.RelayFormatOpenAIResponses` 分支，调用新建的转换函数。

**三分支路由（伪代码）**：
```
switch info.RelayFormat {
case types.RelayFormatClaude:         // Claude 透传
case types.RelayFormatOpenAI:         // Chat Completions 反向
case types.RelayFormatOpenAIResponses: // Responses 反向（本次新增）
}
```

`DoResponse` 本身不需要改动，由内层 switch 分发。

---

## 3. 文件级改动清单

### 文件 A：`src/relay/responses_handler.go`

| 位置 | 改动 | 对应 Finding |
|------|------|-------------|
| `ResponsesHelper` 函数入口，约第 37 行之后（请求类型 switch 之前） | 新增：若 `responsesReq.PreviousResponseID != ""`，立即 return `types.NewErrorWithStatusCode(errors.New("previous_response_id is not supported by stateless gateway"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())` | finding 15 |

**接口签名无变化。**

---

### 文件 B：`src/relay/reasonmap/reasonmap.go`

| 位置 | 改动 | 对应 Finding |
|------|------|-------------|
| `ClaudeStopReasonToOpenAIFinishReason` switch（第 10-23 行） | 新增 `case "pause_turn": return "incomplete"` | finding 11 |

**接口签名无变化。**

---

### 文件 C：`src/dto/claude.go`

| 位置 | 改动 | 对应 Finding |
|------|------|-------------|
| `ClaudeUsage` 结构体（第 529-539 行） | 新增字段 `ThinkingOutputTokens int \`json:"thinking_output_tokens,omitempty"\`` | finding 7 |

**说明**：Anthropic API 在 `message_delta` 事件的 `usage` 对象中返回 `thinking_output_tokens`（扩展思考消耗的 tokens），该字段当前被丢弃，导致计费 thinking tokens 漏算。

---

### 文件 D：`src/dto/openai_response.go`

#### D1 — 新增 `ResponsesUsage` 结构体（约第 265 行之前新插入）

```
// 字段定义规格（不是代码，是 spec）：
type ResponsesOutputTokensDetails struct {
    ReasoningTokens int  // json:"reasoning_tokens"
}

type ResponsesUsage struct {
    InputTokens         int                           // json:"input_tokens"
    OutputTokens        int                           // json:"output_tokens"
    TotalTokens         int                           // json:"total_tokens"
    InputTokensDetails  *InputTokenDetails            // json:"input_tokens_details,omitempty"
    OutputTokensDetails *ResponsesOutputTokensDetails // json:"output_tokens_details,omitempty"
}
```

#### D2 — `OpenAIResponsesResponse.Usage` 类型变更

将第 286 行 `Usage *Usage` 改为 `Usage *ResponsesUsage`，影响：
- `src/relay/channel/openai/relay_responses.go` 中读取 `Usage.InputTokens` / `Usage.OutputTokens` 的代码需同步更新字段引用（`Usage.TotalTokens` 在 `ResponsesUsage` 中仍存在）
- 其他直接引用 `OpenAIResponsesResponse.Usage` 的地方（全局 grep 确认后修改）

#### D3 — `ResponsesStreamResponse` 补字段

在第 376-387 行结构体中新增 `SequenceNumber int \`json:"sequence_number,omitempty"\``，用于 Responses 流式 SSE 事件的序号维护。

**对应 Finding**：finding 7、12、13。

---

### 文件 E：`src/service/openaicompat/chat_to_responses.go`

| 位置 | 改动 | 对应 Finding |
|------|------|-------------|
| 第 349-354 行，`Summary: "detailed"` | 改为：若 `req.Include` 字段（`json.RawMessage`，来自客户端请求）中包含 `"reasoning.encrypted_content"` 字符串，则 `Summary = "auto"`；否则 `Summary = "detailed"`。若 `req.Include` 为空，默认 `"detailed"` | finding 14 |

**说明**：`req` 是 `dto.GeneralOpenAIRequest`，该结构体无 `Include` 字段（Responses 专属），此函数仅由 Chat→Responses 桥接调用，无需处理 `Include`——直接改为 `Summary = "detailed"` 条件化为：若 `req.ReasoningEffort != ""` 则 `"detailed"` 否则不设置 Summary。

**[AUDITOR-CORRECTION]**：审计说"按请求 include 字段决定"，但 `GeneralOpenAIRequest` 无 `include` 字段，`chat_to_responses.go` 的输入是 Chat 格式请求。正确做法：仅当 `req.ReasoningEffort != ""` 时才设置 Reasoning 字段，Summary 默认 `"detailed"` 保持不变即可（`"detailed"` 是最安全的默认值，不改变行为）。本次修复仅把硬编码改为函数返回值以便未来扩展，实际值仍为 `"detailed"`。

---

### 文件 F：`src/relay/channel/claude/relay-claude.go`

#### F1 — 修复 `budget_tokens` 表（第 166-184 行）

重新定义 `reasoning.effort` 到 `BudgetTokens` 的映射：

| effort | 新值 | 旧值 |
|--------|------|------|
| low    | 1024 | 1280（保持，已合理）|
| medium | 8192 | 2048 |
| high   | max(16384, int(float64(maxTokens)*0.8)) | 4096 |

`high` 档取 `16384` 与 `maxTokens*0.8` 中的较大值（但不超过 `maxTokens-1`，Anthropic 要求 `budget_tokens < max_tokens`）。

**对应 Finding**：finding 5。

#### F2 — 修复图片处理（第 333-348 行）

在 `RequestOpenAI2ClaudeMessage` 中，图片处理分支：
```
if strings.HasPrefix(imageUrl.Url, "http") {
    // 当前: 仍走 GetBase64Data 下载再 base64 编码
    // 修改后: ClaudeMessageSource{Type: "url", Url: imageUrl.Url}，跳过 GetBase64Data
```

修改后逻辑：
- URL 以 `http://` 或 `https://` 开头 → `Source = &ClaudeMessageSource{Type: "url", Url: imageUrl.Url}`
- 否则（data URI / base64 原始串）→ 维持现有 `GetBase64Data` 流程，Type 为 `"base64"`

**`ClaudeMessageSource.Url` 字段已存在（`src/dto/claude.go:106`），不需要新增字段。**

**对应 Finding**：finding 4。

#### F3 — 修复 `fcIdx` 独立计数（第 389-395 行）

`StreamResponseClaude2OpenAI` 中，`fcIdx` 当前取 `*claudeResponse.Index - 1`（Anthropic 的全局 content block index），但 OpenAI 的 `tool_calls[].index` 应是工具调用序号（从 0 开始独立递增）。

修复方案：在 `ClaudeResponseInfo` 结构体（第 528-535 行）中新增字段 `ToolCallCounter int`，在 `content_block_start` 事件且 block type 为 `tool_use` 时递增，用 `claudeInfo.ToolCallCounter` 替代 `fcIdx`（在 `HandleStreamResponseData` 调用链中传递 `claudeInfo`，或改造 `StreamResponseClaude2OpenAI` 接受 `claudeInfo *ClaudeResponseInfo` 参数）。

**接口签名变更**：`StreamResponseClaude2OpenAI(claudeResponse *dto.ClaudeResponse, claudeInfo *ClaudeResponseInfo) *dto.ChatCompletionsStreamResponse`（新增 `claudeInfo` 参数）。

**对应 Finding**：finding 9。

#### F4 — 修复 `signature_delta` 透传（第 437-440 行）

Anthropic `thinking` block 的 `signature_delta` 事件包含加密签名，多轮对话续杯时必须原样返回给 Anthropic（放在 `ClaudeMediaMessage.Signature`），当前被替换为 `"\n"` 丢弃。

- 对 `RelayFormatOpenAI`：signature 透传到 `choice.Delta.ReasoningContent`（使用约定分隔符 `||SIG:` + base64 encode，让客户端可以提取后在多轮时回传）
- 对 `RelayFormatOpenAIResponses`：signature 透传策略见 §7 风险登记
- 对 `RelayFormatClaude`：已由 `ClaudeChunkData` 原样透传，无需改动

**`ClaudeResponseInfo` 新增字段 `ThinkingSignature string`**，在 `FormatClaudeResponseInfo` 的 `content_block_delta/signature_delta` 分支中累积，供多轮场景使用（本次仅保存，不实现多轮重注入）。

**对应 Finding**：finding 6。

#### F5 — 修复 `tool_use` arguments 为空时丢弃（第 354-358 行）

`RequestOpenAI2ClaudeMessage` 中，当 `message.ToolCalls` 时：
```go
if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &inputObj); err != nil {
    continue  // BUG: 工具调用被整体丢弃
}
```

修改为：arguments 为空字符串或 `"{}"` 时，`inputObj = map[string]any{}`（空 map），不 continue。仅在 Unmarshal 失败且非空字符串时才 continue（并打 warn 日志）。

**对应 Finding**：finding 8。

#### F6 — 修复 `message_start` 重复发 content（第 397-404 行）

`StreamResponseClaude2OpenAI` 中，`message_start` 事件发送 `choice.Delta.SetContentString("")`（空字符串），某些 OpenAI 兼容客户端会把空 content 视为 content block，再收到 `content_block_start(text)` 时重复追加。

修改：`message_start` 时不设置 `choice.Delta.Content`（保持 nil），只设置 `choice.Delta.Role = "assistant"`。

**对应 Finding**：finding 10。

#### F7 — `HandleStreamResponseData` 新增 `RelayFormatOpenAIResponses` 分支（第 623-644 行）

在现有 `} else if info.RelayFormat == types.RelayFormatOpenAI {` 之后新增：
```
} else if info.RelayFormat == types.RelayFormatOpenAIResponses {
    // 调用 responses.go 中的 StreamClaude2Responses
    apiErr := StreamClaude2Responses(c, claudeInfo, &claudeResponse)
    if apiErr != nil {
        return apiErr
    }
```

#### F8 — `HandleClaudeResponseData` 新增 `RelayFormatOpenAIResponses` 分支（第 720-730 行）

在 switch 的 `case types.RelayFormatClaude:` 之前新增：
```
case types.RelayFormatOpenAIResponses:
    responsesResponse, err := ClaudeResponse2Responses(&claudeResponse, claudeInfo)
    if err != nil {
        return types.NewError(err, types.ErrorCodeBadResponseBody)
    }
    responseData, err = json.Marshal(responsesResponse)
    if err != nil {
        return types.NewError(err, types.ErrorCodeBadResponseBody)
    }
```

#### F9 — `HandleStreamFinalResponse` 新增 `RelayFormatOpenAIResponses` 收尾（第 659-671 行）

```
} else if info.RelayFormat == types.RelayFormatOpenAIResponses {
    // 发送 response.completed 事件
    completedEvent := buildResponsesCompletedEvent(claudeInfo)
    err := helper.ObjectData(c, completedEvent)
    ...
    helper.Done(c)
}
```

---

### 文件 G：新建 `src/relay/channel/claude/responses.go`

**职责**：存放 Responses ↔ Claude 双向转换的所有 helper 函数，避免 `relay-claude.go` 继续膨胀。

#### G1 — `StreamState` 结构体

```
// 字段规格（不是代码）
type StreamState struct {
    SeqNo           int    // 全局事件序号，每发一个 SSE 事件 +1
    CurOutputIdx    int    // 当前 output item index（对应 output_index）
    CurItemID       string // 当前 output item 的 id（msg_<uuid> / fc_<uuid> / rs_<uuid>）
    ToolCallCounter int    // 独立工具调用计数器，替代 fcIdx
    ActiveBlockType string // "text" | "tool_use" | "thinking"
    TextContentIdx  int    // 当前 text content block 的 content_index
}
```

`StreamState` 挂在 `ClaudeResponseInfo` 上（新增 `StreamState *StreamState` 字段），在 `ClaudeStreamHandler` 初始化时创建。

#### G2 — `RequestResponses2Claude(req dto.OpenAIResponsesRequest) (*dto.ClaudeRequest, error)`

将 `OpenAIResponsesRequest` 转为 `ClaudeRequest`，完整字段映射见 §4 请求映射表。

#### G3 — `ClaudeResponse2Responses(resp *dto.ClaudeResponse, info *ClaudeResponseInfo) (*dto.OpenAIResponsesResponse, error)`

非流式转换，完整字段映射见 §4 非流式响应映射表。

#### G4 — `StreamClaude2Responses(c *gin.Context, info *ClaudeResponseInfo, event *dto.ClaudeResponse) *types.NewAPIError`

流式事件逐条转换，映射逻辑见 §4 流式 SSE 事件映射表。通过 `info.StreamState` 维护状态。

#### G5 — `buildResponsesCompletedEvent(info *ClaudeResponseInfo) *dto.ResponsesStreamResponse`

生成最终的 `response.completed` 事件，包含完整的 `ResponsesUsage`。

---

### 文件 H：`src/relay/channel/claude/adaptor.go`

#### H1 — 实现 `ConvertOpenAIResponsesRequest`（第 88-91 行）

```
func (a *Adaptor) ConvertOpenAIResponsesRequest(...) (any, error) {
    return RequestResponses2Claude(request)
}
```

调用 `responses.go` 中的 `RequestResponses2Claude`。

#### H2 — `DoResponse` 分支（可选，当前结构已能通过内层 switch 分发）

`DoResponse` 当前根据 `info.IsStream` 调用 `ClaudeStreamHandler` / `ClaudeHandler`，两者内部均有 `RelayFormat` switch。不需要改 `DoResponse` 签名或结构，**只需确保内层 switch 覆盖 `RelayFormatOpenAIResponses`**（即上述 F7/F8 已覆盖）。

---

## 4. 关键数据结构映射表

### 4.1 请求：Responses → Anthropic

| Responses 字段 | Anthropic 字段 | 备注 |
|--------------|--------------|------|
| `model` | `model` | 直接映射 |
| `input` (string) | `messages[0]` role=user, content=string | input 为字符串时 |
| `input` (array) | `messages[]` | 按 role 展开；`function_call` → assistant role + `tool_use` block；`function_call_output` → user role + `tool_result` block；`tool_use_id` 来自 `call_id` |
| `instructions` | `system` (array of text blocks) | JSON string → `[{type:"text", text:...}]` |
| `max_output_tokens` | `max_tokens` | 0 时取模型默认值 |
| `stream` | `stream` | 直接映射 |
| `temperature` | `temperature` | 直接映射 |
| `top_p` | `top_p` | 直接映射 |
| `tools[]` type=function | `tools[]` | `{name, description, input_schema:parameters}` |
| `tools[]` type=web_search_preview | `tools[]` type=web_search_20250305 | name=web_search，`max_uses` 按 `search_context_size` 映射（low=1/medium=5/high=10）|
| `tools[]` type=file_search | 拒绝，返回 400 "file_search is not supported on Anthropic upstream" | 内置工具不支持 |
| `tools[]` type=computer_use_preview | 拒绝，返回 400（本网关无法代理 computer_use 状态机） | 内置工具不支持 |
| `tool_choice` "auto" | `tool_choice.type="auto"` | |
| `tool_choice` "required" | `tool_choice.type="any"` | |
| `tool_choice` "none" | `tool_choice.type="none"` | |
| `tool_choice` {type:"function", name:...} | `tool_choice.type="tool", name=...` | |
| `reasoning.effort` "low" | `thinking.budget_tokens = max(1024, ...)` | |
| `reasoning.effort` "medium" | `thinking.budget_tokens = 8192` | |
| `reasoning.effort` "high" | `thinking.budget_tokens = max(16384, maxTokens*0.8)`，且 `< max_tokens` | |
| `reasoning.effort` 非空时 | `temperature=1.0, top_p=0`（Anthropic 扩展思考要求） | |
| `previous_response_id` 非空 | **请求前拦截**，返回 HTTP 400（在 `responses_handler.go` 处理） | finding 15 |
| `parallel_tool_calls` | `tool_choice.disable_parallel_tool_use = !value` | |
| `metadata` | `metadata` | 透传 |
| `text.format` | 忽略（Anthropic 不支持 JSON mode via Responses 协议的等价字段） | Out-scope |
| `truncation` | 忽略 | Out-scope |

**tool_use / tool_result 配对规则**：
- 扫描 input array，`{type:"function_call", call_id, name, arguments}` → assistant message 中的 `tool_use` block，`id=call_id`，`input=JSON.parse(arguments)`（arguments 为空时 `input={}`）
- `{type:"function_call_output", call_id, output}` → user message 中的 `tool_result` block，`tool_use_id=call_id`，`content=output`
- 连续的 function_call 合并到同一 assistant message 的 content array
- 连续的 function_call_output 合并到同一 user message 的 content array

---

### 4.2 非流式响应：Anthropic → Responses

| Anthropic 字段 | Responses 字段 | ID 生成规则 | 备注 |
|--------------|--------------|------------|------|
| `response.id` | `response.id` | 直接使用 | |
| `response.model` | `response.model` | 直接使用 | |
| `"message"` | `response.object` | 硬编码 `"response"` | |
| `"completed"` | `response.status` | 由 stop_reason 决定：`end_turn/stop_sequence` → `"completed"`；`max_tokens` → `"incomplete"`；`pause_turn` → `"incomplete"` | |
| content block type=`text` | `output[]` type=`"message"`, role=`"assistant"`, content=[{type:"output_text", text:..., annotations:[]}] | item.id = `msg_<GetUUID()>` | |
| content block type=`tool_use` | `output[]` type=`"function_call"`, call_id=block.id, name=block.name, arguments=JSON.Marshal(block.input) | item.id = `fc_<GetUUID()>` | |
| content block type=`thinking` | `output[]` type=`"reasoning"`, content=[{type:"reasoning_summary", text:thinking_text}] | item.id = `rs_<GetUUID()>` | signature 处理见 §7 |
| `usage.input_tokens` | `usage.input_tokens` | | |
| `usage.output_tokens` | `usage.output_tokens` | | |
| `usage.thinking_output_tokens`（新字段） | `usage.output_tokens_details.reasoning_tokens` | | finding 7 |
| `usage.input_tokens + output_tokens` | `usage.total_tokens` | | |
| `usage.cache_read_input_tokens` | `usage.input_tokens_details.cached_tokens` | | |
| `stop_reason` → `reasonmap` | `response.incomplete_details`（当 status=incomplete 时） | | |

---

### 4.3 流式 SSE 事件映射（Anthropic → Responses Protocol）

**StreamState 维护规则**：
- `SeqNo`：每发出一个 SSE 事件后 +1
- `CurOutputIdx`：每新增一个 output item 时 +1
- `CurItemID`：新 item 时生成；text block → `msg_<uuid>`；tool_use → `fc_<uuid>`；thinking → `rs_<uuid>`
- `ToolCallCounter`：仅 tool_use content_block_start 时 +1
- `ActiveBlockType`：当前活跃 content block 类型
- `TextContentIdx`：text content block 在 message output item 中的 index，每个 text block 开始时置 0

| Anthropic 事件 | 发出的 Responses 事件（按顺序） | StreamState 变化 |
|--------------|---------------------------|----------------|
| `message_start` | 1. `response.created` (含 response 骨架，output=[]) | SeqNo++ |
| `content_block_start` (text) | 1. `response.output_item.added` (type=message, role=assistant, content=[]) 2. `response.content_part.added` (type=output_text, text="") | CurOutputIdx++, CurItemID=msg_xxx, ActiveBlockType=text, SeqNo+=2 |
| `content_block_start` (tool_use) | 1. `response.output_item.added` (type=function_call, call_id=block.id, name=block.name, arguments="") | CurOutputIdx++, CurItemID=fc_xxx, ToolCallCounter++, ActiveBlockType=tool_use, SeqNo++ |
| `content_block_start` (thinking) | 1. `response.output_item.added` (type=reasoning, content=[]) 2. `response.reasoning_summary_part.added` (type=reasoning_summary, text="") | CurOutputIdx++, CurItemID=rs_xxx, ActiveBlockType=thinking, SeqNo+=2 |
| `content_block_delta` (text_delta) | 1. `response.output_text.delta` (output_index=CurOutputIdx, content_index=0, delta=text) | SeqNo++ |
| `content_block_delta` (input_json_delta) | 1. `response.function_call_arguments.delta` (output_index=CurOutputIdx, delta=partial_json) | SeqNo++ |
| `content_block_delta` (thinking_delta) | 1. `response.reasoning_summary_text.delta` (output_index=CurOutputIdx, summary_index=0, delta=thinking) | SeqNo++ |
| `content_block_delta` (signature_delta) | 不发出 SSE（signature 内部保存到 claudeInfo.ThinkingSignature，见 §7） | — |
| `content_block_stop` (text) | 1. `response.output_text.done` (output_index=CurOutputIdx, content_index=0, text=完整文本) 2. `response.content_part.done` | SeqNo+=2 |
| `content_block_stop` (tool_use) | 1. `response.function_call_arguments.done` (output_index=CurOutputIdx, arguments=完整JSON) | SeqNo++ |
| `content_block_stop` (thinking) | 1. `response.reasoning_summary_text.done` (output_index=CurOutputIdx, summary_index=0, text=完整thinking) 2. `response.reasoning_summary_part.done` | SeqNo+=2 |
| `message_delta` (stop_reason) | 1. 如有当前未完成 item → `response.output_item.done` | SeqNo++ |
| `message_stop` | 1. `response.completed` (含完整 ResponsesUsage) | SeqNo++ |

**每个 SSE 事件必须包含 `sequence_number` 字段（当前 `ResponsesStreamResponse` 中新增的字段）**，值为发出前的 `SeqNo`。

---

## 5. 实施顺序（Backend 作战手册）

每步完成后独立可编译，可单独验证。

**Step 1 — DTO 层**（1-2h）
- 改 `src/dto/claude.go`：`ClaudeUsage` 加 `ThinkingOutputTokens`（Finding 7）
- 改 `src/dto/openai_response.go`：新增 `ResponsesOutputTokensDetails`、`ResponsesUsage` 结构体；改 `OpenAIResponsesResponse.Usage` 类型；`ResponsesStreamResponse` 加 `SequenceNumber`（Finding 7/12/13）
- 同步修改 `src/relay/channel/openai/relay_responses.go` 中读取 `Usage` 字段的代码（因类型变更）

**Step 2 — reasonmap**（0.5h）
- 改 `src/relay/reasonmap/reasonmap.go`：加 `pause_turn` 映射（Finding 11）

**Step 3 — ClaudeResponseInfo 扩展 + StreamState 结构体骨架**（0.5h）
- 改 `src/relay/channel/claude/relay-claude.go`：`ClaudeResponseInfo` 加 `StreamState *StreamState` 和 `ThinkingSignature string`
- 新建 `src/relay/channel/claude/responses.go`：定义 `StreamState` 结构体，占位函数骨架（返回 nil/not implemented，保证编译通过）

**Step 4 — 非流式转换实现**（3-4h）
- `responses.go`：实现 `ClaudeResponse2Responses`（非流式完整转换）
- 实现 `buildResponsesUsage(claudeUsage *dto.ClaudeUsage) *dto.ResponsesUsage`

**Step 5 — HandleClaudeResponseData 接线**（0.5h）
- `relay-claude.go`：在 switch 中加 `case types.RelayFormatOpenAIResponses`，调用 `ClaudeResponse2Responses`（Finding 2）

**Step 6 — 流式转换实现**（4-5h）
- `responses.go`：实现 `StreamClaude2Responses` + `StreamState` 事件状态机（按 §4.3 映射表实现全部 12 类事件）
- 实现 `buildResponsesCompletedEvent`

**Step 7 — HandleStreamResponseData 接线 + HandleStreamFinalResponse 接线**（0.5h）
- `relay-claude.go`：加 `RelayFormatOpenAIResponses` 分支（Finding 3）
- `HandleStreamFinalResponse` 加 `RelayFormatOpenAIResponses` 收尾

**Step 8 — ConvertOpenAIResponsesRequest 实现**（3-4h）
- `responses.go`：实现 `RequestResponses2Claude`（Responses → Claude 请求转换，按 §4.1 映射表）
- `adaptor.go`：`ConvertOpenAIResponsesRequest` 调用 `RequestResponses2Claude`（Finding 1）

**Step 9 — HIGH/MEDIUM bug 修复**（2-3h）
- `relay-claude.go`：F1（budget_tokens），F2（图片 URL），F3（fcIdx），F4（signature），F5（arguments 为空），F6（message_start content）
- `chat_to_responses.go`：Summary 条件化（Finding 14）

**Step 10 — previous_response_id 拒绝**（0.5h）
- `responses_handler.go`：入口检查 `PreviousResponseID` 非空返回 400（Finding 15）

---

## 6. 测试策略

### 单元测试文件位置

| 测试函数 | 文件路径 |
|--------|--------|
| `TestRequestResponses2Claude` | `src/relay/channel/claude/responses_test.go` |
| `TestClaudeResponse2Responses` | `src/relay/channel/claude/responses_test.go` |
| `TestStreamClaude2Responses` | `src/relay/channel/claude/responses_test.go` |
| `TestBudgetTokensMapping` | `src/relay/channel/claude/relay_claude_test.go` |
| `TestClaudeStopReasonMapping` | `src/relay/reasonmap/reasonmap_test.go` |

每个测试函数使用 table-driven 模式，至少覆盖：happy path、空输入、边界值（如 arguments=""）。

### 集成端到端用例

**用例 1 — 纯文本非流式**
```bash
curl -X POST http://localhost:3000/v1/responses \
  -H "Authorization: Bearer <key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "input": "Hello, world!",
    "stream": false
  }'
# 期望：HTTP 200, body.object="response", body.status="completed",
#        body.output[0].type="message", body.output[0].role="assistant",
#        body.output[0].content[0].type="output_text",
#        body.usage.input_tokens > 0, body.usage.output_tokens > 0
```

**用例 2 — 含 tool_use 流式**
```bash
curl -X POST http://localhost:3000/v1/responses \
  -H "Authorization: Bearer <key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "input": "What is the weather in Beijing?",
    "tools": [{"type":"function","name":"get_weather","description":"Get weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}],
    "stream": true
  }'
# 期望：SSE 流，包含 response.created, response.output_item.added (type=function_call),
#        response.function_call_arguments.delta, response.function_call_arguments.done,
#        response.output_item.done, response.completed
#        每个事件有 sequence_number 递增
```

**用例 3 — 含 thinking + tool_use 多轮续杯**
```bash
# 第一轮：触发 thinking + tool_use
curl -X POST http://localhost:3000/v1/responses \
  -d '{"model":"claude-3-5-sonnet-20241022","input":"Complex math problem","reasoning":{"effort":"high"},"tools":[...],"stream":false}'
# 期望：output 包含 type=reasoning 和 type=function_call
# usage.output_tokens_details.reasoning_tokens > 0

# 第二轮（模拟 function_call_output）：
curl -X POST http://localhost:3000/v1/responses \
  -d '{"model":"claude-3-5-sonnet-20241022","input":[{"type":"function_call_output","call_id":"<上轮call_id>","output":"42"}]}'
# 期望：HTTP 200, output 包含最终 message
```

### 回归保护

- `RelayFormatOpenAI` 路径（Chat Completions → Claude）：运行现有 `TestStreamResponseClaude2OpenAI` 等测试，结果不变
- `RelayFormatClaude` 路径（Claude 透传）：`ClaudeChunkData` 调用路径不变，现有测试通过
- `RequestOpenAI2ClaudeMessage` 核心逻辑：图片 URL 处理修改后运行图片相关集成测试

---

## 7. 风险登记

### R1 — Anthropic 内置工具（web_search/computer_use）客户端发来怎么办？

- `web_search_preview` → 转换为 Anthropic `web_search_20250305` 工具（§4.1 已覆盖）
- `computer_use_preview` → `RequestResponses2Claude` 内检测到 type=computer_use_preview 时返回 error：`"computer_use is not supported"` → HTTP 400，跳过重试
- `file_search` → 同上，返回 400

**结论**：除 web_search 外，其余内置工具主动拒绝，在 `RequestResponses2Claude` 函数内处理。

### R2 — `previous_response_id` 拒绝后客户端 SDK 兼容性影响

OpenAI Python SDK（openai>=1.x）在 `previous_response_id` 存在时不会自动 fallback，会直接抛出 `BadRequestError`。影响范围：仅使用多轮对话状态管理（stateful conversation）功能的调用者。缓解：error message 明确说明 `"stateless gateway does not support previous_response_id; manage conversation history client-side via input array"`，帮助用户快速定位。

### R3 — streaming 首个 token 延迟

Responses 协议要求 `response.created` 事件先于所有 delta 事件，但 Anthropic `message_start` 已是第一个事件，转换为 `response.created` 无额外延迟。风险：低。

### R4 — signature 在 Responses 协议里的透传

Responses 协议的 `reasoning` output item 无原生 `signature` 字段。方案决策：

**结论：非流式响应中，signature 附加到 reasoning output item 的最后一个 summary content 的 `text` 尾部，格式为 `\n\n[SIGNATURE:<base64-encoded-signature>]`。流式中，`signature_delta` 事件不发送 SSE，signature 在 `response.completed` 事件的 `response.output[]` 对应 reasoning item 的 summary text 尾部附加。**

理由：Responses 协议无扩展字段机制，放入 text 尾部是唯一不破坏协议 schema 的方式；客户端若需多轮续杯，解析 `[SIGNATURE:...]` 提取后放回 Anthropic 请求即可。此方案不完美但可用，且与 Anthropic 官方文档保持结构兼容。

**[AUDITOR-MISSED] Finding 16 — `FormatClaudeResponseInfo` 中 thinking tokens 未计入 `CompletionTokens`**

`relay-claude.go:559-570`：`message_delta` 的 usage 更新仅读取 `OutputTokens`，Anthropic 返回的 `thinking_output_tokens` 未累加到 `CompletionTokens`，导致 thinking 场景下计费少算。

**修复**：Step 1 DTO 层加字段后，在 `FormatClaudeResponseInfo` 的 `message_delta` 分支中：`claudeInfo.Usage.CompletionTokens = claudeResponse.Usage.OutputTokens + claudeResponse.Usage.ThinkingOutputTokens`，并设置 `claudeInfo.Usage.CompletionTokenDetails.ReasoningTokens = claudeResponse.Usage.ThinkingOutputTokens`。

**[AUDITOR-MISSED] Finding 17 — `HandleClaudeResponseData` 中 thinking tokens 同样未计入**

`relay-claude.go:710-717`：非流式路径 `ClaudeHandler` 中 `Usage.CompletionTokens = claudeResponse.Usage.OutputTokens`，未加 `ThinkingOutputTokens`。同步修复。

---

## 8. 与下游 Agent 的交接

1. **后端开发师**：按 §5 实施顺序逐步实现，每步保证独立可编译
2. **代码审计师**：实现完成后，以本方案文档为基准进行 Code Review，重点验证：mapping table 覆盖度、StreamState 事件顺序、错误处理路径
3. **功能测试师**：执行 §6 三个端到端用例 + 回归测试
4. **界面测试师**：如有前端客户端验证 Responses 流式 SSE 事件序号
5. **测试总监师**：裁决是否可验收
6. **运维部署工程师**：待用户补充部署路径和重启方式后上线

---

## Definition of Done

- [ ] `curl` 纯文本非流式请求到 Anthropic 渠道返回 HTTP 200，`body.object="response"`, `body.output[0].type="message"`, `body.usage.input_tokens > 0`
- [ ] `curl` 含 tool_use 流式请求，SSE 事件中 `response.function_call_arguments.delta` 出现，`response.completed` 出现，每个事件 `sequence_number` 严格递增
- [ ] `curl` 含 `reasoning.effort=high` 请求，`body.usage.output_tokens_details.reasoning_tokens > 0`，且计费 `CompletionTokens >= OutputTokens + ThinkingOutputTokens`
- [ ] `curl` 含 `previous_response_id` 非空请求返回 HTTP 400，`error.message` 包含 `"previous_response_id"`
- [ ] 现有 Chat Completions → Claude 路径（`POST /v1/chat/completions` + Anthropic 渠道）所有现有测试通过，无回归
- [ ] `go build ./...` 无编译错误
- [ ] `go test ./src/relay/channel/claude/...` 新增单元测试全部通过
