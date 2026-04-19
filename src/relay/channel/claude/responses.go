package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relay/reasonmap"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// StreamState maintains state for streaming Responses protocol conversion.
// It is embedded in ClaudeResponseInfo and initialized by ClaudeStreamHandler.
type StreamState struct {
	SeqNo           int    // global SSE event sequence number
	CurOutputIdx    int    // current output item index (0-based)
	CurItemID       string // current output item id (msg_xxx / fc_xxx / rs_xxx)
	ToolCallCounter int    // independent tool-call counter (replaces fcIdx for Responses)
	ActiveBlockType string // "text" | "tool_use" | "thinking"
	TextContentIdx  int    // content_index for text content block
	CurToolCallId   string // call_id for current tool_use block
	CurToolName     string // name for current tool_use block
	// accumulated text / arguments for done events
	AccText      strings.Builder
	AccArguments strings.Builder
	AccThinking  strings.Builder
	// completed output items snapshot for response.completed
	CompletedOutputItems []dto.ResponsesOutput
}

// nextSeq returns the current sequence number and increments it.
func (s *StreamState) nextSeq() int {
	n := s.SeqNo
	s.SeqNo++
	return n
}

// intPtr is a local helper to get *int.
func intPtr(i int) *int { return &i }

// ─────────────────────────────────────────────────────────────────────────────
// RequestResponses2Claude converts an OpenAI Responses API request to a
// Anthropic ClaudeRequest.  This is the implementation for Finding 1.
// ─────────────────────────────────────────────────────────────────────────────

func RequestResponses2Claude(req dto.OpenAIResponsesRequest) (*dto.ClaudeRequest, error) {
	// Finding 15: previous_response_id is stateful, not supported.
	// (Primary check is in responses_handler.go; guard here as defense-in-depth.)
	if req.PreviousResponseID != "" {
		return nil, errors.New("stateful responses are not supported: previous_response_id must be empty")
	}

	claudeReq := dto.ClaudeRequest{
		Model:  req.Model,
		Stream: req.Stream,
	}

	// ── max_tokens ─────────────────────────────────────────────────────────
	if req.MaxOutputTokens > 0 {
		claudeReq.MaxTokens = req.MaxOutputTokens
	} else {
		claudeReq.MaxTokens = uint(model_setting.GetClaudeSettings().GetDefaultMaxTokens(req.Model))
		if claudeReq.MaxTokens == 0 {
			claudeReq.MaxTokens = 4096 // absolute fallback
		}
	}

	// ── temperature / top_p ────────────────────────────────────────────────
	if req.Temperature != nil {
		claudeReq.Temperature = req.Temperature
	}
	if req.TopP != nil {
		claudeReq.TopP = *req.TopP
	}

	// ── stop sequences ─────────────────────────────────────────────────────
	// OpenAI Responses API does not have a stop field; nothing to map.

	// ── metadata ───────────────────────────────────────────────────────────
	if len(req.Metadata) > 0 {
		claudeReq.Metadata = req.Metadata
	}

	// ── reasoning.effort → thinking ────────────────────────────────────────
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		maxTokens := int(claudeReq.MaxTokens)
		switch req.Reasoning.Effort {
		case "low":
			claudeReq.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer(1024),
			}
		case "medium":
			claudeReq.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer(8192),
			}
		case "high":
			highBudget := 16384
			if budget := int(float64(maxTokens) * 0.8); budget > highBudget {
				highBudget = budget
			}
			if highBudget >= maxTokens && maxTokens > 1 {
				highBudget = maxTokens - 1
			}
			claudeReq.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer(highBudget),
			}
		}
		// Anthropic extended thinking requires temperature=1.0 and top_p=0
		claudeReq.Temperature = common.GetPointer(1.0)
		claudeReq.TopP = 0
	}

	// ── system / instructions ──────────────────────────────────────────────
	if len(req.Instructions) > 0 {
		var instrStr string
		if err := json.Unmarshal(req.Instructions, &instrStr); err == nil && instrStr != "" {
			claudeReq.System = []dto.ClaudeMediaMessage{
				{Type: "text", Text: common.GetPointer(instrStr)},
			}
		} else {
			// Try as array of text blocks
			var blocks []dto.ClaudeMediaMessage
			if err2 := json.Unmarshal(req.Instructions, &blocks); err2 == nil {
				claudeReq.System = blocks
			}
		}
	}

	// ── tools ──────────────────────────────────────────────────────────────
	if len(req.Tools) > 0 {
		claudeTools, err := responsesToolsToClaudeTools(req.Tools)
		if err != nil {
			return nil, err
		}
		claudeReq.Tools = claudeTools
	}

	// ── tool_choice ────────────────────────────────────────────────────────
	if len(req.ToolChoice) > 0 {
		tc, err := responsesToolChoiceToClaudeToolChoice(req.ToolChoice)
		if err != nil {
			common.SysLog("responses tool_choice parse failed: " + err.Error())
		} else if tc != nil {
			claudeReq.ToolChoice = tc
		}
	}

	// ── parallel_tool_calls ────────────────────────────────────────────────
	if len(req.ParallelToolCalls) > 0 {
		var parallel bool
		if err := json.Unmarshal(req.ParallelToolCalls, &parallel); err == nil {
			tc := claudeReq.ToolChoice
			if tc == nil && !parallel {
				// Only set disable_parallel when there is a tool_choice context
				claudeReq.ToolChoice = &dto.ClaudeToolChoice{
					Type:                   "auto",
					DisableParallelToolUse: true,
				}
			} else if existing, ok := tc.(*dto.ClaudeToolChoice); ok {
				if existing.Type != "none" {
					existing.DisableParallelToolUse = !parallel
				}
			}
		}
	}

	// ── messages (from input) ──────────────────────────────────────────────
	messages, err := responsesInputToClaudeMessages(req.Input)
	if err != nil {
		return nil, err
	}
	claudeReq.Messages = messages

	return &claudeReq, nil
}

// responsesToolsToClaudeTools converts Responses tools JSON to Claude tools.
func responsesToolsToClaudeTools(toolsRaw json.RawMessage) ([]any, error) {
	var toolsList []map[string]any
	if err := json.Unmarshal(toolsRaw, &toolsList); err != nil {
		return nil, fmt.Errorf("parse tools: %w", err)
	}

	claudeTools := make([]any, 0, len(toolsList))
	warnedStrict := false

	for _, t := range toolsList {
		toolType, _ := t["type"].(string)
		switch toolType {
		case "function":
			name, _ := t["name"].(string)
			desc, _ := t["description"].(string)
			params, _ := t["parameters"].(map[string]any)

			// Warn about unsupported strict field (once)
			if _, hasStrict := t["strict"]; hasStrict && !warnedStrict {
				common.SysLog("responses2claude: 'strict' field in function tool is not supported by Anthropic and will be ignored")
				warnedStrict = true
			}

			claudeTool := &dto.Tool{
				Name:        name,
				Description: desc,
				InputSchema: make(map[string]interface{}),
			}
			if params != nil {
				for k, v := range params {
					claudeTool.InputSchema[k] = v
				}
			}
			claudeTools = append(claudeTools, claudeTool)

		case dto.BuildInToolWebSearchPreview, "web_search_preview_2025_03_11":
			// Map to Anthropic web_search_20250305
			webTool := &dto.ClaudeWebSearchTool{
				Type: "web_search_20250305",
				Name: "web_search",
			}
			// search_context_size → max_uses
			if ctx, ok := t["search_context_size"].(string); ok {
				switch ctx {
				case "low":
					webTool.MaxUses = WebSearchMaxUsesLow
				case "medium":
					webTool.MaxUses = WebSearchMaxUsesMedium
				case "high":
					webTool.MaxUses = WebSearchMaxUsesHigh
				}
			}
			claudeTools = append(claudeTools, webTool)

		case dto.BuildInToolFileSearch:
			return nil, fmt.Errorf("file_search is not supported on Anthropic upstream")

		case "computer_use_preview":
			return nil, fmt.Errorf("computer_use is not supported on this gateway")

		default:
			common.SysLog(fmt.Sprintf("responses2claude: unknown tool type '%s', skipping", toolType))
		}
	}

	return claudeTools, nil
}

// responsesToolChoiceToClaudeToolChoice converts Responses tool_choice JSON to Claude format.
func responsesToolChoiceToClaudeToolChoice(raw json.RawMessage) (*dto.ClaudeToolChoice, error) {
	// Try string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto":
			return &dto.ClaudeToolChoice{Type: "auto"}, nil
		case "none":
			return &dto.ClaudeToolChoice{Type: "none"}, nil
		case "required":
			return &dto.ClaudeToolChoice{Type: "any"}, nil
		default:
			return &dto.ClaudeToolChoice{Type: "auto"}, nil
		}
	}

	// Try object
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	tcType, _ := obj["type"].(string)
	switch tcType {
	case "function":
		name, _ := obj["name"].(string)
		return &dto.ClaudeToolChoice{Type: "tool", Name: name}, nil
	case "auto":
		return &dto.ClaudeToolChoice{Type: "auto"}, nil
	case "none":
		return &dto.ClaudeToolChoice{Type: "none"}, nil
	case "required":
		return &dto.ClaudeToolChoice{Type: "any"}, nil
	}
	return nil, nil
}

// responsesInputToClaudeMessages converts the Responses API `input` field
// (string or array) into Anthropic messages.
//
// Mapping rules (per §4.1):
//   - string input → single user message with string content
//   - array items of role "user" / "assistant" → direct message (content converted)
//   - items of type "function_call"        → assistant message with tool_use block
//   - items of type "function_call_output" → user message with tool_result block
//   - consecutive function_call items are merged into one assistant message
//   - consecutive function_call_output items are merged into one user message
func responsesInputToClaudeMessages(inputRaw json.RawMessage) ([]dto.ClaudeMessage, error) {
	if len(inputRaw) == 0 {
		return nil, nil
	}

	// String input
	if common.GetJsonType(inputRaw) == "string" {
		var text string
		_ = json.Unmarshal(inputRaw, &text)
		return []dto.ClaudeMessage{
			{Role: "user", Content: text},
		}, nil
	}

	// Array input
	var rawItems []json.RawMessage
	if err := json.Unmarshal(inputRaw, &rawItems); err != nil {
		return nil, fmt.Errorf("parse input array: %w", err)
	}

	messages := make([]dto.ClaudeMessage, 0, len(rawItems))

	for _, rawItem := range rawItems {
		var item map[string]any
		if err := json.Unmarshal(rawItem, &item); err != nil {
			continue
		}

		itemType, _ := item["type"].(string)

		switch itemType {
		case "function_call":
			// → assistant message with tool_use block
			callID, _ := item["call_id"].(string)
			name, _ := item["name"].(string)
			argumentsStr, _ := item["arguments"].(string)

			inputObj := make(map[string]any)
			// HIGH 8: empty arguments → empty map, not skipped
			if argumentsStr != "" && argumentsStr != "{}" {
				if err := json.Unmarshal([]byte(argumentsStr), &inputObj); err != nil {
					common.SysLog(fmt.Sprintf("responses2claude: function_call arguments unmarshal failed for '%s': %v", name, err))
					inputObj = make(map[string]any)
				}
			}

			toolUseBlock := dto.ClaudeMediaMessage{
				Type:  "tool_use",
				Id:    callID,
				Name:  name,
				Input: inputObj,
			}

			// Merge consecutive function_call into last assistant message
			if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" {
				last := &messages[len(messages)-1]
				if blocks, ok := last.Content.([]dto.ClaudeMediaMessage); ok {
					last.Content = append(blocks, toolUseBlock)
				} else {
					last.Content = []dto.ClaudeMediaMessage{toolUseBlock}
				}
			} else {
				messages = append(messages, dto.ClaudeMessage{
					Role:    "assistant",
					Content: []dto.ClaudeMediaMessage{toolUseBlock},
				})
			}

		case "function_call_output":
			// → user message with tool_result block
			callID, _ := item["call_id"].(string)
			output, _ := item["output"].(string)

			toolResultBlock := dto.ClaudeMediaMessage{
				Type:      "tool_result",
				ToolUseId: callID,
				Content:   output,
			}

			// Merge consecutive function_call_output into last user message
			if len(messages) > 0 && messages[len(messages)-1].Role == "user" {
				last := &messages[len(messages)-1]
				switch c := last.Content.(type) {
				case []dto.ClaudeMediaMessage:
					last.Content = append(c, toolResultBlock)
				case string:
					last.Content = []dto.ClaudeMediaMessage{
						{Type: "text", Text: common.GetPointer(c)},
						toolResultBlock,
					}
				default:
					last.Content = []dto.ClaudeMediaMessage{toolResultBlock}
				}
			} else {
				messages = append(messages, dto.ClaudeMessage{
					Role:    "user",
					Content: []dto.ClaudeMediaMessage{toolResultBlock},
				})
			}

		case "message":
			// Standard message with role + content
			role, _ := item["role"].(string)
			if role == "" {
				role = "user"
			}
			contentRaw, _ := json.Marshal(item["content"])
			claudeContent, err := responsesContentToClaudeContent(contentRaw)
			if err != nil {
				return nil, err
			}
			messages = append(messages, dto.ClaudeMessage{
				Role:    role,
				Content: claudeContent,
			})

		default:
			// Items with role field (plain message items without explicit type)
			role, _ := item["role"].(string)
			if role != "" {
				contentRaw, _ := json.Marshal(item["content"])
				claudeContent, err := responsesContentToClaudeContent(contentRaw)
				if err != nil {
					return nil, err
				}
				messages = append(messages, dto.ClaudeMessage{
					Role:    role,
					Content: claudeContent,
				})
			}
		}
	}

	return messages, nil
}

// responsesContentToClaudeContent converts a Responses content field (string
// or array of content parts) to the Claude message content format.
func responsesContentToClaudeContent(contentRaw json.RawMessage) (any, error) {
	if len(contentRaw) == 0 || string(contentRaw) == "null" {
		return "...", nil
	}

	// String content
	if common.GetJsonType(contentRaw) == "string" {
		var text string
		_ = json.Unmarshal(contentRaw, &text)
		return text, nil
	}

	// Array content
	var parts []map[string]any
	if err := json.Unmarshal(contentRaw, &parts); err != nil {
		return nil, fmt.Errorf("parse content array: %w", err)
	}

	blocks := make([]dto.ClaudeMediaMessage, 0, len(parts))
	for _, part := range parts {
		partType, _ := part["type"].(string)
		switch partType {
		case "input_text", "text", "output_text":
			text, _ := part["text"].(string)
			blocks = append(blocks, dto.ClaudeMediaMessage{
				Type: "text",
				Text: common.GetPointer(text),
			})
		case "input_image", "image_url":
			// HIGH 4: image source handling
			block, err := responsesImagePartToClaudeBlock(part)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, block)
		case "tool_result":
			// Inline tool result inside content
			toolUseID, _ := part["tool_use_id"].(string)
			output, _ := part["content"].(string)
			blocks = append(blocks, dto.ClaudeMediaMessage{
				Type:      "tool_result",
				ToolUseId: toolUseID,
				Content:   output,
			})
		default:
			// Unknown part type - log and skip
			common.SysLog(fmt.Sprintf("responses2claude: unknown content part type '%s', skipping", partType))
		}
	}

	if len(blocks) == 0 {
		return "...", nil
	}
	if len(blocks) == 1 {
		if blocks[0].Type == "text" && blocks[0].Text != nil {
			return *blocks[0].Text, nil
		}
	}
	return blocks, nil
}

// responsesImagePartToClaudeBlock converts an image content part.
// HIGH 4: URL images pass as url type; data URIs become base64 blocks.
func responsesImagePartToClaudeBlock(part map[string]any) (dto.ClaudeMediaMessage, error) {
	block := dto.ClaudeMediaMessage{Type: "image"}

	// Support both nested image_url object and flat url field
	var url string
	if imgObj, ok := part["image_url"].(map[string]any); ok {
		url, _ = imgObj["url"].(string)
	} else {
		url, _ = part["url"].(string)
	}

	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		block.Source = &dto.ClaudeMessageSource{
			Type: "url",
			Url:  url,
		}
	} else if strings.HasPrefix(url, "data:") {
		// data:<mediaType>;base64,<data>
		semicolon := strings.Index(url, ";")
		comma := strings.Index(url, ",")
		if semicolon > 5 && comma > semicolon {
			mediaType := url[5:semicolon]
			data := url[comma+1:]
			block.Source = &dto.ClaudeMessageSource{
				Type:      "base64",
				MediaType: mediaType,
				Data:      data,
			}
		} else {
			return block, fmt.Errorf("invalid data URI for image")
		}
	} else if url != "" {
		// Treat unknown scheme as url
		block.Source = &dto.ClaudeMessageSource{
			Type: "url",
			Url:  url,
		}
	}

	return block, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Non-streaming: ClaudeResponse2Responses converts a complete Anthropic
// response to the OpenAI Responses API format.
// ─────────────────────────────────────────────────────────────────────────────

func ClaudeResponse2Responses(resp *dto.ClaudeResponse, info *ClaudeResponseInfo) (*dto.OpenAIResponsesResponse, error) {
	status := claudeStopReasonToResponsesStatus(resp.StopReason)

	result := &dto.OpenAIResponsesResponse{
		ID:     resp.Id,
		Object: "response",
		Status: status,
		Model:  resp.Model,
		Output: make([]dto.ResponsesOutput, 0),
	}

	if info != nil {
		result.CreatedAt = int(info.Created)
	}

	// Incomplete details
	if status == "incomplete" {
		result.IncompleteDetails = &dto.IncompleteDetails{
			Reasoning: reasonmap.ClaudeStopReasonToOpenAIFinishReason(resp.StopReason),
		}
	}

	// Convert content blocks to output items
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			text := block.GetText()
			item := dto.ResponsesOutput{
				Type:   "message",
				ID:     "msg_" + common.GetUUID(),
				Status: "completed",
				Role:   "assistant",
				Content: []dto.ResponsesOutputContent{
					{
						Type:        "output_text",
						Text:        text,
						Annotations: []interface{}{},
					},
				},
			}
			result.Output = append(result.Output, item)

		case "tool_use":
			if block.Input == nil {
				block.Input = map[string]any{}
			}
			argsBytes, _ := json.Marshal(block.Input)
			item := dto.ResponsesOutput{
				Type:      "function_call",
				ID:        "fc_" + common.GetUUID(),
				Status:    "completed",
				CallId:    block.Id,
				Name:      block.Name,
				Arguments: string(argsBytes),
			}
			result.Output = append(result.Output, item)

		case "thinking":
			thinkingText := ""
			if block.Thinking != nil {
				thinkingText = *block.Thinking
			}
			// Append signature if present
			if block.Signature != "" {
				thinkingText += "\n\n[SIGNATURE:" + block.Signature + "]"
			}
			item := dto.ResponsesOutput{
				Type:   "reasoning",
				ID:     "rs_" + common.GetUUID(),
				Status: "completed",
				Content: []dto.ResponsesOutputContent{
					{
						Type: "reasoning_summary",
						Text: thinkingText,
					},
				},
			}
			result.Output = append(result.Output, item)
		}
	}

	// Usage
	if resp.Usage != nil {
		result.Usage = buildResponsesUsage(resp.Usage)
	}

	return result, nil
}

// buildResponsesUsage converts ClaudeUsage to ResponsesUsage.
func buildResponsesUsage(u *dto.ClaudeUsage) *dto.ResponsesUsage {
	if u == nil {
		return nil
	}
	ru := &dto.ResponsesUsage{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		TotalTokens:  u.InputTokens + u.OutputTokens,
	}
	if u.CacheReadInputTokens > 0 {
		ru.InputTokensDetails = &dto.InputTokenDetails{
			CachedTokens: u.CacheReadInputTokens,
		}
	}
	if u.ThinkingOutputTokens > 0 {
		ru.OutputTokensDetails = &dto.ResponsesOutputTokensDetails{
			ReasoningTokens: u.ThinkingOutputTokens,
		}
	}
	return ru
}

// claudeStopReasonToResponsesStatus maps Anthropic stop_reason to Responses status.
func claudeStopReasonToResponsesStatus(reason string) string {
	switch reason {
	case "end_turn", "stop_sequence", "tool_use":
		return "completed"
	case "max_tokens", "pause_turn":
		return "incomplete"
	default:
		if reason == "" {
			return "completed"
		}
		return "completed"
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Streaming: StreamClaude2Responses translates a single Anthropic SSE event
// into zero or more Responses protocol SSE events.
// ─────────────────────────────────────────────────────────────────────────────

func StreamClaude2Responses(c *gin.Context, claudeInfo *ClaudeResponseInfo, ev *dto.ClaudeResponse) *types.NewAPIError {
	if claudeInfo == nil {
		return nil
	}

	// Ensure StreamState is initialized
	if claudeInfo.StreamState == nil {
		claudeInfo.StreamState = &StreamState{}
	}
	ss := claudeInfo.StreamState

	switch ev.Type {
	case "message_start":
		// Update claudeInfo from message_start
		if ev.Message != nil {
			claudeInfo.ResponseId = ev.Message.Id
			claudeInfo.Model = ev.Message.Model
			if ev.Message.Usage != nil {
				if claudeInfo.Usage == nil {
					claudeInfo.Usage = &dto.Usage{}
				}
				claudeInfo.Usage.PromptTokens = ev.Message.Usage.InputTokens
			}
		}

		// Emit response.created with skeleton response
		responseSnapshot := &dto.OpenAIResponsesResponse{
			ID:        claudeInfo.ResponseId,
			Object:    "response",
			CreatedAt: int(claudeInfo.Created),
			Model:     claudeInfo.Model,
			Status:    "in_progress",
			Output:    []dto.ResponsesOutput{},
		}
		created := &dto.ResponsesStreamResponse{
			Type:           "response.created",
			SequenceNumber: ss.nextSeq(),
			Response:       responseSnapshot,
		}
		if err := helper.ObjectData(c, created); err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}

		// Emit response.in_progress (§4.3 requires both events on message_start)
		inProgress := &dto.ResponsesStreamResponse{
			Type:           "response.in_progress",
			SequenceNumber: ss.nextSeq(),
			Response:       responseSnapshot,
		}
		if err := helper.ObjectData(c, inProgress); err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}

	case "content_block_start":
		if ev.ContentBlock == nil {
			return nil
		}
		blockType := ev.ContentBlock.Type

		switch blockType {
		case "text":
			ss.CurItemID = "msg_" + common.GetUUID()
			ss.ActiveBlockType = "text"
			ss.AccText.Reset()

			// response.output_item.added (type=message)
			itemAdded := &dto.ResponsesStreamResponse{
				Type:           dto.ResponsesOutputTypeItemAdded,
				SequenceNumber: ss.nextSeq(),
				OutputIndex:    intPtr(ss.CurOutputIdx),
				Item: &dto.ResponsesOutput{
					ID:      ss.CurItemID,
					Type:    "message",
					Status:  "in_progress",
					Role:    "assistant",
					Content: []dto.ResponsesOutputContent{},
				},
			}
			if err := helper.ObjectData(c, itemAdded); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}

			// response.content_part.added (type=output_text)
			contentAdded := &dto.ResponsesStreamResponse{
				Type:           "response.content_part.added",
				SequenceNumber: ss.nextSeq(),
				ItemID:         ss.CurItemID,
				OutputIndex:    intPtr(ss.CurOutputIdx),
				ContentIndex:   intPtr(ss.TextContentIdx),
				Part: &dto.ResponsesReasoningSummaryPart{
					Type:        "output_text",
					Text:        "",
					Annotations: []any{},
				},
			}
			if err := helper.ObjectData(c, contentAdded); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}

		case "tool_use":
			ss.CurItemID = "fc_" + common.GetUUID()
			ss.ActiveBlockType = "tool_use"
			ss.AccArguments.Reset()
			ss.ToolCallCounter++
			ss.CurToolCallId = ev.ContentBlock.Id
			ss.CurToolName = ev.ContentBlock.Name

			// response.output_item.added (type=function_call)
			itemAdded := &dto.ResponsesStreamResponse{
				Type:           dto.ResponsesOutputTypeItemAdded,
				SequenceNumber: ss.nextSeq(),
				OutputIndex:    intPtr(ss.CurOutputIdx),
				Item: &dto.ResponsesOutput{
					ID:        ss.CurItemID,
					Type:      "function_call",
					Status:    "in_progress",
					CallId:    ev.ContentBlock.Id,
					Name:      ev.ContentBlock.Name,
					Arguments: "",
				},
			}
			if err := helper.ObjectData(c, itemAdded); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}

		case "thinking":
			ss.CurItemID = "rs_" + common.GetUUID()
			ss.ActiveBlockType = "thinking"
			ss.AccThinking.Reset()

			// response.output_item.added (type=reasoning)
			itemAdded := &dto.ResponsesStreamResponse{
				Type:           dto.ResponsesOutputTypeItemAdded,
				SequenceNumber: ss.nextSeq(),
				OutputIndex:    intPtr(ss.CurOutputIdx),
				Item: &dto.ResponsesOutput{
					ID:      ss.CurItemID,
					Type:    "reasoning",
					Status:  "in_progress",
					Content: []dto.ResponsesOutputContent{},
				},
			}
			if err := helper.ObjectData(c, itemAdded); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}

			// response.reasoning_summary_part.added
			summaryAdded := &dto.ResponsesStreamResponse{
				Type:           "response.reasoning_summary_part.added",
				SequenceNumber: ss.nextSeq(),
				ItemID:         ss.CurItemID,
				OutputIndex:    intPtr(ss.CurOutputIdx),
				SummaryIndex:   intPtr(0),
				Part: &dto.ResponsesReasoningSummaryPart{
					Type:        "reasoning_summary",
					Text:        "",
					Annotations: []any{},
				},
			}
			if err := helper.ObjectData(c, summaryAdded); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}
		}

	case "content_block_delta":
		if ev.Delta == nil {
			return nil
		}
		switch ev.Delta.Type {
		case "text_delta":
			text := ""
			if ev.Delta.Text != nil {
				text = *ev.Delta.Text
				ss.AccText.WriteString(text)
				claudeInfo.ResponseText.WriteString(text)
			}
			deltaEv := &dto.ResponsesStreamResponse{
				Type:           "response.output_text.delta",
				SequenceNumber: ss.nextSeq(),
				ItemID:         ss.CurItemID,
				OutputIndex:    intPtr(ss.CurOutputIdx),
				ContentIndex:   intPtr(ss.TextContentIdx),
				Delta:          text,
			}
			if err := helper.ObjectData(c, deltaEv); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}

		case "input_json_delta":
			partial := ""
			if ev.Delta.PartialJson != nil {
				partial = *ev.Delta.PartialJson
				ss.AccArguments.WriteString(partial)
			}
			deltaEv := &dto.ResponsesStreamResponse{
				Type:           "response.function_call_arguments.delta",
				SequenceNumber: ss.nextSeq(),
				ItemID:         ss.CurItemID,
				OutputIndex:    intPtr(ss.CurOutputIdx),
				Delta:          partial,
			}
			if err := helper.ObjectData(c, deltaEv); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}

		case "thinking_delta":
			thinking := ""
			if ev.Delta.Thinking != nil {
				thinking = *ev.Delta.Thinking
				ss.AccThinking.WriteString(thinking)
			}
			deltaEv := &dto.ResponsesStreamResponse{
				Type:           "response.reasoning_summary_text.delta",
				SequenceNumber: ss.nextSeq(),
				ItemID:         ss.CurItemID,
				OutputIndex:    intPtr(ss.CurOutputIdx),
				SummaryIndex:   intPtr(0),
				Delta:          thinking,
			}
			if err := helper.ObjectData(c, deltaEv); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}

		case "signature_delta":
			// Not emitted as SSE; accumulate in claudeInfo for potential multi-turn use
			if ev.Delta.Text != nil {
				claudeInfo.ThinkingSignature += *ev.Delta.Text
			}
		}

	case "content_block_stop":
		switch ss.ActiveBlockType {
		case "text":
			fullText := ss.AccText.String()
			// response.output_text.done
			doneEv := &dto.ResponsesStreamResponse{
				Type:           "response.output_text.done",
				SequenceNumber: ss.nextSeq(),
				ItemID:         ss.CurItemID,
				OutputIndex:    intPtr(ss.CurOutputIdx),
				ContentIndex:   intPtr(ss.TextContentIdx),
				Delta:          fullText,
			}
			if err := helper.ObjectData(c, doneEv); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			// response.content_part.done
			partDone := &dto.ResponsesStreamResponse{
				Type:           "response.content_part.done",
				SequenceNumber: ss.nextSeq(),
				ItemID:         ss.CurItemID,
				OutputIndex:    intPtr(ss.CurOutputIdx),
				ContentIndex:   intPtr(ss.TextContentIdx),
				Part: &dto.ResponsesReasoningSummaryPart{
					Type:        "output_text",
					Text:        fullText,
					Annotations: []any{},
				},
			}
			if err := helper.ObjectData(c, partDone); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			completedTextItem := dto.ResponsesOutput{
				ID:     ss.CurItemID,
				Type:   "message",
				Status: "completed",
				Role:   "assistant",
				Content: []dto.ResponsesOutputContent{
					{Type: "output_text", Text: fullText, Annotations: []interface{}{}},
				},
			}
			// response.output_item.done (message)
			itemDone := &dto.ResponsesStreamResponse{
				Type:           dto.ResponsesOutputTypeItemDone,
				SequenceNumber: ss.nextSeq(),
				OutputIndex:    intPtr(ss.CurOutputIdx),
				Item:           &completedTextItem,
			}
			if err := helper.ObjectData(c, itemDone); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			ss.CompletedOutputItems = append(ss.CompletedOutputItems, completedTextItem)
			ss.CurOutputIdx++

		case "tool_use":
			fullArgs := ss.AccArguments.String()
			// response.function_call_arguments.done
			argsDone := &dto.ResponsesStreamResponse{
				Type:           "response.function_call_arguments.done",
				SequenceNumber: ss.nextSeq(),
				ItemID:         ss.CurItemID,
				OutputIndex:    intPtr(ss.CurOutputIdx),
				Delta:          fullArgs,
			}
			if err := helper.ObjectData(c, argsDone); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			completedToolItem := dto.ResponsesOutput{
				ID:        ss.CurItemID,
				Type:      "function_call",
				Status:    "completed",
				CallId:    ss.CurToolCallId,
				Name:      ss.CurToolName,
				Arguments: fullArgs,
			}
			// response.output_item.done (function_call)
			itemDone := &dto.ResponsesStreamResponse{
				Type:           dto.ResponsesOutputTypeItemDone,
				SequenceNumber: ss.nextSeq(),
				OutputIndex:    intPtr(ss.CurOutputIdx),
				Item:           &completedToolItem,
			}
			if err := helper.ObjectData(c, itemDone); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			ss.CompletedOutputItems = append(ss.CompletedOutputItems, completedToolItem)
			ss.CurOutputIdx++

		case "thinking":
			fullThinking := ss.AccThinking.String()
			// Append signature if present
			if claudeInfo.ThinkingSignature != "" {
				fullThinking += "\n\n[SIGNATURE:" + claudeInfo.ThinkingSignature + "]"
			}
			// response.reasoning_summary_text.done
			textDone := &dto.ResponsesStreamResponse{
				Type:           "response.reasoning_summary_text.done",
				SequenceNumber: ss.nextSeq(),
				ItemID:         ss.CurItemID,
				OutputIndex:    intPtr(ss.CurOutputIdx),
				SummaryIndex:   intPtr(0),
				Delta:          fullThinking,
			}
			if err := helper.ObjectData(c, textDone); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			// response.reasoning_summary_part.done
			partDone := &dto.ResponsesStreamResponse{
				Type:           "response.reasoning_summary_part.done",
				SequenceNumber: ss.nextSeq(),
				ItemID:         ss.CurItemID,
				OutputIndex:    intPtr(ss.CurOutputIdx),
				SummaryIndex:   intPtr(0),
				Part: &dto.ResponsesReasoningSummaryPart{
					Type:        "reasoning_summary",
					Text:        fullThinking,
					Annotations: []any{},
				},
			}
			if err := helper.ObjectData(c, partDone); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			completedReasoningItem := dto.ResponsesOutput{
				ID:     ss.CurItemID,
				Type:   "reasoning",
				Status: "completed",
				Content: []dto.ResponsesOutputContent{
					{Type: "reasoning_summary", Text: fullThinking, Annotations: []interface{}{}},
				},
			}
			// response.output_item.done (reasoning)
			itemDone := &dto.ResponsesStreamResponse{
				Type:           dto.ResponsesOutputTypeItemDone,
				SequenceNumber: ss.nextSeq(),
				OutputIndex:    intPtr(ss.CurOutputIdx),
				Item:           &completedReasoningItem,
			}
			if err := helper.ObjectData(c, itemDone); err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			ss.CompletedOutputItems = append(ss.CompletedOutputItems, completedReasoningItem)
			ss.CurOutputIdx++
		}
		ss.ActiveBlockType = ""

	case "message_delta":
		// Usage update — FormatClaudeResponseInfo already handles this; we only
		// track state here.
		if ev.Usage != nil && claudeInfo.Usage != nil {
			if ev.Usage.OutputTokens > 0 {
				claudeInfo.Usage.CompletionTokens = ev.Usage.OutputTokens + ev.Usage.ThinkingOutputTokens
				if ev.Usage.ThinkingOutputTokens > 0 {
					claudeInfo.Usage.CompletionTokenDetails.ReasoningTokens = ev.Usage.ThinkingOutputTokens
				}
			}
			claudeInfo.Usage.TotalTokens = claudeInfo.Usage.PromptTokens + claudeInfo.Usage.CompletionTokens
		}
		claudeInfo.Done = true

	case "message_stop":
		// Nothing to emit here; buildResponsesCompletedEvent is called by
		// HandleStreamFinalResponse after this function returns.
	}

	return nil
}

// buildResponsesCompletedEvent constructs the final response.completed SSE event.
func buildResponsesCompletedEvent(info *ClaudeResponseInfo) *dto.ResponsesStreamResponse {
	if info == nil {
		return &dto.ResponsesStreamResponse{Type: "response.completed"}
	}

	var ss *StreamState
	if info.StreamState != nil {
		ss = info.StreamState
	} else {
		ss = &StreamState{}
	}

	var usage *dto.ResponsesUsage
	if info.Usage != nil {
		usage = &dto.ResponsesUsage{
			InputTokens:  info.Usage.PromptTokens,
			OutputTokens: info.Usage.CompletionTokens,
			TotalTokens:  info.Usage.TotalTokens,
		}
		if info.Usage.PromptTokensDetails.CachedTokens > 0 {
			usage.InputTokensDetails = &dto.InputTokenDetails{
				CachedTokens: info.Usage.PromptTokensDetails.CachedTokens,
			}
		}
		if info.Usage.CompletionTokenDetails.ReasoningTokens > 0 {
			usage.OutputTokensDetails = &dto.ResponsesOutputTokensDetails{
				ReasoningTokens: info.Usage.CompletionTokenDetails.ReasoningTokens,
			}
		}
	}

	seqNo := ss.nextSeq()

	output := ss.CompletedOutputItems
	if output == nil {
		output = []dto.ResponsesOutput{}
	}

	return &dto.ResponsesStreamResponse{
		Type:           "response.completed",
		SequenceNumber: seqNo,
		Response: &dto.OpenAIResponsesResponse{
			ID:        info.ResponseId,
			Object:    "response",
			CreatedAt: int(info.Created),
			Model:     info.Model,
			Status:    "completed",
			Output:    output,
			Usage:     usage,
		},
	}
}
