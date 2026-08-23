package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type testTool struct{ calls atomic.Int32 }

func (t *testTool) Specification() ToolSpecification {
	return ToolSpecification{Name: "lookup", Description: "Look something up", Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`)}
}

func (t *testTool) Execute(_ context.Context, arguments json.RawMessage) (any, error) {
	t.calls.Add(1)
	var request struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(arguments, &request); err != nil {
		return nil, err
	}
	return map[string]string{"answer": "result for " + request.Query}, nil
}

func TestOpenAIClientComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"done"}}]}`))
	}))
	defer server.Close()

	result, err := NewOpenAIClient(server.URL, "secret", "test-model").Complete(context.Background(), "hello")
	if err != nil || result != "done" {
		t.Fatalf("Complete() = %q, %v", result, err)
	}
}

func TestOpenAIClientErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "http error", status: http.StatusBadGateway, body: `upstream unavailable`, want: "502 Bad Gateway"},
		{name: "invalid response", status: http.StatusOK, body: `{`, want: "decode llm response"},
		{name: "no choices", status: http.StatusOK, body: `{}`, want: "no choices"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			_, err := NewOpenAIClient(server.URL, "key", "model").Complete(context.Background(), "hello")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Complete() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestOpenAIClientExecutesToolLoop(t *testing.T) {
	var requests atomic.Int32
	tool := &testTool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request chatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Tools) != 1 || request.ToolChoice != "auto" {
			t.Fatalf("tools = %#v, choice = %q", request.Tools, request.ToolChoice)
		}
		if requests.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"query\":\"today\"}"}}]}}]}`))
			return
		}
		last := request.Messages[len(request.Messages)-1]
		if last.Role != "tool" || last.ToolCallID != "call_1" || !strings.Contains(last.Content, "result for today") {
			t.Fatalf("tool result message = %#v", last)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"verified answer"}}]}`))
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "secret", "test-model", WithTools(tool), WithSystemPrompt(func() string { return "use tools" }))
	result, err := client.Complete(context.Background(), "what happened?")
	if err != nil || result != "verified answer" {
		t.Fatalf("Complete() = %q, %v", result, err)
	}
	if tool.calls.Load() != 1 || requests.Load() != 2 {
		t.Fatalf("tool calls = %d, requests = %d", tool.calls.Load(), requests.Load())
	}
}

func TestOpenAIClientRejectsCurrentAnswerWithoutResearch(t *testing.T) {
	tool := &testTool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"I cannot browse right now."}}]}`))
	}))
	defer server.Close()

	_, err := NewOpenAIClient(server.URL, "secret", "test-model", WithTools(tool)).Complete(context.Background(), "总结今日全球新闻")
	if err == nil || !strings.Contains(err.Error(), "without using web search") {
		t.Fatalf("Complete() error = %v", err)
	}
}

func TestOpenAIClientFinalizesWhenToolBudgetIsExhausted(t *testing.T) {
	var requests atomic.Int32
	tool := &testTool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request chatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if requests.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"query\":\"politics\"}"}},{"id":"call_2","type":"function","function":{"name":"lookup","arguments":"{\"query\":\"economy\"}"}}]}}]}`))
			return
		}
		if len(request.Tools) != 0 || request.ToolChoice != "" {
			t.Fatalf("final request unexpectedly exposed tools: %#v, choice = %q", request.Tools, request.ToolChoice)
		}
		if len(request.Messages) < 5 || request.Messages[len(request.Messages)-1].Role != "system" {
			t.Fatalf("final messages = %#v", request.Messages)
		}
		if !strings.Contains(request.Messages[len(request.Messages)-2].Content, "budget exhausted") {
			t.Fatalf("skipped tool result = %#v", request.Messages[len(request.Messages)-2])
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"verified answer https://example.com/source"}}]}`))
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "secret", "test-model", WithTools(tool))
	client.maxToolCalls = 1
	result, err := client.Complete(context.Background(), "summarize today's news")
	if err != nil || result != "verified answer https://example.com/source" {
		t.Fatalf("Complete() = %q, %v", result, err)
	}
	if tool.calls.Load() != 1 || requests.Load() != 2 {
		t.Fatalf("tool calls = %d, requests = %d", tool.calls.Load(), requests.Load())
	}
}

func TestCompactToolContextBoundsEvidence(t *testing.T) {
	messages := []chatMessage{{Role: "system", Content: "keep this unchanged"}}
	for index := 0; index < 16; index++ {
		messages = append(messages, chatMessage{Role: "tool", ToolCallID: fmt.Sprintf("call_%d", index), Content: strings.Repeat("证据 https://example.com/source ", 1000)})
	}

	compacted := compactToolContext(messages)
	if compacted[0].Content != messages[0].Content {
		t.Fatal("non-tool message was changed")
	}
	total := 0
	for _, message := range compacted {
		if message.Role == "tool" {
			total += len(message.Content)
			if !strings.Contains(message.Content, "https://example.com/source") || !strings.Contains(message.Content, "truncated") {
				t.Fatalf("compacted tool message lost its source or marker: %q", message.Content)
			}
		}
	}
	if total > 56<<10 {
		t.Fatalf("compacted tool context = %d bytes", total)
	}
	if messages[1].Content == compacted[1].Content {
		t.Fatal("input messages were modified or not compacted")
	}
}
