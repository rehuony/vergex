package agent

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// FailureKind says why a call to a model failed, in terms of what to do about it.
type FailureKind string

const (
	FailureAuth          FailureKind = "auth"
	FailureBalance       FailureKind = "balance"
	FailureRateLimit     FailureKind = "rate_limit"
	FailureModelNotFound FailureKind = "model_not_found"
	FailureContextLength FailureKind = "context_length"
	FailureOverloaded    FailureKind = "overloaded"
	FailureTimeout       FailureKind = "timeout"
	FailureNetwork       FailureKind = "network"
	FailureOther         FailureKind = "other"
)

var failureWords = map[FailureKind]struct{ en, zh string }{
	FailureAuth:          {"The provider refused the API key. Check it in Settings → Agent models", "服务商拒绝了 API Key。请在 设置 → Agent 模型 检查"},
	FailureBalance:       {"The provider says this account is out of balance or quota", "服务商说这个账户的余额或额度不足"},
	FailureRateLimit:     {"The provider is limiting requests on this key. Try again in a moment", "服务商正在限制这个 Key 的请求。请稍后再试"},
	FailureModelNotFound: {"The provider does not offer this model. Check its name in Settings → Agent models", "服务商没有这个模型。请在 设置 → Agent 模型 检查名称"},
	FailureContextLength: {"This is too long for the model. Start a new conversation, or choose a model with more room", "内容超过了模型的上限。请开始新对话，或换一个容量更大的模型"},
	FailureOverloaded:    {"The provider is overloaded or having trouble. Try again in a moment", "服务商负载过高或出了问题。请稍后再试"},
	FailureTimeout:       {"The model took too long to answer", "模型回答超时"},
	FailureNetwork:       {"The provider could not be reached. Check the network and the base URL", "连不上服务商。请检查网络和 Base URL"},
	FailureOther:         {"The model request failed", "模型请求失败"},
}

// Failure is a failed call as it can be shown: why, and the provider's own line about it,
// which never holds the API key.
type Failure struct {
	Kind   FailureKind
	Detail string
}

func (f *Failure) Error() string {
	return joinDetail(failureWords[f.Kind].en, f.Detail)
}

// Key is what the web interface words it by.
func (f *Failure) Key() string { return "agentError." + string(f.Kind) }

// Describe is the failure in words, for what Go writes to the user itself.
func (f *Failure) Describe(zh bool) string {
	if zh {
		return joinDetail(failureWords[f.Kind].zh, f.Detail)
	}
	return f.Error()
}

func joinDetail(words, detail string) string {
	if detail == "" {
		return words
	}
	return words + " (" + detail + ")"
}

// Classify says why a call failed. A client's own key is taken out of what it reports.
func Classify(client Client, err error) *Failure {
	apiKey := ""
	if embedder, ok := client.(ClientEmbedder); ok {
		apiKey = embedder.Base().APIKey
	}
	failure := &Failure{Kind: failureKind(err), Detail: ProviderMessage(err, apiKey)}
	// With no answer from a provider, there is no line of its own worth repeating.
	var refused *StatusError
	if !errors.As(err, &refused) {
		failure.Detail = ""
	}
	return failure
}

func failureKind(err error) FailureKind {
	var refused *StatusError
	switch {
	case errors.As(err, &refused):
		return statusFailure(refused)
	case errors.Is(err, context.DeadlineExceeded):
		return FailureTimeout
	case errors.Is(err, context.Canceled):
		return FailureOther
	}
	return FailureNetwork
}

var (
	modelMissing   = regexp.MustCompile(`model.*(not exist|not found|not support|unknown|invalid|passed)|(unknown|invalid|unsupported|no such) model`)
	contextTooLong = regexp.MustCompile(`context.?length|maximum context|context window|too long|too many tokens|prompt is too`)
	outOfBalance   = regexp.MustCompile(`insufficient|balance|quota|billing|credit`)
	keyProblem     = regexp.MustCompile(`api.?key|unauthorized|authentication`)
)

func statusFailure(refused *StatusError) FailureKind {
	body := strings.ToLower(refused.Body)
	switch {
	case refused.Status == 401, refused.Status == 403:
		return FailureAuth
	case refused.Status == 402:
		return FailureBalance
	case refused.Status == 404:
		return FailureModelNotFound
	case refused.Status == 413:
		return FailureContextLength
	case refused.Status == 429:
		if outOfBalance.MatchString(body) {
			return FailureBalance
		}
		return FailureRateLimit
	case refused.Status >= 500:
		return FailureOverloaded
	case outOfBalance.MatchString(body):
		return FailureBalance
	case modelMissing.MatchString(body):
		return FailureModelNotFound
	case contextTooLong.MatchString(body):
		return FailureContextLength
	case keyProblem.MatchString(body):
		return FailureAuth
	}
	return FailureOther
}

var (
	retriedPrefix = regexp.MustCompile(`^still failed after \d+ retries: `)
	statusPrefix  = regexp.MustCompile(`^API returned error \(status (\d+)\): `)
)

// ProviderMessage turns a failed call into one short line: the provider's own message where
// its answer is JSON, never the API key.
func ProviderMessage(err error, apiKey string) string {
	msg := retriedPrefix.ReplaceAllString(err.Error(), "")
	if i := strings.Index(msg, "{"); i >= 0 {
		var body struct {
			Error   json.RawMessage `json:"error"`
			Message string          `json:"message"`
		}
		if json.Unmarshal([]byte(msg[i:]), &body) == nil {
			var nested struct {
				Message string `json:"message"`
			}
			var plain string
			switch {
			case json.Unmarshal(body.Error, &nested) == nil && nested.Message != "":
				msg = msg[:i] + nested.Message
			case json.Unmarshal(body.Error, &plain) == nil && plain != "":
				msg = msg[:i] + plain
			case body.Message != "":
				msg = msg[:i] + body.Message
			}
		}
	}
	msg = statusPrefix.ReplaceAllString(msg, "HTTP $1: ")
	if apiKey != "" {
		msg = strings.ReplaceAll(msg, apiKey, "***")
	}
	msg = strings.Join(strings.Fields(msg), " ")
	if runes := []rune(msg); len(runes) > 200 {
		msg = string(runes[:200]) + "…"
	}
	return msg
}
