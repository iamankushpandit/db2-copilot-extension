package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Config struct {
	Access   *AccessConfig
	Safety   *QuerySafety
	LLM      *LLMConfig
	Glossary *Glossary
	Admin    *AdminConfig
}

var (
	globalConfig = &Config{}
	configMutex  = &sync.RWMutex{}
)

func LoadAll(configPath string) error {
	// Initial load
	if err := loadConfig(configPath); err != nil {
		return err
	}

	go watchConfig(configPath)
	return nil
}

func watchConfig(configPath string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("ERROR creating config watcher: %v", err)
		return
	}
	defer watcher.Close()

	err = watcher.Add(configPath)
	if err != nil {
		log.Printf("ERROR adding config path to watcher: %v", err)
		return
	}

	log.Printf("INFO watching config directory for changes: %s", configPath)

	var (
		debounceTimer *time.Timer
		lastEvent     time.Time
	)
	const debounceDuration = 2 * time.Second

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				lastEvent = time.Now()
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(debounceDuration, func() {
					// Check if a new event came in since we started the timer
					if time.Since(lastEvent) >= debounceDuration {
						log.Printf("INFO reloading config due to change in: %s", event.Name)
						if err := loadConfig(configPath); err != nil {
							log.Printf("ERROR reloading config: %v", err)
						} else {
							log.Printf("INFO config reloaded successfully")
						}
					}
				})
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("ERROR watching config: %v", err)
		}
	}
}

func loadConfig(configPath string) error {
	accessConfig, err := loadAccessConfig(filepath.Join(configPath, "access_config.json"))
	if err != nil {
		return err
	}

	querySafety, err := loadQuerySafety(filepath.Join(configPath, "query_safety.json"))
	if err != nil {
		return err
	}

	llmConfig, err := loadLLMConfig(filepath.Join(configPath, "llm_config.json"))
	if err != nil {
		return err
	}

	glossary, err := loadGlossary(filepath.Join(configPath, "glossary.json"))
	if err != nil {
		return err
	}

	adminConfig, err := loadAdminConfig(filepath.Join(configPath, "admin_config.json"))
	if err != nil {
		return err
	}

	newConfig := &Config{
		Access:   accessConfig,
		Safety:   querySafety,
		LLM:      llmConfig,
		Glossary: glossary,
		Admin:    adminConfig,
	}

	configMutex.Lock()
	defer configMutex.Unlock()
	globalConfig = newConfig

	return nil
}


func Get() *Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return globalConfig
}


func loadAccessConfig(path string) (*AccessConfig, error) {
	var config AccessConfig
	err := loadJSONFile(path, &config)
	return &config, err
}

func loadQuerySafety(path string) (*QuerySafety, error) {
	var config QuerySafety
	err := loadJSONFile(path, &config)
	return &config, err
}

func loadLLMConfig(path string) (*LLMConfig, error) {
	var config LLMConfig
	err := loadJSONFile(path, &config)
	return &config, err
}

func loadGlossary(path string) (*Glossary, error) {
	var config Glossary
	err := loadJSONFile(path, &config)
	return &config, err
}

func loadAdminConfig(path string) (*AdminConfig, error) {
	var config AdminConfig
	err := loadJSONFile(path, &config)
	return &config, err
}

func loadJSONFile(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
