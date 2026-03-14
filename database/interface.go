package database

import "context"

// Client is the common interface that all database backends must implement.
// New database backends can be added by implementing this interface.
type Client interface {
	// ExecuteQuery runs a SQL query and returns results as a slice of maps.
	ExecuteQuery(ctx context.Context, query string) ([]map[string]interface{}, error)

	// GetSchemaInfo returns a formatted string describing the database schema
	// (tables, columns, types) for use as LLM context.
	GetSchemaInfo() (string, error)

	// FormatResults formats query results as a markdown table.
	FormatResults(results []map[string]interface{}) string

	// Ping checks database connectivity.
	Ping(ctx context.Context) error

	// Close closes the database connection pool.
	Close() error

	// DatabaseType returns the type of database (e.g., "db2", "postgres").
	DatabaseType() string
}
