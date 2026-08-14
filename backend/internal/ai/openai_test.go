package ai

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// cannedStream builds a Stream reading from a fixed SSE payload without any
// network. The body is a no-op closer; parsing goes through the reader.
func cannedStream(payload string) *Stream {
	return &Stream{body: io.NopCloser(strings.NewReader("")), r: bufio.NewReader(strings.NewReader(payload))}
}

func TestStreamContentDeltas(t *testing.T) {
	s := cannedStream(
		"data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"，老师\"}}]}\n\n" +
			"data: [DONE]\n\n")
	ctx := context.Background()

	var got strings.Builder
	for {
		ch, event, err := s.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !event {
			continue
		}
		got.WriteString(ch.Choices[0].Delta.Content)
	}
	if got.String() != "你好，老师" {
		t.Fatalf("content = %q, want %q", got.String(), "你好，老师")
	}
}

func TestStreamToolCallFragments(t *testing.T) {
	// Raw literals: the arguments field is a JSON string whose content is
	// itself escaped JSON, so the wire text contains literal backslashes.
	s := cannedStream(
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"query_availability","arguments":"{\"date\":\""}}]}}]}` + "\n\n" +
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"2026-08-17\"}"}}]}}]}` + "\n\n" +
			"data: [DONE]\n\n")
	ctx := context.Background()

	// Each chunk carries its own fragment; accumulation across fragments is
	// the agent loop's job. Here we verify the fragments parse losslessly.
	var fragments []string
	for {
		ch, event, err := s.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !event || len(ch.Choices) == 0 || len(ch.Choices[0].Delta.ToolCalls) == 0 {
			continue
		}
		tc := ch.Choices[0].Delta.ToolCalls[0]
		fragments = append(fragments, tc.Function.Name+"/"+tc.Function.Arguments)
	}
	if len(fragments) != 2 {
		t.Fatalf("fragments = %v, want 2", fragments)
	}
	if fragments[0] != "query_availability/{\"date\":\"" {
		t.Fatalf("fragment 0 = %q", fragments[0])
	}
	if fragments[1] != "/2026-08-17\"}" {
		t.Fatalf("fragment 1 = %q", fragments[1])
	}
}

func TestStreamKeepAliveAndMalformedLines(t *testing.T) {
	s := cannedStream(
		": keep-alive comment\n\n" +
			"data: \n\n" +
			"data: not-json\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
	ctx := context.Background()

	// Two keep-alive returns (comment + empty data), then the malformed line
	// is skipped, then the real chunk.
	for i := 0; i < 2; i++ {
		_, event, err := s.Next(ctx)
		if err != nil {
			t.Fatalf("keep-alive Next: %v", err)
		}
		if event {
			t.Fatal("keep-alive line reported as event")
		}
	}
	ch, event, err := s.Next(ctx)
	if err != nil || !event {
		t.Fatalf("after malformed line: event=%v err=%v", event, err)
	}
	if ch.Choices[0].Delta.Content != "ok" {
		t.Fatalf("content = %q", ch.Choices[0].Delta.Content)
	}
}

func TestStreamCRLF(t *testing.T) {
	s := cannedStream("data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\r\n\r\n")
	ch, event, err := s.Next(context.Background())
	if err != nil || !event {
		t.Fatalf("event=%v err=%v", event, err)
	}
	if ch.Choices[0].Delta.Content != "a" {
		t.Fatalf("content = %q", ch.Choices[0].Delta.Content)
	}
}

func TestStreamUpstreamErrorChunk(t *testing.T) {
	s := cannedStream("data: {\"error\":{\"message\":\"boom\"}}\n\n")
	ch, event, err := s.Next(context.Background())
	if err != nil || !event {
		t.Fatalf("event=%v err=%v", event, err)
	}
	if ch.Error == nil || ch.Error.Message != "boom" {
		t.Fatalf("error chunk = %+v", ch.Error)
	}
}

func TestStreamFallbackNonSSE(t *testing.T) {
	chunk := streamChunk{}
	chunk.Choices = []struct {
		Delta struct {
			Content   string         `json:"content"`
			ToolCalls []ChatToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	}{{Delta: struct {
		Content   string         `json:"content"`
		ToolCalls []ChatToolCall `json:"tool_calls"`
	}{Content: "一次性回答"}}}
	s := &Stream{fallback: &chunk}

	got, event, err := s.Next(context.Background())
	if err != nil || !event {
		t.Fatalf("event=%v err=%v", event, err)
	}
	if got.Choices[0].Delta.Content != "一次性回答" {
		t.Fatalf("content = %q", got.Choices[0].Delta.Content)
	}
	if _, _, err := s.Next(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("second Next err = %v, want EOF", err)
	}
}
