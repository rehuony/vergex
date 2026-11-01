package provider

import (
	"encoding/json"
	"testing"

	"vergex/internal/agent"
)

// Traders call models through the Request path to get the model's own thinking back.
// That must not change what the model is sent.
func TestARequestSendsWhatThePlainCallSent(t *testing.T) {
	for _, name := range []string{"deepseek", "openai", "claude", "qwen", "gemini", "grok", "kimi", "minimax"} {
		t.Run(name, func(t *testing.T) {
			client := agent.NewClientByProvider(name)
			if client == nil {
				t.Fatal("provider is not registered")
			}
			client.SetAPIKey("key", "", "")
			base := client.(agent.ClientEmbedder).Base()

			plain, err := json.Marshal(base.Hooks.BuildRequestBody("system prompt", "user prompt"))
			if err != nil {
				t.Fatal(err)
			}
			request, err := json.Marshal(base.Hooks.BuildRequestBodyFromRequest(&agent.Request{
				Model:    base.Model,
				Messages: []agent.Message{agent.NewSystemMessage("system prompt"), agent.NewUserMessage("user prompt")},
			}))
			if err != nil {
				t.Fatal(err)
			}
			if string(plain) != string(request) {
				t.Fatalf("the request changed:\nplain   %s\nrequest %s", plain, request)
			}
		})
	}
}
