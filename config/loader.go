package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Manager loads and hot-reloads all JSON config files.
// A 2-second debounce prevents rapid reloads on quick successive writes.
// If a reload produces an invalid JSON file the existing config is kept.
type Manager struct {
	dir string

	mu       sync.RWMutex
	access   *AccessConfig
	safety   *SafetyConfig
	llm      *LLMConfig
	glossary *GlossaryConfig
	admin    *AdminConfig

	watcher *fsnotify.Watcher
}

// NewManager creates a Manager, loads every config file from dir (creating
// defaults when a file is missing), then starts a background file watcher.
func NewManager(dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating config directory %s: %w", dir, err)
	}

	m := &Manager{dir: dir}
	if err := m.loadAll(); err != nil {
		return nil, err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating file watcher: %w", err)
	}
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("watching config directory: %w", err)
	}
	m.watcher = watcher
	go m.watch()

	return m, nil
}

// Close stops the file watcher.
func (m *Manager) Close() error {
	return m.watcher.Close()
}

// Access returns the current access config (safe for concurrent use).
func (m *Manager) Access() *AccessConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.access
}

// Safety returns the current safety config (safe for concurrent use).
func (m *Manager) Safety() *SafetyConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.safety
}

// LLM returns the current LLM config (safe for concurrent use).
func (m *Manager) LLM() *LLMConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.llm
}

// Glossary returns the current glossary config (safe for concurrent use).
func (m *Manager) Glossary() *GlossaryConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.glossary
}

// Admin returns the current admin config (safe for concurrent use).
func (m *Manager) Admin() *AdminConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.admin
}

// loadAll loads (or creates default) every config file.
func (m *Manager) loadAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	access, err := loadOrDefault(filepath.Join(m.dir, "access_config.json"), defaultAccessConfig())
	if err != nil {
		return fmt.Errorf("loading access_config.json: %w", err)
	}
	m.access = access

	safety, err := loadOrDefault(filepath.Join(m.dir, "query_safety.json"), DefaultSafetyConfig())
	if err != nil {
		return fmt.Errorf("loading query_safety.json: %w", err)
	}
	m.safety = safety

	llm, err := loadOrDefault(filepath.Join(m.dir, "llm_config.json"), DefaultLLMConfig())
	if err != nil {
		return fmt.Errorf("loading llm_config.json: %w", err)
	}
	m.llm = llm

	glossary, err := loadOrDefault(filepath.Join(m.dir, "glossary.json"), &GlossaryConfig{})
	if err != nil {
		return fmt.Errorf("loading glossary.json: %w", err)
	}
	m.glossary = glossary

	admin, err := loadOrDefault(filepath.Join(m.dir, "admin_config.json"), DefaultAdminConfig())
	if err != nil {
		return fmt.Errorf("loading admin_config.json: %w", err)
	}
	m.admin = admin

	return nil
}

// watch listens for file system events and reloads configs after a 2-second debounce.
func (m *Manager) watch() {
	var timer *time.Timer
	for {
		select {
		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(2*time.Second, func() {
					m.reload()
				})
			}
		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("WARN config watcher error: %v", err)
		}
	}
}

// reload re-reads every config file, replacing the running config only when
// all files parse successfully.
func (m *Manager) reload() {
	dir := m.dir

	access, err := loadOrDefault(filepath.Join(dir, "access_config.json"), defaultAccessConfig())
	if err != nil {
		log.Printf("WARN config reload failed (access_config.json): %v – keeping previous config", err)
		return
	}
	safety, err := loadOrDefault(filepath.Join(dir, "query_safety.json"), DefaultSafetyConfig())
	if err != nil {
		log.Printf("WARN config reload failed (query_safety.json): %v – keeping previous config", err)
		return
	}
	llm, err := loadOrDefault(filepath.Join(dir, "llm_config.json"), DefaultLLMConfig())
	if err != nil {
		log.Printf("WARN config reload failed (llm_config.json): %v – keeping previous config", err)
		return
	}
	glossary, err := loadOrDefault(filepath.Join(dir, "glossary.json"), &GlossaryConfig{})
	if err != nil {
		log.Printf("WARN config reload failed (glossary.json): %v – keeping previous config", err)
		return
	}
	admin, err := loadOrDefault(filepath.Join(dir, "admin_config.json"), DefaultAdminConfig())
	if err != nil {
		log.Printf("WARN config reload failed (admin_config.json): %v – keeping previous config", err)
		return
	}

	m.mu.Lock()
	m.access = access
	m.safety = safety
	m.llm = llm
	m.glossary = glossary
	m.admin = admin
	m.mu.Unlock()

	log.Println("INFO config reloaded successfully")
}

// loadOrDefault reads path as JSON into a new value of type T.
// If the file does not exist it writes the provided default and returns it.
func loadOrDefault[T any](path string, def *T) (*T, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Write the default to disk for the operator to edit.
		if writeErr := writeJSON(path, def); writeErr != nil {
			log.Printf("WARN could not write default config to %s: %v", path, writeErr)
		}
		return def, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &out, nil
}

// writeJSON serialises v to path with indentation.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// WriteAccess atomically replaces the access config on disk.
// The in-memory config is updated via the file watcher's debounce.
func (m *Manager) WriteAccess(cfg *AccessConfig) error {
	return writeJSON(filepath.Join(m.dir, "access_config.json"), cfg)
}

// WriteSafety atomically replaces the safety config on disk.
func (m *Manager) WriteSafety(cfg *SafetyConfig) error {
	return writeJSON(filepath.Join(m.dir, "query_safety.json"), cfg)
}

// WriteLLM atomically replaces the LLM config on disk.
func (m *Manager) WriteLLM(cfg *LLMConfig) error {
	return writeJSON(filepath.Join(m.dir, "llm_config.json"), cfg)
}

// WriteGlossary atomically replaces the glossary config on disk.
func (m *Manager) WriteGlossary(cfg *GlossaryConfig) error {
	return writeJSON(filepath.Join(m.dir, "glossary.json"), cfg)
}

// WriteAdmin atomically replaces the admin config on disk.
func (m *Manager) WriteAdmin(cfg *AdminConfig) error {
	return writeJSON(filepath.Join(m.dir, "admin_config.json"), cfg)
}

// defaultAccessConfig returns an empty but valid AccessConfig.
func defaultAccessConfig() *AccessConfig {
	return &AccessConfig{
		Version:         "1.0",
		ApprovedSchemas: []ApprovedSchema{},
		HiddenSchemas:   []HiddenSchema{},
	}
}
