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
