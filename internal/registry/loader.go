package registry

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"bgy-ai/internal/provider"
)

var registryLogger = log.New(os.Stderr, "", log.LstdFlags)

func SetVerbose(v bool) {
	if v {
		registryLogger.SetOutput(os.Stderr)
	} else {
		registryLogger.SetOutput(io.Discard)
	}
}

type ServerEntry struct {
	Config   provider.ServerConfig
	Provider provider.ToolProvider
	Tools    []provider.ToolDef
}

type Registry struct {
	entries map[string]*ServerEntry
}

func New() *Registry {
	return &Registry{
		entries: make(map[string]*ServerEntry),
	}
}

func (r *Registry) Load(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		cfg, err := LoadManifest(path)
		if err != nil {
			registryLogger.Printf("[warn] skip %s: %v", path, err)
			continue
		}

		if _, exists := r.entries[cfg.Name]; exists {
			registryLogger.Printf("[warn] duplicate server name %q, skipping %s", cfg.Name, path)
			continue
		}

		p, err := provider.NewProvider(*cfg)
		if err != nil {
			registryLogger.Printf("[warn] create provider %s: %v", cfg.Name, err)
			continue
		}

		tools, err := p.ListTools(context.Background())
		if err != nil {
			registryLogger.Printf("[warn] discover tools for %s: %v", cfg.Name, err)
			p.Close()
			continue
		}

		r.entries[cfg.Name] = &ServerEntry{
			Config:   *cfg,
			Provider: p,
			Tools:    tools,
		}
		registryLogger.Printf("[info] loaded %s (%d tools)", cfg.Name, len(tools))
	}

	return nil
}

func (r *Registry) Reload() error {
	for name, entry := range r.entries {
		entry.Provider.Close()
		tools, err := entry.Provider.ListTools(context.Background())
		if err != nil {
			return fmt.Errorf("reload %s: %w", name, err)
		}
		entry.Tools = tools
	}
	return nil
}

func (r *Registry) Servers() []*ServerEntry {
	entries := make([]*ServerEntry, 0, len(r.entries))
	for _, e := range r.entries {
		entries = append(entries, e)
	}
	return entries
}

func (r *Registry) Get(name string) *ServerEntry {
	return r.entries[name]
}

func (r *Registry) Close() {
	for _, entry := range r.entries {
		entry.Provider.Close()
	}
}
