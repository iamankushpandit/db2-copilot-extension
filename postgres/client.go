package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	// Register the postgres driver.
	_ "github.com/lib/pq"

	"github.com/iamankushpandit/db2-copilot-extension/database"
)

// Client is a PostgreSQL database client implementing the database.Client interface.
type Client struct {
	db         *sql.DB
	schemaOnce sync.Once
	schemaInfo string
	schemaErr  error
}

// NewClient opens a new PostgreSQL connection pool using the provided DSN.
//
// DSN formats:
//   - URL:      postgres://user:password@host:port/dbname?sslmode=disable
//   - Key=val:  host=localhost port=5432 user=postgres password=secret dbname=mydb sslmode=disable
func NewClient(connStr string) (*Client, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to open connection: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	return &Client{db: db}, nil
}

// DatabaseType returns the database type identifier.
func (c *Client) DatabaseType() string {
	return "postgres"
}

// Ping verifies the database connection is alive.
func (c *Client) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// Close closes the underlying connection pool.
func (c *Client) Close() error {
	return c.db.Close()
}

// ExecuteQuery executes a SQL query with a 30-second timeout and returns
// the results as a slice of column-name→value maps.
func (c *Client) ExecuteQuery(ctx context.Context, query string) ([]map[string]interface{}, error) {
	sanitized, err := database.SanitizeSQL(query, "postgres")
	if err != nil {
		return nil, fmt.Errorf("postgres: query rejected: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, sanitized)
	if err != nil {
		return nil, fmt.Errorf("postgres: query execution failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to get columns: %w", err)
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("postgres: failed to scan row: %w", err)
		}
		row := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: row iteration error: %w", err)
	}

	return results, nil
}

// GetSchemaInfo returns a human-readable description of the PostgreSQL database schema.
// The result is cached after the first successful call.
func (c *Client) GetSchemaInfo() (string, error) {
	c.schemaOnce.Do(func() {
		c.schemaInfo, c.schemaErr = c.fetchSchemaInfo()
	})
	return c.schemaInfo, c.schemaErr
}

// FormatResults formats query results as a Markdown table.
func (c *Client) FormatResults(results []map[string]interface{}) string {
	return formatMarkdownTable(results)
}

// formatMarkdownTable renders results as a Markdown table.
func formatMarkdownTable(results []map[string]interface{}) string {
	if len(results) == 0 {
		return "_No results returned._"
	}

	columns := make([]string, 0, len(results[0]))
	for col := range results[0] {
		columns = append(columns, col)
	}

	var sb strings.Builder

	// Header row
	sb.WriteString("| ")
	sb.WriteString(strings.Join(columns, " | "))
	sb.WriteString(" |\n")

	// Separator row
	sb.WriteString("| ")
	seps := make([]string, len(columns))
	for i, col := range columns {
		seps[i] = strings.Repeat("-", max(len(col), 3))
	}
	sb.WriteString(strings.Join(seps, " | "))
	sb.WriteString(" |\n")

	// Data rows
	for _, row := range results {
		vals := make([]string, len(columns))
		for i, col := range columns {
			vals[i] = fmt.Sprintf("%v", row[col])
		}
		sb.WriteString("| ")
		sb.WriteString(strings.Join(vals, " | "))
		sb.WriteString(" |\n")
	}

	return sb.String()
}


