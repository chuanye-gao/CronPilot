package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type OpenAIClient struct {
	baseURL       string
	apiKey        string
	model         string
	http          *http.Client
	tools         map[string]Tool
	systemPrompt  func() string
	maxToolRounds int
	maxToolCalls  int
	onToolEvent   func(ToolEvent)
}

type chatRequest struct {
	Model      string        `json:"model"`
	Messages   []chatMessage `json:"messages"`
	Tools      []chatTool    `json:"tools,omitempty"`
	ToolChoice string        `json:"tool_choice,omitempty"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
}

type chatTool struct {
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

type chatToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type OpenAIOption func(*OpenAIClient)

func WithTools(values ...Tool) OpenAIOption {
	return func(client *OpenAIClient) {
		for _, value := range values {
			if value == nil {
				continue
			}
			client.tools[value.Specification().Name] = value
		}
	}
}

func WithSystemPrompt(value func() string) OpenAIOption {
	return func(client *OpenAIClient) { client.systemPrompt = value }
}

func WithMaxToolRounds(value int) OpenAIOption {
	return func(client *OpenAIClient) {
		if value > 0 {
			client.maxToolRounds = value
		}
	}
}

func WithToolObserver(value func(ToolEvent)) OpenAIOption {
	return func(client *OpenAIClient) { client.onToolEvent = value }
}

func NewOpenAIClient(baseURL, apiKey, model string, options ...OpenAIOption) *OpenAIClient {
	client := &OpenAIClient{
		baseURL:       strings.TrimRight(baseURL, "/"),
		apiKey:        apiKey,
		model:         model,
		http:          &http.Client{Timeout: 4 * time.Minute},
		tools:         make(map[string]Tool),
		maxToolRounds: 10,
		maxToolCalls:  32,
	}
	for _, option := range options {
		option(client)
	}
	return client
}

func (c *OpenAIClient) Complete(ctx context.Context, prompt string) (string, error) {
	messages := make([]chatMessage, 0, 4)
	if c.systemPrompt != nil {
		if system := strings.TrimSpace(c.systemPrompt()); system != "" {
			messages = append(messages, chatMessage{Role: "system", Content: system})
		}
	}
	messages = append(messages, chatMessage{Role: "user", Content: prompt})
	toolCalls := 0
	requiresWeb := len(c.tools) > 0 && promptNeedsWeb(prompt)
	for round := 0; round <= c.maxToolRounds; round++ {
		response, err := c.chat(ctx, compactToolContext(messages), true)
		if err != nil {
			return "", err
		}
		if len(response.ToolCalls) == 0 {
			content := strings.TrimSpace(response.Content)
			if content == "" {
				return "", fmt.Errorf("llm returned an empty response")
			}
			if requiresWeb && toolCalls == 0 {
				return "", fmt.Errorf("current-information task completed without using web search")
			}
			if requiresWeb && !strings.Contains(content, "http://") && !strings.Contains(content, "https://") {
				return "", fmt.Errorf("current-information task completed without source links")
			}
			return content, nil
		}
		messages = append(messages, response)

		remainingCalls := c.maxToolCalls - toolCalls
		allowedCalls := len(response.ToolCalls)
		if remainingCalls < allowedCalls {
			allowedCalls = remainingCalls
		}
		if round == c.maxToolRounds {
			allowedCalls = 0
		}
		if allowedCalls > 0 {
			messages = append(messages, c.executeTools(ctx, response.ToolCalls[:allowedCalls])...)
			toolCalls += allowedCalls
		}
		for _, call := range response.ToolCalls[allowedCalls:] {
			messages = append(messages, chatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    `{"ok":false,"error":"web research budget exhausted; synthesize the final answer from sources already collected"}`,
			})
		}
		if allowedCalls < len(response.ToolCalls) || round == c.maxToolRounds {
			return c.finalizeAfterToolBudget(ctx, messages, requiresWeb, toolCalls)
		}
	}
	return "", fmt.Errorf("llm tool loop ended unexpectedly")
}

func (c *OpenAIClient) finalizeAfterToolBudget(ctx context.Context, messages []chatMessage, requiresWeb bool, toolCalls int) (string, error) {
	messages = append(messages, chatMessage{
		Role: "system",
		Content: "The web research budget is exhausted. Do not request more tools. " +
			"Produce the best final answer now using only the collected evidence. Keep uncertainty explicit and include clickable source URLs for current-information claims.",
	})
	response, err := c.chat(ctx, compactToolContext(messages), false)
	if err != nil {
		return "", err
	}
	content := strings.TrimSpace(response.Content)
	if content == "" {
		return "", fmt.Errorf("llm returned an empty response after web research budget was exhausted")
	}
	if requiresWeb && toolCalls == 0 {
		return "", fmt.Errorf("current-information task exhausted its tool budget without using web search")
	}
	if requiresWeb && !strings.Contains(content, "http://") && !strings.Contains(content, "https://") {
		return "", fmt.Errorf("current-information task completed without source links")
	}
	return content, nil
}

func compactToolContext(messages []chatMessage) []chatMessage {
	toolMessages := 0
	for _, message := range messages {
		if message.Role == "tool" {
			toolMessages++
		}
	}
	if toolMessages == 0 {
		return messages
	}

	const totalToolContextBytes = 48 << 10
	const maxToolMessageBytes = 8 << 10
	perMessage := totalToolContextBytes / toolMessages
	if perMessage > maxToolMessageBytes {
		perMessage = maxToolMessageBytes
	}
	if perMessage < 1024 {
		perMessage = 1024
	}

	compacted := append([]chatMessage(nil), messages...)
	for index := range compacted {
		message := &compacted[index]
		if message.Role != "tool" || len(message.Content) <= perMessage {
			continue
		}
		partial := message.Content[:perMessage]
		for !utf8.ValidString(partial) && len(partial) > 0 {
			partial = partial[:len(partial)-1]
		}
		data, _ := json.Marshal(map[string]any{
			"truncated":      true,
			"partial_result": partial,
			"guidance":       "Use the visible titles, snippets and source URLs; do not infer omitted text.",
		})
		message.Content = string(data)
	}
	return compacted
}

func promptNeedsWeb(prompt string) bool {
	value := strings.ToLower(prompt)
	markers := []string{
		"最新", "今日", "今天", "当天", "当日", "实时", "近期", "过去24小时", "过去 24 小时", "新闻", "大事摘要",
		"latest", "today", "current", "real-time", "realtime", "recent", "past 24 hours", "news", "weather", "stock price", "exchange rate",
	}
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func (c *OpenAIClient) chat(ctx context.Context, messages []chatMessage, includeTools bool) (chatMessage, error) {
	payload := chatRequest{Model: c.model, Messages: messages}
	if includeTools && len(c.tools) > 0 {
		payload.ToolChoice = "auto"
		payload.Tools = make([]chatTool, 0, len(c.tools))
		names := make([]string, 0, len(c.tools))
		for name := range c.tools {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			tool := c.tools[name]
			spec := tool.Specification()
			payload.Tools = append(payload.Tools, chatTool{Type: "function", Function: chatToolFunction{
				Name: spec.Name, Description: spec.Description, Parameters: spec.Parameters,
			}})
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return chatMessage{}, fmt.Errorf("marshal chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return chatMessage{}, fmt.Errorf("create chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return chatMessage{}, fmt.Errorf("call llm: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return chatMessage{}, fmt.Errorf("read llm response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return chatMessage{}, fmt.Errorf("llm returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}

	var decoded chatResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return chatMessage{}, fmt.Errorf("decode llm response: %w", err)
	}
	if decoded.Error != nil {
		return chatMessage{}, fmt.Errorf("llm error: %s", decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return chatMessage{}, fmt.Errorf("llm returned no choices")
	}
	return decoded.Choices[0].Message, nil
}

func (c *OpenAIClient) executeTools(ctx context.Context, calls []chatToolCall) []chatMessage {
	results := make([]chatMessage, len(calls))
	var wg sync.WaitGroup
	for index, call := range calls {
		index, call := index, call
		wg.Add(1)
		go func() {
			defer wg.Done()
			started := time.Now()
			content, toolErr := c.executeTool(ctx, call)
			results[index] = chatMessage{Role: "tool", ToolCallID: call.ID, Content: content}
			if c.onToolEvent != nil {
				event := ToolEvent{Name: call.Function.Name, Duration: time.Since(started).Round(time.Millisecond).String()}
				if toolErr != nil {
					event.Error = toolErr.Error()
				}
				c.onToolEvent(event)
			}
		}()
	}
	wg.Wait()
	return results
}

func (c *OpenAIClient) executeTool(ctx context.Context, call chatToolCall) (string, error) {
	tool, ok := c.tools[call.Function.Name]
	if !ok {
		err := fmt.Errorf("unknown tool %q", call.Function.Name)
		return toolErrorJSON(err), err
	}
	if len(call.Function.Arguments) > 16<<10 {
		err := fmt.Errorf("tool arguments are too large")
		return toolErrorJSON(err), err
	}
	result, err := tool.Execute(ctx, json.RawMessage(call.Function.Arguments))
	if err != nil {
		return toolErrorJSON(err), err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return toolErrorJSON(fmt.Errorf("encode tool result: %w", err)), err
	}
	const maxToolResultBytes = 24 << 10
	if len(data) > maxToolResultBytes {
		partial := string(data[:maxToolResultBytes])
		for !utf8.ValidString(partial) && len(partial) > 0 {
			partial = partial[:len(partial)-1]
		}
		data, _ = json.Marshal(map[string]any{"truncated": true, "partial_result": partial})
	}
	return string(data), nil
}

func toolErrorJSON(err error) string {
	data, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
	return string(data)
}
