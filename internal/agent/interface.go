package agent

import (
	"time"
)

// ClientEmbedder is implemented by provider types that embed *BaseClient,
// allowing generic extraction of the underlying base client (e.g. for cloning).
type ClientEmbedder interface {
	Base() *BaseClient
}

// Client is how callers reach a model.
type Client interface {
	SetAPIKey(apiKey string, customURL string, customModel string)
	SetTimeout(timeout time.Duration)
	CallWithMessages(systemPrompt, userPrompt string) (string, error)
	CallWithRequest(req *Request) (string, error)
	// CallWithRequestFull returns both text content and tool calls.
	// Use this when the request includes Tools — the model may respond with
	// either a plain text reply (Response.Content) or tool invocations
	// (Response.ToolCalls), but not both.
	CallWithRequestFull(req *Request) (*Response, error)
}
