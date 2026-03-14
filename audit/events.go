package audit

import "time"

type EventType string

const (
	SystemStart        EventType = "SYSTEM_START"
	DBConnectionOK     EventType = "DB_CONNECTION_OK"
	DBConnectionFailed EventType = "DB_CONNECTION_FAILED"
	// ... add other event types here
)

type Event struct {
	Timestamp time.Time   `json:"timestamp"`
	Type      EventType   `json:"type"`
	Payload   interface{} `json:"payload,omitempty"`
}

type SystemStartPayload struct {
	ConfigSummary string `json:"config_summary"`
}

// ... add other payload structs here
