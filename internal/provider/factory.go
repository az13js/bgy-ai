package provider

import "fmt"

func NewProvider(cfg ServerConfig) (ToolProvider, error) {
	switch cfg.Type {
	case "mcp", "streamable-http", "":
		return NewMCPProvider(cfg)
	case "http", "rest":
		return NewHTTPProvider(cfg)
	default:
		return nil, fmt.Errorf("unknown provider type %q (valid: mcp, http)", cfg.Type)
	}
}
