package converter

import (
	"testing"

	"bgy-ai/internal/provider"
)

func TestToFlagName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"search_wiki", "search-wiki"},
		{"my_tool", "my-tool"},
		{"SIMPLE", "simple"},
		{"Mixed_Case", "mixed-case"},
		{"nochange", "nochange"},
		{"double__underscore", "double--underscore"},
		{"_leading", "-leading"},
	}
	for _, c := range cases {
		got := toFlagName(c.in)
		if got != c.want {
			t.Errorf("toFlagName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMakeSet(t *testing.T) {
	s := makeSet([]string{"a", "b", "c"})
	if !s["a"] || !s["b"] || !s["c"] || s["d"] {
		t.Error("makeSet returned wrong set")
	}
	s2 := makeSet(nil)
	if len(s2) != 0 {
		t.Error("makeSet(nil) should return empty map")
	}
}

func TestMapPropType(t *testing.T) {
	cases := []struct {
		prop *provider.PropDef
		want string
	}{
		{nil, "string"},
		{&provider.PropDef{Type: "string"}, "string"},
		{&provider.PropDef{Type: "number"}, "int"},
		{&provider.PropDef{Type: "integer"}, "int"},
		{&provider.PropDef{Type: "boolean"}, "bool"},
		{&provider.PropDef{Type: "array"}, "stringSlice"},
		{&provider.PropDef{Type: "object"}, "string"},
		{&provider.PropDef{Type: "array", Items: &provider.PropDef{Type: "string"}}, "stringSlice"},
	}
	for _, c := range cases {
		got := mapPropType(c.prop)
		if got != c.want {
			t.Errorf("mapPropType(%+v) = %q, want %q", c.prop, got, c.want)
		}
	}
}

func TestMapDefault(t *testing.T) {
	if g := mapDefault(nil); g != "" {
		t.Errorf("nil prop should return empty: got %q", g)
	}
	if g := mapDefault(&provider.PropDef{}); g != "" {
		t.Errorf("nil Default should return empty: got %q", g)
	}
	if g := mapDefault(&provider.PropDef{Default: "now"}); g != "now" {
		t.Errorf("Default 'now': got %q", g)
	}
	if g := mapDefault(&provider.PropDef{Default: 42}); g != "42" {
		t.Errorf("Default 42: got %q", g)
	}
}

func TestBuildSchemaFromHTTPParams(t *testing.T) {
	// nil params
	if s := buildSchemaFromHTTPParams(provider.ToolDef{}); s != nil {
		t.Error("nil params should return nil")
	}
	// empty params
	if s := buildSchemaFromHTTPParams(provider.ToolDef{Params: map[string]*provider.PropDef{}}); s != nil {
		t.Error("empty params should return nil")
	}
	// with params
	s := buildSchemaFromHTTPParams(provider.ToolDef{
		Params: map[string]*provider.PropDef{
			"keyword": {Type: "string", Default: "x"},
			"page":    {Type: "number"},
		},
	})
	if s == nil {
		t.Fatal("expected schema")
	}
	if len(s.Properties) != 2 {
		t.Errorf("expected 2 properties, got %d", len(s.Properties))
	}
	// page has no default → required
	if len(s.Required) != 1 || s.Required[0] != "page" {
		t.Errorf("expected [page] as required, got %v", s.Required)
	}
}

func TestConvert(t *testing.T) {
	// Empty input
	flags, err := Convert(provider.ToolDef{})
	if err != nil {
		t.Fatal(err)
	}
	if len(flags) != 0 {
		t.Errorf("empty tool should produce no flags, got %d", len(flags))
	}

	// With HTTP params
	flags, err = Convert(provider.ToolDef{
		Name: "search",
		Params: map[string]*provider.PropDef{
			"keyword":   {Type: "string", Description: "Search text"},
			"verbose":   {Type: "boolean", Default: false},
			"max_count": {Type: "number", Description: "Max results"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(flags) != 3 {
		t.Fatalf("expected 3 flags, got %d", len(flags))
	}

	find := func(name string) *Flag {
		for i := range flags {
			if flags[i].Long == name {
				return &flags[i]
			}
		}
		return nil
	}

	f := find("keyword")
	if f == nil || f.Type != "string" {
		t.Error("keyword should be string")
	}
	f = find("verbose")
	if f == nil || f.Type != "bool" || f.Default != "false" || f.Required {
		t.Error("verbose should be bool, not required")
	}
	f = find("max-count")
	if f == nil || f.Type != "int" || !f.Required {
		t.Error("max_count should be int, required")
	}
}

func TestConvertMany(t *testing.T) {
	tools := []provider.ToolDef{
		{Name: "foo", Params: map[string]*provider.PropDef{"x": {Type: "string"}}},
		{Name: "bar", Params: map[string]*provider.PropDef{"y": {Type: "number"}}},
	}
	result, err := ConvertMany(tools)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
	if len(result["foo"]) != 1 || result["foo"][0].Long != "x" {
		t.Error("foo flags wrong")
	}
	if len(result["bar"]) != 1 || result["bar"][0].Long != "y" {
		t.Error("bar flags wrong")
	}
}
