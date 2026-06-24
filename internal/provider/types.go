package provider

type ToolDef struct {
	Name        string      `json:"name" yaml:"name"`
	Description string      `json:"description" yaml:"description"`
	InputSchema *JSONSchema `json:"inputSchema,omitempty" yaml:"inputSchema,omitempty"`

	// HTTP provider fields
	Method       string              `json:"method,omitempty" yaml:"method,omitempty"`
	Path         string              `json:"path,omitempty" yaml:"path,omitempty"`
	Params       map[string]*PropDef `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	BodyTemplate string              `json:"body_template,omitempty" yaml:"body_template,omitempty"`
	ResponsePath string              `json:"response_path,omitempty" yaml:"response_path,omitempty"`

	// External command (takes over request building entirely)
	Command string `json:"command,omitempty" yaml:"command,omitempty"`
}

type JSONSchema struct {
	Type       string              `json:"type" yaml:"type"`
	Properties map[string]*PropDef  `json:"properties,omitempty" yaml:"properties,omitempty"`
	Required   []string            `json:"required,omitempty" yaml:"required,omitempty"`
}

type PropDef struct {
	Type        string   `json:"type" yaml:"type"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Items       *PropDef `json:"items,omitempty" yaml:"items,omitempty"`
	Enum        []string `json:"enum,omitempty" yaml:"enum,omitempty"`
	Default     any      `json:"default,omitempty" yaml:"default,omitempty"`
}

type CallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Data string `json:"data,omitempty"`
}

type ServerConfig struct {
	Name          string            `json:"name" yaml:"name"`
	Description   string            `json:"description" yaml:"description"`
	Type          string            `json:"type" yaml:"type"`
	URL           string            `json:"url,omitempty" yaml:"url,omitempty"`
	BaseURL       string            `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	Headers       map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	HeaderCommand string            `json:"header_command,omitempty" yaml:"header_command,omitempty"`
	Shell         string            `json:"shell,omitempty" yaml:"shell,omitempty"`
	Tools         []ToolDef         `json:"tools,omitempty" yaml:"tools,omitempty"`
}
