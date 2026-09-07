package llm

import (
	"context"
	"encoding/json"
)

type Client interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// Tool is a capability exposed to an OpenAI-compatible chat model. CronPilot
// owns the tool loop; providers only decide when and with which arguments to
// invoke a tool.
type Tool interface {
	Specification() ToolSpecification
	Execute(context.Context, json.RawMessage) (any, error)
}

type ToolSpecification struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

type ToolEvent struct {
	Name     string
	Duration string
	Error    string
}

type toolProgressKey struct{}

// WithToolProgress attaches a callback that is invoked for each tool call made
// during Complete. The callback may run concurrently and must be safe for
// concurrent use by the caller.
func WithToolProgress(ctx context.Context, fn func(ToolEvent)) context.Context {
	return context.WithValue(ctx, toolProgressKey{}, fn)
}

func toolProgressFromContext(ctx context.Context) func(ToolEvent) {
	fn, _ := ctx.Value(toolProgressKey{}).(func(ToolEvent))
	return fn
}
