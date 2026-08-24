package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// FallbackClient keeps the primary provider on the normal path and only uses
// the secondary provider after an operational failure or a clear refusal.
type FallbackClient struct {
	primary    Client
	fallback   Client
	onFallback func(reason string)
}

func NewFallbackClient(primary, fallback Client, onFallback func(reason string)) *FallbackClient {
	return &FallbackClient{primary: primary, fallback: fallback, onFallback: onFallback}
}

func (c *FallbackClient) Complete(ctx context.Context, prompt string) (string, error) {
	output, primaryErr := c.primary.Complete(ctx, prompt)
	if primaryErr == nil && !looksLikeRefusal(output) {
		return output, nil
	}
	if ctx.Err() != nil || errors.Is(primaryErr, context.Canceled) || errors.Is(primaryErr, context.DeadlineExceeded) {
		return "", primaryErr
	}
	reason := "primary provider failed"
	if primaryErr == nil {
		reason = "primary provider returned a refusal"
	}
	if c.onFallback != nil {
		c.onFallback(reason)
	}
	fallbackOutput, fallbackErr := c.fallback.Complete(ctx, prompt)
	if fallbackErr != nil {
		if primaryErr != nil {
			return "", fmt.Errorf("primary provider failed: %v; fallback provider failed: %w", primaryErr, fallbackErr)
		}
		return "", fmt.Errorf("primary provider returned a refusal; fallback provider failed: %w", fallbackErr)
	}
	if looksLikeRefusal(fallbackOutput) {
		return "", fmt.Errorf("both primary and fallback providers returned a refusal")
	}
	return fallbackOutput, nil
}

func looksLikeRefusal(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return true
	}
	// Model refusals are normally short and announce themselves at the start.
	// Restrict matching to the opening text to avoid treating a real news report
	// that quotes somebody refusing an interview as a model refusal.
	prefix := []rune(value)
	if len(prefix) > 600 {
		prefix = prefix[:600]
	}
	opening := string(prefix)
	markers := []string{
		"抱歉，我无法", "抱歉，无法", "我无法回答", "我不能回答", "我无法提供", "我不能提供",
		"不便回答", "根据相关法律法规", "作为一个ai", "作为 ai",
		"i'm sorry, but i can't", "i’m sorry, but i can’t", "i cannot assist", "i can't assist",
		"i am unable to help", "i'm unable to help", "i cannot provide", "i can't provide",
	}
	for _, marker := range markers {
		if strings.Contains(opening, marker) {
			return true
		}
	}
	return false
}
