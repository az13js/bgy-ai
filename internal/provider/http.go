package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type HTTPProvider struct {
	name          string
	baseURL       string
	headers       map[string]string
	tools         []ToolDef
	client        *http.Client
	headerCommand string
	shell         []string
}

func NewHTTPProvider(cfg ServerConfig) (*HTTPProvider, error) {
	return &HTTPProvider{
		name:          cfg.Name,
		baseURL:       strings.TrimRight(cfg.BaseURL, "/"),
		headers:       cfg.Headers,
		tools:         cfg.Tools,
		headerCommand: cfg.HeaderCommand,
		shell:         resolveShell(cfg.Shell),
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

func resolveShell(custom string) []string {
	if custom != "" {
		return strings.Fields(custom)
	}
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c"}
	}
	return []string{"sh", "-c"}
}

func (p *HTTPProvider) CallTool(ctx context.Context, name string, args map[string]any) (*CallResult, error) {
	tool := p.findTool(name)
	if tool == nil {
		return nil, fmt.Errorf("tool %q not found in provider %q", name, p.name)
	}

	if tool.Command != "" {
		return p.execToolCommand(ctx, tool, args)
	}

	method := strings.ToUpper(tool.Method)
	if method == "" {
		method = "GET"
	}

	if tool.BodyTemplate != "" {
		return p.callWithBody(ctx, tool, method, args)
	}
	return p.callWithQueryParams(ctx, tool, method, args)
}

func (p *HTTPProvider) callWithBody(ctx context.Context, tool *ToolDef, method string, args map[string]any) (*CallResult, error) {
	bodyStr, err := p.buildBody(tool.BodyTemplate, args)
	if err != nil {
		return nil, fmt.Errorf("build body: %w", err)
	}

	path := p.buildPath(tool.Path, args)
	reqURL := p.baseURL + "/" + strings.TrimLeft(path, "/")
	httpReq, err := http.NewRequestWithContext(ctx, method, reqURL, strings.NewReader(bodyStr))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range p.headers {
		httpReq.Header.Set(k, v)
	}

	if p.headerCommand != "" {
		p.applySigning(httpReq, method, reqURL, bodyStr)
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http %s %s: %w", method, tool.Name, err)
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

	if p.headerCommand != "" {
		p.applySigning(httpReq, method, reqURL, "")
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

func (p *HTTPProvider) applySigning(httpReq *http.Request, method string, reqURL string, bodyStr string) {
	parsed, _ := url.Parse(reqURL)
	timestampStr := parsed.Query().Get("timestamp")
	if timestampStr == "" {
		timestampStr = strconv.FormatInt(time.Now().UnixMilli(), 10)
	}

	p.execSignCommand(httpReq, method, reqURL, bodyStr, timestampStr)
}

func (p *HTTPProvider) execSignCommand(httpReq *http.Request, method string, reqURL string, bodyStr string, timestampStr string) {
	parsed, _ := url.Parse(reqURL)
	signPath := parsed.Path
	if parsed.RawQuery != "" {
		signPath += "?" + parsed.RawQuery
	}

	input := map[string]string{
		"method":    method,
		"path":      signPath,
		"timestamp": timestampStr,
		"body":      bodyStr,
	}
	inputJSON, _ := json.Marshal(input)

	ctx, cancel := context.WithTimeout(httpReq.Context(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.shell[0], append(p.shell[1:], p.headerCommand)...)
	cmd.Stdin = bytes.NewReader(inputJSON)

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[warn] sign command failed: %v (output: %s)\n", err, string(output))
		return
	}

	var headers map[string]string
	if err := json.Unmarshal(output, &headers); err != nil {
		fmt.Fprintf(os.Stderr, "[warn] sign command output is not valid JSON: %v\n%s\n", err, string(output))
		return
	}

	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
}

type toolCommandInput struct {
	Method       string         `json:"method"`
	PathTemplate string         `json:"path_template"`
	BaseURL      string         `json:"base_url"`
	Params       map[string]any `json:"params"`
	BodyTemplate string         `json:"body_template"`
	TimestampMS  string         `json:"timestamp_ms"`
}

type toolCommandOutput struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func (p *HTTPProvider) execToolCommand(ctx context.Context, tool *ToolDef, args map[string]any) (*CallResult, error) {
	timestampStr := strconv.FormatInt(time.Now().UnixMilli(), 10)

	input := toolCommandInput{
		Method:       strings.ToUpper(tool.Method),
		PathTemplate: tool.Path,
		BaseURL:      p.baseURL,
		Params:       args,
		BodyTemplate: tool.BodyTemplate,
		TimestampMS:  timestampStr,
	}
	inputJSON, _ := json.Marshal(input)

	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, p.shell[0], append(p.shell[1:], tool.Command)...)
	cmd.Stdin = bytes.NewReader(inputJSON)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tool command failed: %w (output: %s)", err, string(output))
	}

	var out toolCommandOutput
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("tool command output is not valid JSON: %w\n%s", err, string(output))
	}

	method := strings.ToUpper(tool.Method)
	if method == "" {
		method = "GET"
	}

	reqURL := out.URL
	if reqURL == "" {
		return nil, fmt.Errorf("tool command did not return a url")
	}

	var bodyReader io.Reader
	if out.Body != "" {
		bodyReader = strings.NewReader(out.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	for k, v := range p.headers {
		httpReq.Header.Set(k, v)
	}
	if out.Body != "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	for k, v := range out.Headers {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http call %s: %w", tool.Name, err)
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

	// Replace any remaining unresolved placeholders with null
	re := regexp.MustCompile(`\{\{\.\w+\}\}`)
	result = re.ReplaceAllString(result, "null")

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

	resolved := p.resolveDefaults(args)
	q := u.Query()
	if tool.Params != nil {
		for key, prop := range tool.Params {
			if strings.Contains(tool.Path, "{{."+key+"}}") {
				continue
			}
			val, ok := resolved[key]
			if !ok {
				val, ok = args[key]
			}
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
