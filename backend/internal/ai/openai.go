package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ocm-backend/internal/logging"
)

// Hand-rolled OpenAI-compatible chat completions client. The surface the
// assistant needs is small (one streaming endpoint with function calling) and
// the project stays dependency-light; a lenient hand-rolled parser also
// tolerates the field-level quirks of OpenAI-compatible providers
// (DeepSeek/Qwen/GLM/...) that strict SDKs choke on.

// Upstream errors mapped to user-facing Chinese messages. The handler
// surfaces these verbatim; nothing about the upstream (body, URL, key) is
// ever echoed to the client.
var (
	ErrUpstreamAuth        = errors.New("AI 助手 API 密钥无效或已过期，请联系管理员")
	ErrUpstreamNotFound    = errors.New("AI 助手接口地址或模型不存在，请联系管理员核对配置")
	ErrUpstreamRate        = errors.New("AI 助手请求过于频繁，请稍后再试")
	ErrUpstreamUnavailable = errors.New("AI 助手服务暂时不可用，请稍后再试")
)

// Client calls one OpenAI-compatible chat completions endpoint.
type Client struct {
	baseURL string // no trailing slash
	apiKey  string
	model   string
	http    *http.Client // no Timeout: deadlines come from the caller's context
}

func NewClient(baseURL, apiKey, model string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{},
	}
}

// ToolDef is the OpenAI function-calling definition of one assistant tool,
// in the standard wrapped wire shape (type:"function" + function:{...}).
// DeepSeek and other strict deserializers reject the flat form.
type ToolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

// NewToolDef builds a standard function-call tool definition.
func NewToolDef(name, description string, params map[string]any) ToolDef {
	t := ToolDef{Type: "function"}
	t.Function.Name = name
	t.Function.Description = description
	t.Function.Parameters = params
	return t
}

// ChatMessage is one message in the upstream request. ToolCallID/ToolCalls
// are set for tool results and assistant tool-call turns respectively.
type ChatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []ChatToolCall `json:"tool_calls,omitempty"`
}

// ChatToolCall mirrors the OpenAI tool_call wire shape (id + function name +
// arguments-as-JSON-string; arguments arrive fragmented across deltas, and
// Index ties fragments of parallel tool calls together).
type ChatToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// streamChunk is one upstream SSE data payload. Only the fields the assistant
// consumes are declared; unknown fields are ignored by json.Unmarshal.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string         `json:"content"`
			ToolCalls []ChatToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Stream yields upstream SSE chunks. It also covers providers that answer a
// 2xx without streaming: the whole body is read once and returned as a
// single one-shot chunk.
type Stream struct {
	body io.ReadCloser
	r    *bufio.Reader

	fallback       *streamChunk // non-SSE 2xx response, served once
	fallbackServed bool
}

// Next returns the next event chunk. event=false with a nil error means an
// SSE keep-alive/comment line was received — no event, but the upstream is
// provably alive; callers use it to reset idle timers. io.EOF ends the
// stream (both [DONE] and the fallback path).
func (s *Stream) Next(ctx context.Context) (streamChunk, bool, error) {
	if s.fallback != nil {
		if s.fallbackServed {
			return streamChunk{}, false, io.EOF
		}
		s.fallbackServed = true
		return *s.fallback, true, nil
	}
	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return streamChunk{}, false, io.EOF
			}
			return streamChunk{}, false, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue // event separator, not traffic
		}
		if !strings.HasPrefix(line, "data:") {
			// SSE comment lines are keep-alive traffic: no event, but proof
			// the upstream is alive.
			return streamChunk{}, false, nil
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			return streamChunk{}, false, nil
		}
		if data == "[DONE]" {
			return streamChunk{}, false, io.EOF
		}
		var ch streamChunk
		if err := json.Unmarshal([]byte(data), &ch); err != nil {
			// Lenient: skip unparseable payloads rather than failing the turn.
			continue
		}
		return ch, true, nil
	}
}

func (s *Stream) Close() { _ = s.body.Close() }

// upstreamRequest is the wire shape sent to the provider. ToolChoice is
// omitted entirely when there are no tools.
type upstreamRequest struct {
	Model      string        `json:"model"`
	Messages   []ChatMessage `json:"messages"`
	Tools      []ToolDef     `json:"tools,omitempty"`
	ToolChoice string        `json:"tool_choice,omitempty"`
	Stream     bool          `json:"stream"`
}

// StreamRequest opens a streaming chat completion. On a non-2xx it returns a
// classified user-facing error; on a 2xx non-SSE response it falls back to a
// one-shot stream. The caller owns the returned Stream and must Close it.
func (c *Client) StreamRequest(ctx context.Context, msgs []ChatMessage, tools []ToolDef) (*Stream, error) {
	req := upstreamRequest{Model: c.model, Messages: msgs, Tools: tools, Stream: true}
	if len(tools) > 0 {
		req.ToolChoice = "auto"
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode chat request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build chat request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		logging.L.Error("ai upstream request", "err", err)
		return nil, ErrUpstreamUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		logging.L.Error("ai upstream status", "status", resp.StatusCode)
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, ErrUpstreamAuth
		case http.StatusNotFound:
			return nil, ErrUpstreamNotFound
		case http.StatusTooManyRequests:
			return nil, ErrUpstreamRate
		default:
			return nil, ErrUpstreamUnavailable
		}
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return &Stream{body: resp.Body, r: bufio.NewReader(resp.Body)}, nil
	}
	// Provider ignored stream:true. Read the whole completion once and serve
	// it as a single chunk.
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("read chat response: %w", err)
	}
	var nonStream struct {
		Choices []struct {
			Message struct {
				Content   string         `json:"content"`
				ToolCalls []ChatToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &nonStream); err != nil {
		return nil, fmt.Errorf("decode chat response: %w", err)
	}
	chunk := streamChunk{}
	if len(nonStream.Choices) > 0 {
		chunk.Choices = []struct {
			Delta struct {
				Content   string         `json:"content"`
				ToolCalls []ChatToolCall `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		}{{
			Delta: struct {
				Content   string         `json:"content"`
				ToolCalls []ChatToolCall `json:"tool_calls"`
			}{Content: nonStream.Choices[0].Message.Content, ToolCalls: nonStream.Choices[0].Message.ToolCalls},
		}}
	}
	return &Stream{fallback: &chunk}, nil
}
