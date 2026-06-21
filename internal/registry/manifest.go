package registry

import (
	"os"

	"gopkg.in/yaml.v3"

	"bgy-ai/internal/provider"
)

type Manifest struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Type        string            `yaml:"type"`
	URL         string            `yaml:"url,omitempty"`
	BaseURL     string            `yaml:"base_url,omitempty"`
	Headers     map[string]string `yaml:"headers,omitempty"`
	Tools       []ManifestTool    `yaml:"tools,omitempty"`
}

type ManifestTool struct {
	Name         string                       `yaml:"name"`
	Description  string                       `yaml:"description"`
	Method       string                       `yaml:"method,omitempty"`
	Path         string                       `yaml:"path,omitempty"`
	Parameters   map[string]*provider.PropDef `yaml:"parameters,omitempty"`
	BodyTemplate string                       `yaml:"body_template,omitempty"`
	ResponsePath string                       `yaml:"response_path,omitempty"`
}

func LoadManifest(path string) (*provider.ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	cfg := &provider.ServerConfig{
		Name:        m.Name,
		Description: m.Description,
		Type:        m.Type,
		URL:         m.URL,
		BaseURL:     m.BaseURL,
		Headers:     m.Headers,
	}

	if m.Type == "" {
		cfg.Type = "mcp"
	}

	for _, t := range m.Tools {
		tool := provider.ToolDef{
			Name:         t.Name,
			Description:  t.Description,
			Method:       t.Method,
			Path:         t.Path,
			Params:       t.Parameters,
			BodyTemplate: t.BodyTemplate,
			ResponsePath: t.ResponsePath,
		}
		if tool.Method == "" {
			tool.Method = "GET"
		}
		cfg.Tools = append(cfg.Tools, tool)
	}

	return cfg, nil
}
