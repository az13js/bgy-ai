package provider

import "context"

type ToolProvider interface {
	ListTools(ctx context.Context) ([]ToolDef, error)
	CallTool(ctx context.Context, name string, args map[string]any) (*CallResult, error)
	Name() string
	Close() error
}
