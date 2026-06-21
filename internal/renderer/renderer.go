package renderer

import (
	"encoding/json"
	"fmt"
	"strings"

	"bgy-ai/internal/provider"
)

type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

type Renderer interface {
	Render(result *provider.CallResult) (string, error)
}

func New(format Format) Renderer {
	switch format {
	case FormatJSON:
		return jsonRenderer{}
	default:
		return textRenderer{}
	}
}

type textRenderer struct{}

func (r textRenderer) Render(result *provider.CallResult) (string, error) {
	var parts []string
	for _, item := range result.Content {
		switch item.Type {
		case "text":
			if item.Text != "" {
				parts = append(parts, item.Text)
			}
		case "resource":
			if item.Data != "" {
				parts = append(parts, item.Data)
			}
		}
	}
	return strings.Join(parts, "\n---\n"), nil
}

type jsonRenderer struct{}

func (r jsonRenderer) Render(result *provider.CallResult) (string, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("render json: %w", err)
	}
	return string(data), nil
}
