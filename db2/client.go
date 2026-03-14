package db2

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	maxOpenConns     = 10
	maxIdleConns     = 5
	connMaxLifetime  = 5 * time.Minute
	queryTimeout     = 30 * time.Second
	schemaQueryLimit = 5000
)

// Client manages a DB2 connection pool and provides query helpers.
type Client struct {
	db         *sql.DB
	schemaOnce sync.Once
	schemaInfo string
	schemaErr  error
}

// NewClient opens a DB2 connection pool using the provided connection string.
// Connection string format:
//
//	HOSTNAME=<host>;PORT=<port>;DATABASE=<db>;UID=<user>;PWD=<password>;PROTOCOL=TCPIP
func NewClient(connStr string) (*Client, error) {
	db, err := sql.Open("go_ibm_db", connStr)
	if err != nil {
		return nil, fmt.Errorf("opening DB2 connection: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	return &Client{db: db}, nil
}

// Ping verifies that the DB2 server is reachable.
func (c *Client) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// Close shuts down the connection pool.
func (c *Client) Close() error {
	return c.db.Close()
}

// ExecuteQuery runs a SQL query with a 30-second timeout and returns the rows
// as a slice of maps keyed by column name.
func (c *Client) ExecuteQuery(ctx context.Context, query string) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("executing query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("fetching column names: %w", err)
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		row := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return results, nil
}

// GetSchemaInfo returns a human-readable summary of user-defined tables and
// columns from the DB2 system catalog. The result is computed once and cached.
func (c *Client) GetSchemaInfo() (string, error) {
	c.schemaOnce.Do(func() {
		c.schemaInfo, c.schemaErr = c.fetchSchemaInfo()
	})
	return c.schemaInfo, c.schemaErr
}

func (c *Client) fetchSchemaInfo() (string, error) {
	query := fmt.Sprintf(`SELECT TABSCHEMA, TABNAME, COLNAME, TYPENAME, LENGTH
		FROM SYSCAT.COLUMNS
		WHERE TABSCHEMA NOT LIKE 'SYS%%'
		  AND TABSCHEMA NOT IN ('NULLID', 'SQLJ', 'SYSCAT', 'SYSFUN', 'SYSIBM', 'SYSIBMADM', 'SYSSTAT', 'SYSTOOLS')
		ORDER BY TABSCHEMA, TABNAME, COLNO
		FETCH FIRST %d ROWS ONLY`, schemaQueryLimit)

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("querying schema catalog: %w", err)
	}
	defer rows.Close()

	type colMeta struct {
		schema   string
		table    string
		column   string
		typeName string
		length   int
	}

	// Group by schema.table
	type tableKey struct{ schema, table string }
	tableMap := make(map[tableKey][]colMeta)
	var tableOrder []tableKey

	for rows.Next() {
		var m colMeta
		if err := rows.Scan(&m.schema, &m.table, &m.column, &m.typeName, &m.length); err != nil {
			return "", fmt.Errorf("scanning schema row: %w", err)
		}
		k := tableKey{schema: strings.TrimSpace(m.schema), table: strings.TrimSpace(m.table)}
		if _, exists := tableMap[k]; !exists {
			tableOrder = append(tableOrder, k)
		}
		tableMap[k] = append(tableMap[k], m)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterating schema rows: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("## Database Schema\n\n")
	for _, k := range tableOrder {
		cols := tableMap[k]
		sb.WriteString(fmt.Sprintf("### %s.%s\n", k.schema, k.table))
		for _, c := range cols {
			sb.WriteString(fmt.Sprintf("  - %s: %s(%d)\n", strings.TrimSpace(c.column), strings.TrimSpace(c.typeName), c.length))
		}
		sb.WriteString("\n")
	}

	if len(tableOrder) == 0 {
		sb.WriteString("No user-defined tables found.\n")
	}

	return sb.String(), nil
}

// FormatResults converts query results into a markdown table string.
func FormatResults(results []map[string]interface{}) string {
	if len(results) == 0 {
		return "_No rows returned._"
	}

	// Collect ordered column names from the first row.
	columns := make([]string, 0, len(results[0]))
	for col := range results[0] {
		columns = append(columns, col)
	}

	var sb strings.Builder

	// Header row
	sb.WriteString("| ")
	sb.WriteString(strings.Join(columns, " | "))
	sb.WriteString(" |\n")

	// Separator
	sb.WriteString("| ")
	for i := range columns {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString("---")
	}
	sb.WriteString(" |\n")

	// Data rows
	for _, row := range results {
		sb.WriteString("| ")
		for i, col := range columns {
			if i > 0 {
				sb.WriteString(" | ")
			}
			val := row[col]
			if val == nil {
				sb.WriteString("NULL")
			} else {
				sb.WriteString(fmt.Sprintf("%v", val))
			}
		}
		sb.WriteString(" |\n")
	}

	return sb.String()
}
