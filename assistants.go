package serpkite

import (
	"context"
	"net/http"
	"time"
)

// ── AI assistants ────────────────────────────────────────────────────────

// assistantMinTimeout is the least per-attempt timeout of an AI assistant
// call: answers take 10-90 s.
const assistantMinTimeout = 120 * time.Second

// isAssistant reports whether endpoint is an AI assistant endpoint.
func isAssistant(e Endpoint) bool {
	switch e {
	case EndpointChatGPT, EndpointGemini, EndpointCopilot, EndpointAIMode, EndpointBraveAI:
		return true
	}
	return false
}

// ChatGPT asks ChatGPT as a logged-out user and returns its answer (Text,
// Markdown), cited Sources, SearchQueries, Products, Places and the guest
// Model. WebSearch (default true) makes it search the web; Country asks
// from an exit in that country; Track adds Mentions. 4 credits.
//
// The AI assistant methods are billed even when the response is lost, so
// they are retried only on 429 (failed, refused and empty answers are free),
// and their per-attempt timeout is at least 120 s: answers take 10-90 s.
// With Format "markdown" only Markdown (and Request.Prompt) is filled in.
func (c *Client) ChatGPT(ctx context.Context, p ChatGPTParams, opts ...RequestOption) (*AssistantResponse, error) {
	return c.assistant(ctx, EndpointChatGPT, p.Prompt, p, opts)
}

// Gemini asks Google Gemini as a logged-out user: the answer with the
// sources it grounded on (grounded answers take 40-90 s). 3 credits. See
// [Client.ChatGPT] for retries, timeouts and Markdown.
func (c *Client) Gemini(ctx context.Context, p AssistantParams, opts ...RequestOption) (*AssistantResponse, error) {
	return c.assistant(ctx, EndpointGemini, p.Prompt, p, opts)
}

// Copilot asks Microsoft Copilot Search on Bing: the answer, cited sources
// and Bing's sponsored product cards. Country and Language set Bing's
// market. 3 credits. See [Client.ChatGPT] for retries, timeouts and Markdown.
func (c *Client) Copilot(ctx context.Context, p AssistantParams, opts ...RequestOption) (*AssistantResponse, error) {
	return c.assistant(ctx, EndpointCopilot, p.Prompt, p, opts)
}

// AIMode asks Google AI Mode: the answer, cited sources and products.
// Country is the country Google answers for. 2 credits. See [Client.ChatGPT]
// for retries, timeouts and Markdown.
func (c *Client) AIMode(ctx context.Context, p AssistantParams, opts ...RequestOption) (*AssistantResponse, error) {
	return c.assistant(ctx, EndpointAIMode, p.Prompt, p, opts)
}

// BraveAI asks Brave Search's AI answer: cited sources, the searches it ran
// and products. Country sets Brave's market. 2 credits. See [Client.ChatGPT]
// for retries, timeouts and Markdown.
func (c *Client) BraveAI(ctx context.Context, p AssistantParams, opts ...RequestOption) (*AssistantResponse, error) {
	return c.assistant(ctx, EndpointBraveAI, p.Prompt, p, opts)
}

func (c *Client) assistant(ctx context.Context, e Endpoint, prompt string, body any, opts []RequestOption) (*AssistantResponse, error) {
	var out AssistantResponse
	text, isJSON, err := c.do(ctx, assistantCall("/v1/"+string(e), body, opts, &out))
	if err != nil {
		return nil, err
	}
	if !isJSON {
		return &AssistantResponse{Request: AssistantEcho{Endpoint: string(e), Prompt: prompt}, Markdown: text}, nil
	}
	return &out, nil
}

// assistantCall retries only on 429 and waits at least assistantMinTimeout.
func assistantCall(path string, body any, opts []RequestOption, out any) call {
	return call{method: http.MethodPost, path: path, body: body, opts: opts, out: out, rateLimitOnly: true, minTimeout: assistantMinTimeout}
}
