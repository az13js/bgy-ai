package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"
)

func TestResolveShell(t *testing.T) {
	// empty → auto-detect
	s := resolveShell("")
	if runtime.GOOS == "windows" {
		if len(s) != 2 || s[0] != "cmd" || s[1] != "/c" {
			t.Errorf("default windows shell: got %v", s)
		}
	} else {
		if len(s) != 2 || s[0] != "sh" || s[1] != "-c" {
			t.Errorf("default unix shell: got %v", s)
		}
	}

	// custom
	s2 := resolveShell("bash -c")
	if len(s2) != 2 || s2[0] != "bash" || s2[1] != "-c" {
		t.Errorf("bash shell: got %v", s2)
	}

	// powershell
	s3 := resolveShell("powershell -Command")
	if len(s3) != 2 || s3[0] != "powershell" || s3[1] != "-Command" {
		t.Errorf("powershell: got %v", s3)
	}
}

func TestEncodeValue(t *testing.T) {
	p := &HTTPProvider{}
	tests := []struct {
		in   any
		want string
	}{
		{"hello", "hello"},
		{42, "42"},
		{true, "true"},
		{3.14, "3.14"},
		{[]int{1, 2}, "[1,2]"},
		{map[string]int{"a": 1}, `{"a":1}`},
	}
	for _, tc := range tests {
		got := p.encodeValue(tc.in)
		if got != tc.want {
			t.Errorf("encodeValue(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestExtractByPath(t *testing.T) {
	data := map[string]any{
		"items": []any{
			map[string]any{"name": "foo", "val": 42},
			map[string]any{"name": "bar", "val": 99},
		},
		"count": 2,
	}
	// simple key
	if s, err := extractByPath(data, "count"); err != nil || s != "2" {
		t.Errorf("count: got %q, err=%v", s, err)
	}
	// nested array
	if s, err := extractByPath(data, "items.0.name"); err != nil || s != "foo" {
		t.Errorf("items.0.name: got %q, err=%v", s, err)
	}
	// wildcard (first key of a map)
	wildData := map[string]any{
		"result": map[string]any{"a": "hello", "b": "world"},
	}
	if s, err := extractByPath(wildData, "result.*"); err != nil || s != "hello" {
		t.Errorf("result.*: got %q, err=%v", s, err)
	}

	// float64 integer
	if s, err := extractByPath(data, "items.1.val"); err != nil || s != "99" {
		t.Errorf("items.1.val: got %q, err=%v", s, err)
	}
	// non-existent key
	if _, err := extractByPath(data, "nonexistent"); err == nil {
		t.Error("expected error for nonexistent key")
	}
	// empty path
	if s, err := extractByPath(data, ""); err != nil || s == "" {
		t.Errorf("empty path: got %q, err=%v", s, err)
	}
}

func TestFormatResponse(t *testing.T) {
	p := &HTTPProvider{}
	// JSON response
	body := []byte(`{"status":"ok","data":[1,2,3]}`)
	result, err := p.formatResponse(body, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatal("bad result structure")
	}
	// should be pretty-printed
	if !strings.Contains(result.Content[0].Text, "status") {
		t.Error("missing status in output")
	}

	// response_path extraction
	result2, err := p.formatResponse(body, "data")
	if err != nil {
		t.Fatal(err)
	}
	// data is [1,2,3], extractByPath returns "1\n2\n3"
	if !strings.Contains(result2.Content[0].Text, "1") {
		t.Errorf("expected extracted data, got %q", result2.Content[0].Text)
	}

	// non-JSON response
	result3, _ := p.formatResponse([]byte("plain text"), "")
	if result3.Content[0].Text != "plain text" {
		t.Errorf("non-JSON should pass through, got %q", result3.Content[0].Text)
	}
}

func TestBuildPath(t *testing.T) {
	p := &HTTPProvider{}
	args := map[string]any{"id": "42", "name": "hello"}
	path := p.buildPath("/api/items/{{.id}}/info", args)
	if path != "/api/items/42/info" {
		t.Errorf("path substitution: got %q", path)
	}
	// no template
	path2 := p.buildPath("/api/static", args)
	if path2 != "/api/static" {
		t.Errorf("static path: got %q", path2)
	}
}

func TestBuildBody(t *testing.T) {
	p := &HTTPProvider{}
	args := map[string]any{"name": "test", "count": 5}
	body, err := p.buildBody(`{"name":"{{.name}}","n":{{.count}}}`, args)
	if err != nil {
		t.Fatal(err)
	}
	if body != `{"name":"test","n":5}` {
		t.Errorf("body substitution: got %q", body)
	}
	// invalid JSON
	_, err = p.buildBody(`{"name": {{.name}}`, args)
	if err == nil {
		t.Error("expected error for invalid JSON body")
	}
}

func TestBuildURL(t *testing.T) {
	p := &HTTPProvider{baseURL: "https://api.example.com/api"}
	tool := &ToolDef{
		Path: "/v1/search",
		Params: map[string]*PropDef{
			"keyword": {Type: "string"},
			"page":    {Type: "number", Default: 1},
		},
	}
	args := map[string]any{"keyword": "test"}
	u, err := p.buildURL(tool, args)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(u)
	if parsed.Query().Get("keyword") != "test" {
		t.Errorf("expected keyword=test, got %s", parsed.Query().Get("keyword"))
	}
	if parsed.Query().Get("page") != "1" {
		t.Errorf("expected page=1 (from default), got %s", parsed.Query().Get("page"))
	}
	if parsed.Path != "/api/v1/search" {
		t.Errorf("expected /api/v1/search, got %s", parsed.Path)
	}

	// Path template substitution
	tool2 := &ToolDef{
		Path: "/v1/items/{{.id}}",
		Params: map[string]*PropDef{
			"id":    {Type: "string"},
			"extra": {Type: "string"},
		},
	}
	args2 := map[string]any{"id": "123", "extra": "value"}
	u2, _ := p.buildURL(tool2, args2)
	parsed2, _ := url.Parse(u2)
	if parsed2.Path != "/api/v1/items/123" {
		t.Errorf("path template: got %s", parsed2.Path)
	}
	// id should NOT be in query (it's used in path)
	if parsed2.Query().Get("id") != "" {
		t.Error("id should not appear in query params")
	}
	if parsed2.Query().Get("extra") != "value" {
		t.Errorf("extra should be in query: got %s", parsed2.Query().Get("extra"))
	}
}

func TestResolveDefaults(t *testing.T) {
	p := &HTTPProvider{}
	args := map[string]any{
		"timestamp": "now",
		"from":      "now-1h",
		"to":        "now",
		"fixed":     "hello",
		"num":       42,
	}
	resolved := p.resolveDefaults(args)

	// "now" should be a numeric timestamp
	nowVal, ok := resolved["timestamp"].(string)
	if !ok || len(nowVal) != 13 {
		t.Errorf("timestamp should be 13-digit ms string, got %v (%d chars)", resolved["timestamp"], len(nowVal))
	}
	// "now-1h" should be different
	fromVal, _ := resolved["from"].(string)
	if fromVal == nowVal {
		t.Error("now-1h should differ from now")
	}
	// fixed value passes through
	if resolved["fixed"] != "hello" {
		t.Errorf("fixed value: got %v", resolved["fixed"])
	}
	// numeric passes through
	if resolved["num"] != 42 {
		t.Errorf("numeric value: got %v", resolved["num"])
	}
	// "now" reserved key should be set
	if _, ok := resolved["now"]; !ok {
		t.Error("now reserved key missing")
	}
}

func TestCallToolNotFound(t *testing.T) {
	p := &HTTPProvider{name: "test", tools: []ToolDef{{Name: "exists"}}}
	_, err := p.CallTool(nil, "missing", nil)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %v", err)
	}
}

func TestCallWithBodyMethod(t *testing.T) {
	methodSeen := ""
	bodySeen := ""
	contentTypeSeen := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methodSeen = r.Method
		contentTypeSeen = r.Header.Get("Content-Type")
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		bodySeen = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":1,"status":"ok"}`))
	}))
	defer srv.Close()

	p := &HTTPProvider{
		client: srv.Client(),
		baseURL: srv.URL,
		name:    "test",
		tools:   []ToolDef{{
			Name:         "create",
			Method:       "POST",
			Path:         "/api/prompt",
			BodyTemplate: `{"content":"{{.content}}"}`,
		}},
	}
	args := map[string]any{"content": "hello world"}
	result, err := p.CallTool(context.Background(), "create", args)
	if err != nil {
		t.Fatalf("callWithBody POST failed: %v", err)
	}
	if methodSeen != "POST" {
		t.Errorf("expected POST, got %s", methodSeen)
	}
	if contentTypeSeen != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", contentTypeSeen)
	}
	if !strings.Contains(bodySeen, "hello world") {
		t.Errorf("body should contain hello world, got %q", bodySeen)
	}
	if result == nil || len(result.Content) == 0 {
		t.Fatal("expected non-empty result")
	}

	// Test PATCH
	methodSeen = ""
	patchTool := &ToolDef{
		Name:         "update",
		Method:       "PATCH",
		Path:         "/api/prompt/{{.id}}",
		BodyTemplate: `{"content":"{{.content}}"}`,
	}
	p.tools = []ToolDef{*patchTool}
	result2, err := p.CallTool(context.Background(), "update", map[string]any{"id": "42", "content": "updated"})
	if err != nil {
		t.Fatalf("callWithBody PATCH failed: %v", err)
	}
	if methodSeen != "PATCH" {
		t.Errorf("expected PATCH, got %s", methodSeen)
	}
	if !strings.Contains(bodySeen, "updated") {
		t.Errorf("PATCH body should contain 'updated', got %q", bodySeen)
	}
	if result2 == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestCallWithQueryParamsMethod(t *testing.T) {
	methodSeen := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methodSeen = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	// Test DELETE
	p := &HTTPProvider{
		client: srv.Client(),
		baseURL: srv.URL,
		name:    "test",
		tools:   []ToolDef{{
			Name:   "delete",
			Method: "DELETE",
			Path:   "/api/prompt/{{.id}}",
		}},
	}
	_, err := p.CallTool(context.Background(), "delete", map[string]any{"id": "99"})
	if err != nil {
		t.Fatalf("DELETE call failed: %v", err)
	}
	if methodSeen != "DELETE" {
		t.Errorf("expected DELETE, got %s", methodSeen)
	}
}
