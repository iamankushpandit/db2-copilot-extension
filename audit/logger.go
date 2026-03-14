package audit

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/iamankushpandit/db2-copilot-extension/config"
)

type Logger struct {
	file *os.File
	mu   sync.Mutex
}

var (
	globalLogger *Logger
)

func Init(cfg *config.Audit) error {
	if !cfg.Enabled {
		log.Println("INFO audit logging is disabled")
		return nil
	}

	fileName := fmt.Sprintf("audit-%s.jsonl", time.Now().Format("2006-01-02"))
	filePath := filepath.Join(cfg.Directory, fileName)

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening audit log file: %w", err)
	}

	globalLogger = &Logger{
		file: file,
	}

	log.Printf("INFO audit logger initialised, writing to %s", filePath)
	return nil
}

func Log(eventType EventType, payload interface{}) {
	if globalLogger == nil {
		return
	}

	event := Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Payload:   payload,
	}

	globalLogger.mu.Lock()
	defer globalLogger.mu.Unlock()

	line, err := json.Marshal(event)
	if err != nil {
		log.Printf("ERROR marshaling audit event: %v", err)
		return
	}

	_, err = globalLogger.file.WriteString(string(line) + "
")
	if err != nil {
		log.Printf("ERROR writing to audit log: %v", err)
	}
}

func Close() {
	if globalLogger != nil && globalLogger.file != nil {
		globalLogger.file.Close()
	}
}
