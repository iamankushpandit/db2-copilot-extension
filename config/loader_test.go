package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// ---- helpers ---------------------------------------------------------------

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func writeJSON(t *testing.T, dir, filename string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0o640); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

// ---- LoadAll: defaults created when files missing --------------------------

func TestLoadAll_CreatesDefaults(t *testing.T) {
	dir := tempDir(t)
	m := NewConfigManager(dir)
	if err := m.LoadAll(dir); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	// Every file should now exist on disk.
	for _, f := range []string{fileAccess, fileSafety, fileLLM, fileGlossary, fileAdmin} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("expected %s to be created, got err: %v", f, err)
		}
	}
}

// ---- LoadAll: loads existing valid files -----------------------------------

func TestLoadAll_LoadsValidFiles(t *testing.T) {
	dir := tempDir(t)

	// Write a non-default access config.
	access := &AccessConfig{
		Version: "2.0",
		ApprovedSchemas: []ApprovedSchema{
			{Schema: "MYSCHEMA", AccessLevel: "full"},
		},
		HiddenSchemas: []HiddenSchema{},
	}
	writeJSON(t, dir, fileAccess, access)

	// Write default files for the rest so LoadAll doesn't create them.
	writeJSON(t, dir, fileSafety, defaultSafetyConfig())
	writeJSON(t, dir, fileLLM, defaultLLMConfig())
	writeJSON(t, dir, fileGlossary, defaultGlossaryConfig())
	writeJSON(t, dir, fileAdmin, defaultAdminConfig())

	m := NewConfigManager(dir)
	if err := m.LoadAll(dir); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	if got := m.AccessConfig().Version; got != "2.0" {
		t.Errorf("version: got %q, want %q", got, "2.0")
	}
	if !m.AccessConfig().IsSchemaApproved("MYSCHEMA") {
		t.Error("MYSCHEMA should be approved")
	}
}

// ---- LoadAll: invalid JSON causes error ------------------------------------

func TestLoadAll_InvalidJSON_ReturnsError(t *testing.T) {
	dir := tempDir(t)
	if err := os.WriteFile(filepath.Join(dir, fileAccess), []byte("{bad json"), 0o640); err != nil {
		t.Fatalf("write bad file: %v", err)
	}

	m := NewConfigManager(dir)
	if err := m.LoadAll(dir); err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// ---- Thread safety: concurrent reads during reload -------------------------

func TestLoadAll_ConcurrentReads(t *testing.T) {
	dir := tempDir(t)
	m := NewConfigManager(dir)
	if err := m.LoadAll(dir); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.AccessConfig()
			_ = m.SafetyConfig()
			_ = m.LLMConfig()
			_ = m.GlossaryConfig()
			_ = m.AdminConfig()
		}()
	}

	// Simultaneously trigger a reload.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = m.LoadAll(dir)
	}()

	wg.Wait()
}

// ---- Save helpers ----------------------------------------------------------

func TestSaveAccessConfig_PersistsAndUpdates(t *testing.T) {
	dir := tempDir(t)
	m := NewConfigManager(dir)
	if err := m.LoadAll(dir); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	cfg := &AccessConfig{
		Version: "3.0",
		ApprovedSchemas: []ApprovedSchema{
			{Schema: "SAVED", AccessLevel: "full"},
		},
		HiddenSchemas: []HiddenSchema{},
	}
	if err := m.SaveAccessConfig(cfg); err != nil {
		t.Fatalf("SaveAccessConfig: %v", err)
	}

	// In-memory should be updated.
	if !m.AccessConfig().IsSchemaApproved("SAVED") {
		t.Error("in-memory config not updated after save")
	}

	// File should be updated too — reload from disk.
	m2 := NewConfigManager(dir)
	if err := m2.LoadAll(dir); err != nil {
		t.Fatalf("second LoadAll: %v", err)
	}
	if got := m2.AccessConfig().Version; got != "3.0" {
		t.Errorf("persisted version: got %q, want %q", got, "3.0")
	}
}

// ---- Hot-reload debounce ---------------------------------------------------

func TestStartWatching_ReloadsOnFileChange(t *testing.T) {
	dir := tempDir(t)
	m := NewConfigManager(dir)
	if err := m.LoadAll(dir); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	if err := m.StartWatching(); err != nil {
		t.Fatalf("StartWatching: %v", err)
	}
	defer m.StopWatching()

	// Write a new access config to trigger a reload.
	updated := &AccessConfig{
		Version:         "9.0",
		ApprovedSchemas: []ApprovedSchema{{Schema: "HOT", AccessLevel: "full"}},
		HiddenSchemas:   []HiddenSchema{},
	}
	writeJSON(t, dir, fileAccess, updated)

	// Wait up to 5 seconds for the 2s debounce + reload to complete.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if m.AccessConfig().IsSchemaApproved("HOT") {
			return // success
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Error("hot-reload did not update config within 5 seconds")
}

// ---- NewConfigManager defaults --------------------------------------------

func TestNewConfigManager_DefaultDir(t *testing.T) {
	t.Setenv("CONFIG_DIR", "/tmp/test-config-dir")
	m := NewConfigManager("")
	if m.configDir != "/tmp/test-config-dir" {
		t.Errorf("expected /tmp/test-config-dir, got %s", m.configDir)
	}
}
