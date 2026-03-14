package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// fetchSchemaInfo queries PostgreSQL information_schema to build schema context.
// Results are cached via sync.Once in the Client struct.
func (c *Client) fetchSchemaInfo() (string, error) {
	query := `
		SELECT table_schema, table_name, column_name, data_type,
		       character_maximum_length, is_nullable
		FROM information_schema.columns
		WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
		ORDER BY table_schema, table_name, ordinal_position`

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("postgres: failed to query schema: %w", err)
	}
	defer rows.Close()

	type columnInfo struct {
		schema    string
		table     string
		column    string
		dataType  string
		maxLength *int
		isNull    string
	}

	type tableKey struct{ schema, table string }
	tableColumns := make(map[tableKey][]columnInfo)
	var tableOrder []tableKey
	seen := make(map[tableKey]bool)

	for rows.Next() {
		var ci columnInfo
		if err := rows.Scan(&ci.schema, &ci.table, &ci.column, &ci.dataType, &ci.maxLength, &ci.isNull); err != nil {
			return "", fmt.Errorf("postgres: failed to scan schema row: %w", err)
		}
		key := tableKey{ci.schema, ci.table}
		if !seen[key] {
			seen[key] = true
			tableOrder = append(tableOrder, key)
		}
		tableColumns[key] = append(tableColumns[key], ci)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("postgres: schema row iteration error: %w", err)
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
			typeStr := col.dataType
			if col.maxLength != nil {
				typeStr = fmt.Sprintf("%s(%d)", col.dataType, *col.maxLength)
			}
			nullStr := ""
			if strings.ToUpper(col.isNull) == "NO" {
				nullStr = ", NOT NULL"
			}
			fmt.Fprintf(&sb, "    - %s (%s%s)\n", col.column, typeStr, nullStr)
		}
	}

	return sb.String(), nil
}


