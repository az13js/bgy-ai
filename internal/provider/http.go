package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HTTPProvider struct {
	name    string
	baseURL string
	headers map[string]string
	tools   []ToolDef
	client  *http.Client
}

func NewHTTPProvider(cfg ServerConfig) (*HTTPProvider, error) {
	return &HTTPProvider{
		name:    cfg.Name,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		headers: cfg.Headers,
		tools:   cfg.Tools,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

func (p *HTTPProvider) Name() string { return p.name }

func (p *HTTPProvider) Close() error { return nil }

func (p *HTTPProvider) ListTools(_ context.Context) ([]ToolDef, error) {
	return p.tools, nil
}

func (p *HTTPProvider) CallTool(ctx context.Context, name string, args map[string]any) (*CallResult, error) {
	tool := p.findTool(name)
	if tool == nil {
		return nil, fmt.Errorf("tool %q not found in provider %q", name, p.name)
	}

	method := strings.ToUpper(tool.Method)
	if method == "" {
		method = "GET"
	}

	if tool.BodyTemplate != "" {
		return p.callWithBody(ctx, tool, args)
	}
	return p.callWithQueryParams(ctx, tool, method, args)
}

func (p *HTTPProvider) callWithBody(ctx context.Context, tool *ToolDef, args map[string]any) (*CallResult, error) {
	bodyStr, err := p.buildBody(tool.BodyTemplate, args)
	if err != nil {
		return nil, fmt.Errorf("build body: %w", err)
	}

	path := p.buildPath(tool.Path, args)
	reqURL := p.baseURL + "/" + strings.TrimLeft(path, "/")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(bodyStr))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range p.headers {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http post %s: %w", tool.Name, err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if httpResp.StatusCode >= 400 {
		return nil, fmt.Errorf("http status %d: %s", httpResp.StatusCode, string(respBody))
	}

	return p.formatResponse(respBody, tool.ResponsePath)
}

func (p *HTTPProvider) callWithQueryParams(ctx context.Context, tool *ToolDef, method string, args map[string]any) (*CallResult, error) {
	reqURL, err := p.buildURL(tool, args)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range p.headers {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http call %s: %w", tool.Name, err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read http response: %w", err)
	}
	if httpResp.StatusCode >= 400 {
		return nil, fmt.Errorf("http status %d: %s", httpResp.StatusCode, string(respBody))
	}

	return p.formatResponse(respBody, tool.ResponsePath)
}

func (p *HTTPProvider) formatResponse(body []byte, respPath string) (*CallResult, error) {
	var parsed any
	if json.Valid(body) {
		_ = json.Unmarshal(body, &parsed)
	}

	var text string
	if respPath != "" && parsed != nil {
		extracted, err := extractByPath(parsed, respPath)
		if err != nil {
			text = string(body)
		} else {
			text = extracted
		}
	} else if parsed != nil {
		pretty, _ := json.MarshalIndent(parsed, "", "  ")
		text = string(pretty)
	} else {
		text = string(body)
	}

	return &CallResult{
		Content: []ContentItem{
			{Type: "text", Text: text},
		},
	}, nil
}

func (p *HTTPProvider) buildPath(tmpl string, args map[string]any) string {
	data := p.resolveDefaults(args)
	result := tmpl
	for key, val := range data {
		placeholder := "{{." + key + "}}"
		if !strings.Contains(result, placeholder) {
			continue
		}
		result = strings.ReplaceAll(result, placeholder, p.encodeValue(val))
	}
	return result
}

func (p *HTTPProvider) buildBody(tmpl string, args map[string]any) (string, error) {
	data := p.resolveDefaults(args)
	result := tmpl

	for key, val := range data {
		placeholder := "{{." + key + "}}"
		if !strings.Contains(result, placeholder) {
			continue
		}
		replacement := p.encodeValue(val)
		result = strings.ReplaceAll(result, placeholder, replacement)
	}

	// Validate the result is valid JSON
	var tmp any
	if err := json.Unmarshal([]byte(result), &tmp); err != nil {
		return "", fmt.Errorf("invalid JSON body after substitution: %w\nbody: %s", err, result)
	}
	return result, nil
}

func (p *HTTPProvider) encodeValue(val any) string {
	b, err := json.Marshal(val)
	if err != nil {
		return fmt.Sprintf("%v", val)
	}
	// Strip surrounding quotes for string values (we need the raw JSON representation)
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func (p *HTTPProvider) resolveDefaults(args map[string]any) map[string]any {
	out := make(map[string]any, len(args)+2)
	now := time.Now().UnixMilli()

	for k, v := range args {
		out[k] = v
	}

	if _, ok := out["now"]; !ok {
		out["now"] = strconv.FormatInt(now, 10)
	}
	if _, ok := out["now-1h"]; !ok {
		out["now-1h"] = strconv.FormatInt(now-3600*1000, 10)
	}
	if _, ok := out["now-6h"]; !ok {
		out["now-6h"] = strconv.FormatInt(now-6*3600*1000, 10)
	}
	if _, ok := out["now-24h"]; !ok {
		out["now-24h"] = strconv.FormatInt(now-24*3600*1000, 10)
	}

	// Resolve "now", "now-6h" etc. as string values in args
	for k, v := range out {
		s, ok := v.(string)
		if !ok {
			continue
		}
		switch s {
		case "now":
			out[k] = strconv.FormatInt(now, 10)
		case "now-1h":
			out[k] = strconv.FormatInt(now-3600*1000, 10)
		case "now-6h":
			out[k] = strconv.FormatInt(now-6*3600*1000, 10)
		case "now-24h":
			out[k] = strconv.FormatInt(now-24*3600*1000, 10)
		}
	}
	return out
}

func (p *HTTPProvider) findTool(name string) *ToolDef {
	for i := range p.tools {
		if p.tools[i].Name == name {
			return &p.tools[i]
		}
	}
	return nil
}

func (p *HTTPProvider) buildURL(tool *ToolDef, args map[string]any) (string, error) {
	path := p.buildPath(tool.Path, args)
	u, err := url.Parse(p.baseURL + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		return "", fmt.Errorf("build url: %w", err)
	}

	q := u.Query()
	if tool.Params != nil {
		for key, prop := range tool.Params {
			if strings.Contains(tool.Path, "{{."+key+"}}") {
				continue
			}
			val, ok := args[key]
			if !ok && prop.Default != nil {
				val = prop.Default
			}
			if val != nil {
				switch v := val.(type) {
				case []any:
					for _, item := range v {
						q.Add(key, fmt.Sprintf("%v", item))
					}
				default:
					q.Set(key, fmt.Sprintf("%v", v))
				}
			}
		}
	} else {
		for k, v := range args {
			q.Set(k, fmt.Sprintf("%v", v))
		}
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

func extractByPath(data any, path string) (string, error) {
	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		if part == "" {
			continue
		}

		// Numeric index: array/slice access
		if idx, err := strconv.Atoi(part); err == nil {
			switch v := current.(type) {
			case []any:
				if idx < 0 || idx >= len(v) {
					return "", fmt.Errorf("index %d out of range (len=%d)", idx, len(v))
				}
				current = v[idx]
			default:
				return "", fmt.Errorf("cannot index non-array at %q", part)
			}
			continue
		}

		// Wildcard: take first key of a map
		if part == "*" {
			switch v := current.(type) {
			case map[string]any:
				for _, val := range v {
					current = val
					break
				}
			default:
				return "", fmt.Errorf("cannot wildcard non-map at %q", part)
			}
			continue
		}

		// Named key: map access
		switch v := current.(type) {
		case map[string]any:
			next, ok := v[part]
			if !ok {
				return "", fmt.Errorf("key %q not found", part)
			}
			current = next
		default:
			return "", fmt.Errorf("cannot access key %q on non-map", part)
		}
	}

	// Terminal: format
	switch v := current.(type) {
	case []any:
		var lines []string
		for _, item := range v {
			var line string
			switch x := item.(type) {
			case string:
				line = x
			case []any:
				for _, sub := range x {
					line += fmt.Sprintf("%v", sub)
				}
			default:
				line = fmt.Sprintf("%v", x)
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n"), nil
	case string:
		return v, nil
	case float64:
		if v == math.Trunc(v) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		pretty, _ := json.MarshalIndent(v, "", "  ")
		return string(pretty), nil
	}
}
