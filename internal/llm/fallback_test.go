package llm

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

type stubClient struct {
	output string
	err    error
	calls  *atomic.Int32
}

func (c stubClient) Complete(context.Context, string) (string, error) {
	if c.calls != nil {
		c.calls.Add(1)
	}
	return c.output, c.err
}

func TestFallbackClientUsesPrimaryOnSuccess(t *testing.T) {
	fallbackCalls := &atomic.Int32{}
	client := NewFallbackClient(stubClient{output: "primary answer"}, stubClient{output: "fallback answer", calls: fallbackCalls}, nil)
	output, err := client.Complete(context.Background(), "prompt")
	if err != nil || output != "primary answer" || fallbackCalls.Load() != 0 {
		t.Fatalf("Complete() = %q, %v; fallback calls = %d", output, err, fallbackCalls.Load())
	}
}

func TestFallbackClientHandlesFailureAndRefusal(t *testing.T) {
	tests := []stubClient{
		{err: errors.New("provider unavailable")},
		{output: "抱歉，我无法回答这个问题。"},
		{output: "I'm sorry, but I can't provide that information."},
	}
	for _, primary := range tests {
		client := NewFallbackClient(primary, stubClient{output: "neutral summary with sources"}, nil)
		output, err := client.Complete(context.Background(), "prompt")
		if err != nil || output != "neutral summary with sources" {
			t.Fatalf("Complete() = %q, %v", output, err)
		}
	}
}

func TestFallbackClientDoesNotMisclassifyReportedRefusal(t *testing.T) {
	output := "今日新闻摘要：有关机构拒绝回答记者提问，但报道已由多个来源确认。"
	client := NewFallbackClient(stubClient{output: output}, stubClient{output: "wrong fallback"}, nil)
	got, err := client.Complete(context.Background(), "prompt")
	if err != nil || got != output {
		t.Fatalf("Complete() = %q, %v", got, err)
	}
}

func TestFallbackClientHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fallbackCalls := &atomic.Int32{}
	client := NewFallbackClient(stubClient{err: context.Canceled}, stubClient{output: "fallback", calls: fallbackCalls}, nil)
	if _, err := client.Complete(ctx, "prompt"); !errors.Is(err, context.Canceled) || fallbackCalls.Load() != 0 {
		t.Fatalf("error = %v; fallback calls = %d", err, fallbackCalls.Load())
	}
}
