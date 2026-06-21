package converter

import (
	"fmt"
	"strings"

	"bgy-ai/internal/provider"
)

type Flag struct {
	Long        string
	Short       string
	Type        string
	Description string
	Required    bool
	Default     string
	Enum        []string
}

func Convert(tool provider.ToolDef) ([]Flag, error) {
	schema := tool.InputSchema
	if schema == nil {
		schema = buildSchemaFromHTTPParams(tool)
	}
	if schema == nil || len(schema.Properties) == 0 {
		return nil, nil
	}

	required := makeSet(schema.Required)
	var flags []Flag

	for name, prop := range schema.Properties {
		flag := Flag{
			Long:        toFlagName(name),
			Description: prop.Description,
			Required:    required[name],
			Enum:        prop.Enum,
		}

		flag.Type = mapPropType(prop)
		flag.Default = mapDefault(prop)

		flags = append(flags, flag)
	}

	return flags, nil
}

func ConvertMany(tools []provider.ToolDef) (map[string][]Flag, error) {
	result := make(map[string][]Flag, len(tools))
	for _, tool := range tools {
		flags, err := Convert(tool)
		if err != nil {
			return nil, fmt.Errorf("convert %q: %w", tool.Name, err)
		}
		result[tool.Name] = flags
	}
	return result, nil
}

func buildSchemaFromHTTPParams(tool provider.ToolDef) *provider.JSONSchema {
	if tool.Params == nil || len(tool.Params) == 0 {
		return nil
	}
	schema := &provider.JSONSchema{
		Type:       "object",
		Properties: tool.Params,
	}
	for name, prop := range tool.Params {
		if prop.Default == nil {
			schema.Required = append(schema.Required, name)
		}
	}
	return schema
}

func toFlagName(name string) string {
	name = strings.ReplaceAll(name, "_", "-")
	return strings.ToLower(name)
}

func makeSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

func mapPropType(prop *provider.PropDef) string {
	if prop == nil {
		return "string"
	}
	if prop.Items != nil {
		return "stringSlice"
	}
	switch prop.Type {
	case "number", "integer":
		return "int"
	case "boolean":
		return "bool"
	case "array":
		return "stringSlice"
	default:
		return "string"
	}
}

func mapDefault(prop *provider.PropDef) string {
	if prop == nil || prop.Default == nil {
		return ""
	}
	return fmt.Sprintf("%v", prop.Default)
}
