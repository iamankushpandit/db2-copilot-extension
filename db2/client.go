package db2

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/iamankushpandit/db2-copilot-extension/database"
)

// Client is a DB2 database client implementing the database.Client interface.
// It uses the "go_ibm_db" SQL driver which must be registered separately.
// Build with the 'db2' build tag to include the driver registration:
//
//	go build -tags db2 .
type Client struct {
	db         *sql.DB
	schemaOnce sync.Once
	schemaInfo string
	schemaErr  error
}

// NewClient opens a new DB2 connection pool using the provided DSN.
// The DSN format is: HOSTNAME=<host>;PORT=<port>;DATABASE=<db>;UID=<user>;PWD=<pass>;PROTOCOL=TCPIP
//
// The go_ibm_db driver must be registered before calling NewClient. Register it
// by building with the 'db2' build tag or by importing it explicitly in main.
func NewClient(connStr string) (*Client, error) {
	db, err := sql.Open("go_ibm_db", connStr)
	if err != nil {
		return nil, fmt.Errorf("db2: failed to open connection: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	return &Client{db: db}, nil
}

// DatabaseType returns the database type identifier.
func (c *Client) DatabaseType() string {
	return "db2"
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
	sanitized, err := database.SanitizeSQL(query, "db2")
	if err != nil {
		return nil, fmt.Errorf("db2: query rejected: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, sanitized)
	if err != nil {
		return nil, fmt.Errorf("db2: query execution failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("db2: failed to get columns: %w", err)
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("db2: failed to scan row: %w", err)
		}
		row := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db2: row iteration error: %w", err)
	}

	return results, nil
}

// GetSchemaInfo returns a human-readable description of the DB2 database schema.
// The result is cached after the first successful call.
func (c *Client) GetSchemaInfo() (string, error) {
	c.schemaOnce.Do(func() {
		c.schemaInfo, c.schemaErr = c.fetchSchemaInfo()
	})
	return c.schemaInfo, c.schemaErr
}

func (c *Client) fetchSchemaInfo() (string, error) {
	query := `SELECT TABSCHEMA, TABNAME, COLNAME, TYPENAME, LENGTH
	          FROM SYSCAT.COLUMNS
	          WHERE TABSCHEMA NOT LIKE 'SYS%'
	            AND TABSCHEMA NOT IN ('NULLID', 'SQLJ', 'SYSCAT', 'SYSFUN', 'SYSIBM', 'SYSIBMADM', 'SYSPROC', 'SYSSTAT', 'SYSTOOLS')
	          ORDER BY TABSCHEMA, TABNAME, COLNO`

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("db2: failed to query schema: %w", err)
	}
	defer rows.Close()

	type columnInfo struct {
		schema   string
		table    string
		column   string
		typeName string
		length   int
	}

	// Group by schema → table → columns
	type tableKey struct{ schema, table string }
	tableColumns := make(map[tableKey][]columnInfo)
	var tableOrder []tableKey
	seen := make(map[tableKey]bool)

	for rows.Next() {
		var ci columnInfo
		if err := rows.Scan(&ci.schema, &ci.table, &ci.column, &ci.typeName, &ci.length); err != nil {
			return "", fmt.Errorf("db2: failed to scan schema row: %w", err)
		}
		ci.schema = strings.TrimSpace(ci.schema)
		ci.table = strings.TrimSpace(ci.table)
		key := tableKey{ci.schema, ci.table}
		if !seen[key] {
			seen[key] = true
			tableOrder = append(tableOrder, key)
		}
		tableColumns[key] = append(tableColumns[key], ci)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("db2: schema row iteration error: %w", err)
	}

	if len(tableOrder) == 0 {
		return "No user tables found in the database.", nil
	}

	var sb strings.Builder
	currentSchema := ""
	for _, key := range tableOrder {
		if key.schema != currentSchema {
			currentSchema = key.schema
			fmt.Fprintf(&sb, "Schema: %s\n", currentSchema)
		}
		fmt.Fprintf(&sb, "  Table: %s\n", key.table)
		for _, col := range tableColumns[key] {
			typeStr := col.typeName
			if col.length > 0 {
				typeStr = fmt.Sprintf("%s(%d)", col.typeName, col.length)
			}
			fmt.Fprintf(&sb, "    - %s (%s)\n", col.column, typeStr)
		}
	}

	return sb.String(), nil
}

// FormatResults formats query results as a Markdown table.
// Returns a message if results is empty.
func (c *Client) FormatResults(results []map[string]interface{}) string {
	return formatMarkdownTable(results)
}

// formatMarkdownTable renders results as a Markdown table.
func formatMarkdownTable(results []map[string]interface{}) string {
	if len(results) == 0 {
		return "_No results returned._"
	}

	// Collect ordered column names from first row
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
	for i, col := range columns {
		sb.WriteString(strings.Repeat("-", len(col)))
		if i < len(columns)-1 {
			sb.WriteString(" | ")
		}
	}
	sb.WriteString(" |\n")

	// Data rows
	for _, row := range results {
		sb.WriteString("| ")
		vals := make([]string, len(columns))
		for i, col := range columns {
			vals[i] = fmt.Sprintf("%v", row[col])
		}
		sb.WriteString(strings.Join(vals, " | "))
		sb.WriteString(" |\n")
	}

	return sb.String()
}
