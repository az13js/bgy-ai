package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type MCPProvider struct {
	name       string
	url        string
	headers    map[string]string
	client     *http.Client
	sessionID  string
	sessionDir string
	mu         sync.Mutex
	initDone   bool
}

type sessionState struct {
	SessionID string    `json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
	InitDone  bool      `json:"init_done"`
}

var defaultSessionDir = ""

func SetSessionDir(dir string) {
	defaultSessionDir = dir
}

func NewMCPProvider(cfg ServerConfig) (*MCPProvider, error) {
	p := &MCPProvider{
		name:       cfg.Name,
		url:        cfg.URL,
		headers:    cfg.Headers,
		sessionDir: defaultSessionDir,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Try to restore session from disk
	p.loadSession()
	return p, nil
}

func (p *MCPProvider) Name() string { return p.name }
func (p *MCPProvider) Close() error { return nil }

func (p *MCPProvider) sessionPath() string {
	if p.sessionDir == "" {
		return ""
	}
	return filepath.Join(p.sessionDir, p.name+".json")
}

func (p *MCPProvider) loadSession() {
	path := p.sessionPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var s sessionState
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	// Sessions expire after 24h
	if time.Since(s.CreatedAt) > 24*time.Hour {
		os.Remove(path)
		return
	}
	p.sessionID = s.SessionID
	p.initDone = s.InitDone
}

func (p *MCPProvider) saveSession() {
	path := p.sessionPath()
	if path == "" {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0700)
	s := sessionState{
		SessionID: p.sessionID,
		CreatedAt: time.Now(),
		InitDone:  p.initDone,
	}
	data, _ := json.Marshal(s)
	os.WriteFile(path, data, 0600)
}

func (p *MCPProvider) initSession(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initDone {
		return nil
	}

	sid, err := p.sendInitialize(ctx)
	if err == nil && sid != "" {
		p.sessionID = sid
		p.sendNotification(ctx, "notifications/initialized")
	}

	p.initDone = true
	p.saveSession()
	return nil
}

func (p *MCPProvider) sendInitialize(ctx context.Context) (string, error) {
	body, _ := json.Marshal(jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      0,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]string{
				"name":    "bgy-ai",
				"version": "0.1.0",
			},
		},
	})

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(body))
	p.setHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	return resp.Header.Get("Mcp-Session-Id"), nil
}

func (p *MCPProvider) sendNotification(ctx context.Context, method string) error {
	body, _ := json.Marshal(jsonrpcRequest{
		JSONRPC: "2.0",
		Method:  method,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(body))
	p.setHeaders(req)
	resp, _ := p.client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	return nil
}

func (p *MCPProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	if p.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", p.sessionID)
	}
	for k, v := range p.headers {
		if k != "Content-Type" {
			req.Header.Set(k, v)
		}
	}
}

func (p *MCPProvider) ListTools(ctx context.Context) ([]ToolDef, error) {
	if err := p.initSession(ctx); err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}

	req := jsonrpcRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"}
	var resp jsonrpcResponse
	if _, err := p.doJSONRPC(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("tools/list error: %s", resp.Error.Message)
	}

	var result struct {
		Tools []ToolDef `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list result: %w", err)
	}
	return result.Tools, nil
}

func (p *MCPProvider) CallTool(ctx context.Context, name string, args map[string]any) (*CallResult, error) {
	if err := p.initSession(ctx); err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: &callParams{
			Name:      name,
			Arguments: args,
		},
	}
	var resp jsonrpcResponse
	if _, err := p.doJSONRPC(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("tools/call %s: %w", name, err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("tools/call %s error: %s", name, resp.Error.Message)
	}

	var result CallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/call result: %w", err)
	}
	return &result, nil
}

func (p *MCPProvider) doJSONRPC(ctx context.Context, req any, resp *jsonrpcResponse) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	p.setHeaders(httpReq)

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	jsonData := extractJSON(string(respBody))
	_ = json.Unmarshal([]byte(jsonData), resp)

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return httpResp, fmt.Errorf("http status %d: %s", httpResp.StatusCode, string(respBody))
	}

	return httpResp, nil
}

func extractJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "{") {
		return raw
	}
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimSpace(data)
			if strings.HasPrefix(data, "{") {
				return data
			}
		}
	}
	return raw
}

type jsonrpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type callParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}
