package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"vergex/internal/llm"
)

func textToolMessages(messages []llm.Message, tools []llm.Tool) []llm.Message {
	definitions, _ := json.Marshal(tools)
	guide := `Tool transport for this text-only model: respond with exactly one JSON object, without markdown or text outside it. Format: {"content":"brief explanation or final answer","tool_calls":[{"name":"registered_tool_name","arguments":{}}]}. Use up to five calls per response, or an empty array for a final answer. Arguments must be JSON objects matching the tool schema. To create draft plans you MUST request propose_plans; text descriptions alone do not save plans. Do not invent results. Tools execute in Go only after validation. No approval, order execution, payment or unrestricted host tool is available. Tool definitions: ` + string(definitions)
	out := make([]llm.Message, 0, len(messages)+1)
	for _, message := range messages {
		switch {
		case len(message.ToolCalls) > 0:
			calls := make([]map[string]any, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				calls = append(calls, map[string]any{"id": call.ID, "name": call.Function.Name, "arguments": json.RawMessage(call.Function.Arguments)})
			}
			raw, _ := json.Marshal(map[string]any{"content": message.Content, "tool_calls": calls})
			out = append(out, llm.NewAssistantMessage(string(raw)))
		case message.Role == "tool":
			raw, _ := json.Marshal(map[string]any{"tool_result_for": message.ToolCallID, "result": message.Content, "untrusted_data": true})
			out = append(out, llm.NewUserMessage(string(raw)))
		default:
			out = append(out, llm.Message{Role: message.Role, Content: message.Content})
		}
	}
	if len(out) > 0 && out[0].Role == "system" {
		out[0].Content += "\n\n" + guide
	} else {
		out = append([]llm.Message{llm.NewSystemMessage(guide)}, out...)
	}
	return out
}

func parseTextTools(text string, tools []Tool, turn uint64) (Message, error) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json\n") && strings.HasSuffix(text, "\n```") {
		text = strings.TrimSuffix(strings.TrimPrefix(text, "```json\n"), "\n```")
	}
	var response struct {
		Content string `json:"content"`
		Calls   []struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"tool_calls"`
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&response) != nil {
		return Message{}, errors.New("The model returned an invalid planning response; no tools were executed")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF || len(response.Calls) > 5 || (response.Content == "" && len(response.Calls) == 0) {
		return Message{}, errors.New("The model returned an incomplete or excessive planning response; no tools were executed")
	}
	allowed := map[string]bool{}
	for _, tool := range tools {
		allowed[tool.Function.Name] = true
	}
	out := Message{Role: "assistant", Content: response.Content}
	for index, call := range response.Calls {
		raw := bytes.TrimSpace(call.Arguments)
		var args map[string]json.RawMessage
		if !allowed[call.Name] || len(raw) > 24000 || len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &args) != nil || args == nil {
			return Message{}, errors.New("The model requested an unknown tool or invalid arguments; no tools were executed")
		}
		invocation := ToolCall{ID: fmt.Sprintf("text_%d_%d", turn, index), Type: "function"}
		invocation.Function.Name = call.Name
		invocation.Function.Arguments = string(raw)
		out.ToolCalls = append(out.ToolCalls, invocation)
	}
	return out, nil
}
