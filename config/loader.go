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

const (
	fileAccess  = "access_config.json"
	fileSafety  = "query_safety.json"
	fileLLM     = "llm_config.json"
	fileGlossary = "glossary.json"
	fileAdmin   = "admin_config.json"
)

// ConfigManager holds all JSON-based configuration files and manages their
// lifecycle: initial load, default creation, hot-reload, and thread-safe access.
type ConfigManager struct {
	mu        sync.RWMutex
	configDir string

	access   *AccessConfig
	safety   *SafetyConfig
	llm      *LLMConfig
	glossary *GlossaryConfig
	admin    *AdminConfig

	watcher  *fsnotify.Watcher
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewConfigManager creates a ConfigManager that reads from configDir.
// If configDir is empty, the value of the CONFIG_DIR environment variable is
// used, falling back to "config/".
func NewConfigManager(configDir string) *ConfigManager {
	if configDir == "" {
		configDir = os.Getenv("CONFIG_DIR")
	}
	if configDir == "" {
		configDir = "config/"
	}
	return &ConfigManager{
		configDir: configDir,
		stopCh:    make(chan struct{}),
	}
}

// LoadAll loads (or creates) all five JSON config files.
// Returns an error (and refuses to proceed) if any file contains invalid JSON.
func (m *ConfigManager) LoadAll(configDir string) error {
	if configDir != "" {
		m.configDir = configDir
	}

	access, err := loadOrCreate(m.configDir, fileAccess, defaultAccessConfig())
	if err != nil {
		return fmt.Errorf("access config: %w", err)
	}

	safety, err := loadOrCreate(m.configDir, fileSafety, defaultSafetyConfig())
	if err != nil {
		return fmt.Errorf("safety config: %w", err)
	}

	llm, err := loadOrCreate(m.configDir, fileLLM, defaultLLMConfig())
	if err != nil {
		return fmt.Errorf("llm config: %w", err)
	}

	glossary, err := loadOrCreate(m.configDir, fileGlossary, defaultGlossaryConfig())
	if err != nil {
		return fmt.Errorf("glossary config: %w", err)
	}

	admin, err := loadOrCreate(m.configDir, fileAdmin, defaultAdminConfig())
	if err != nil {
		return fmt.Errorf("admin config: %w", err)
	}

	m.mu.Lock()
	m.access = access
	m.safety = safety
	m.llm = llm
	m.glossary = glossary
	m.admin = admin
	m.mu.Unlock()

	return nil
}

// --- Thread-safe getters -------------------------------------------------

// AccessConfig returns a snapshot of the current access configuration.
func (m *ConfigManager) AccessConfig() *AccessConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.access
}

// SafetyConfig returns a snapshot of the current safety configuration.
func (m *ConfigManager) SafetyConfig() *SafetyConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.safety
}

// LLMConfig returns a snapshot of the current LLM configuration.
func (m *ConfigManager) LLMConfig() *LLMConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.llm
}

// GlossaryConfig returns a snapshot of the current glossary configuration.
func (m *ConfigManager) GlossaryConfig() *GlossaryConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.glossary
}

// AdminConfig returns a snapshot of the current admin configuration.
func (m *ConfigManager) AdminConfig() *AdminConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.admin
}

// --- Save helpers (used by the admin UI) --------------------------------

// SaveAccessConfig persists cfg to disk and updates the in-memory config.
func (m *ConfigManager) SaveAccessConfig(cfg *AccessConfig) error {
	if err := saveConfig(m.configDir, fileAccess, cfg); err != nil {
		return err
	}
	m.mu.Lock()
	m.access = cfg
	m.mu.Unlock()
	return nil
}

// SaveSafetyConfig persists cfg to disk and updates the in-memory config.
func (m *ConfigManager) SaveSafetyConfig(cfg *SafetyConfig) error {
	if err := saveConfig(m.configDir, fileSafety, cfg); err != nil {
		return err
	}
	m.mu.Lock()
	m.safety = cfg
	m.mu.Unlock()
	return nil
}

// SaveLLMConfig persists cfg to disk and updates the in-memory config.
func (m *ConfigManager) SaveLLMConfig(cfg *LLMConfig) error {
	if err := saveConfig(m.configDir, fileLLM, cfg); err != nil {
		return err
	}
	m.mu.Lock()
	m.llm = cfg
	m.mu.Unlock()
	return nil
}

// SaveGlossaryConfig persists cfg to disk and updates the in-memory config.
func (m *ConfigManager) SaveGlossaryConfig(cfg *GlossaryConfig) error {
	if err := saveConfig(m.configDir, fileGlossary, cfg); err != nil {
		return err
	}
	m.mu.Lock()
	m.glossary = cfg
	m.mu.Unlock()
	return nil
}

// SaveAdminConfig persists cfg to disk and updates the in-memory config.
func (m *ConfigManager) SaveAdminConfig(cfg *AdminConfig) error {
	if err := saveConfig(m.configDir, fileAdmin, cfg); err != nil {
		return err
	}
	m.mu.Lock()
	m.admin = cfg
	m.mu.Unlock()
	return nil
}

// --- Hot-reload ---------------------------------------------------------

// StartWatching starts a fsnotify watcher on the config directory.
// File change events are debounced by 2 seconds before triggering a reload.
// Call StopWatching for graceful shutdown.
func (m *ConfigManager) StartWatching() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create fsnotify watcher: %w", err)
	}
	if err := watcher.Add(m.configDir); err != nil {
		watcher.Close()
		return fmt.Errorf("watch config dir %q: %w", m.configDir, err)
	}
	m.watcher = watcher

	go m.watchLoop()
	return nil
}

// StopWatching stops the fsnotify watcher and the background goroutine.
// Safe to call multiple times.
func (m *ConfigManager) StopWatching() {
	if m.watcher == nil {
		return
	}
	m.stopOnce.Do(func() {
		close(m.stopCh)
		m.watcher.Close()
	})
}

// watchLoop is the background goroutine that processes fsnotify events.
func (m *ConfigManager) watchLoop() {
	const debounce = 2 * time.Second
	timer := time.NewTimer(debounce)
	timer.Stop()

	for {
		select {
		case <-m.stopCh:
			timer.Stop()
			return

		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				// Reset debounce timer on every write/create event.
				timer.Reset(debounce)
			}

		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[config] watcher error: %v", err)

		case <-timer.C:
			m.reloadAll()
		}
	}
}

// reloadAll re-reads every config file and atomically replaces any that parse
// successfully. A file with invalid JSON is skipped (logged) and the running
// config remains unchanged.
func (m *ConfigManager) reloadAll() {
	type reloadResult struct {
		access   *AccessConfig
		safety   *SafetyConfig
		llm      *LLMConfig
		glossary *GlossaryConfig
		admin    *AdminConfig
	}

	var r reloadResult

	// Read current values as fall-backs for failed reloads.
	m.mu.RLock()
	r.access = m.access
	r.safety = m.safety
	r.llm = m.llm
	r.glossary = m.glossary
	r.admin = m.admin
	m.mu.RUnlock()

	if v, err := readAndParse[AccessConfig](m.configDir, fileAccess); err != nil {
		log.Printf("[config] reload failed for %s: %v — keeping previous config", fileAccess, err)
	} else {
		r.access = v
		log.Printf("[config] reloaded %s", fileAccess)
	}

	if v, err := readAndParse[SafetyConfig](m.configDir, fileSafety); err != nil {
		log.Printf("[config] reload failed for %s: %v — keeping previous config", fileSafety, err)
	} else {
		r.safety = v
		log.Printf("[config] reloaded %s", fileSafety)
	}

	if v, err := readAndParse[LLMConfig](m.configDir, fileLLM); err != nil {
		log.Printf("[config] reload failed for %s: %v — keeping previous config", fileLLM, err)
	} else {
		r.llm = v
		log.Printf("[config] reloaded %s", fileLLM)
	}

	if v, err := readAndParse[GlossaryConfig](m.configDir, fileGlossary); err != nil {
		log.Printf("[config] reload failed for %s: %v — keeping previous config", fileGlossary, err)
	} else {
		r.glossary = v
		log.Printf("[config] reloaded %s", fileGlossary)
	}

	if v, err := readAndParse[AdminConfig](m.configDir, fileAdmin); err != nil {
		log.Printf("[config] reload failed for %s: %v — keeping previous config", fileAdmin, err)
	} else {
		r.admin = v
		log.Printf("[config] reloaded %s", fileAdmin)
	}

	m.mu.Lock()
	m.access = r.access
	m.safety = r.safety
	m.llm = r.llm
	m.glossary = r.glossary
	m.admin = r.admin
	m.mu.Unlock()
}

// --- Internal helpers ---------------------------------------------------

// loadOrCreate reads the named file from dir, parses it as T, and returns the
// result. If the file does not exist, defaultVal is written to disk and
// returned. A file that exists but contains invalid JSON is an error.
func loadOrCreate[T any](dir, filename string, defaultVal *T) (*T, error) {
	path := filepath.Join(dir, filename)

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Create the directory if necessary.
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create config dir %q: %w", dir, err)
		}
		if err := saveConfig(dir, filename, defaultVal); err != nil {
			return nil, fmt.Errorf("write default %s: %w", filename, err)
		}
		log.Printf("[config] created default config file: %s", path)
		return defaultVal, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filename, err)
	}

	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", filename, err)
	}
	return &out, nil
}

// readAndParse reads and parses a single config file; used during hot-reload.
func readAndParse[T any](dir, filename string) (*T, error) {
	path := filepath.Join(dir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filename, err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", filename, err)
	}
	return &out, nil
}

// saveConfig marshals v as indented JSON and writes it to dir/filename.
func saveConfig(dir, filename string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filename, err)
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return fmt.Errorf("write %s: %w", filename, err)
	}
	return nil
}
