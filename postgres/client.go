package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	_ "github.com/lib/pq"
	"github.com/iamankushpandit/db2-copilot-extension/config"
	"github.com/iamankushpandit/db2-copilot-extension/database"
)

type pgClient struct {
	db *sql.DB
}

func NewClient(connStr string) (database.Client, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("opening postgres connection: %w", err)
	}

	return &pgClient{db: db}, nil
}

func (c *pgClient) GetTier1Schema(ctx context.Context) (*database.Schema, error) {
	// TODO: implement full schema crawler with constraints, comments, etc.
	tables, err := c.getTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting tables: %w", err)
	}

	for i := range tables {
		columns, err := c.getColumns(ctx, tables[i].Schema, tables[i].Name)
		if err != nil {
			return nil, fmt.Errorf("getting columns for table %s.%s: %w", tables[i].Schema, tables[i].Name, err)
		}
		tables[i].Columns = columns
	}

	return &database.Schema{Tables: tables}, nil
}

func (c *pgClient) GetTier2Schema(ctx context.Context, accessConfig *config.AccessConfig) (*database.Schema, error) {
	// TODO: implement filtering based on accessConfig
	return c.GetTier1Schema(ctx)
}


func (c *pgClient) getTables(ctx context.Context) ([]database.Table, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT table_schema, table_name
		FROM information_schema.tables
		WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []database.Table
	for rows.Next() {
		var table database.Table
		if err := rows.Scan(&table.Schema, &table.Name); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}

	return tables, nil
}

func (c *pgClient) getColumns(ctx context.Context, schema, table string) ([]database.Column, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
	`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []database.Column
	for rows.Next() {
		var column database.Column
		if err := rows.Scan(&column.Name, &column.Type); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}

	return columns, nil
}

func (c *pgClient) ExecuteQuery(ctx context.Context, query string) ([]map[string]interface{}, error) {
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("executing query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("getting columns: %w", err)
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		rowData := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// TODO: handle different data types
			rowData[col] = val
		}
		results = append(results, rowData)
	}

	return results, nil
}

func (c *pgClient) EstimateQueryCost(ctx context.Context, query string) (*database.CostEstimate, error) {
	explainQuery := "EXPLAIN (FORMAT JSON) " + query
	var result string
	err := c.db.QueryRowContext(ctx, explainQuery).Scan(&result)
	if err != nil {
		return nil, fmt.Errorf("executing explain query: %w", err)
	}

	var plans []map[string]interface{}
	err = json.Unmarshal([]byte(result), &plans)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling explain result: %w", err)
	}

	if len(plans) == 0 {
		return nil, fmt.Errorf("explain result is empty")
	}

	plan := plans[0]["Plan"].(map[string]interface{})
	estimatedRows := int(plan["Plan Rows"].(float64))
	estimatedCost := int(plan["Total Cost"].(float64))

	return &database.CostEstimate{
		EstimatedRows: estimatedRows,
		EstimatedCost: estimatedCost,
	}, nil
}

func (c *pgClient) VerifyReadOnly() error {
	var readonly string
	err := c.db.QueryRow("SHOW default_transaction_read_only").Scan(&readonly)
	if err != nil {
		return fmt.Errorf("could not query default_transaction_read_only: %w", err)
	}

	if readonly != "on" {
		// TODO: Log verbose warning with liability disclaimer
		fmt.Println("WARNING: default_transaction_read_only is not 'on'. The database connection is not guaranteed to be read-only.")
	} else {
		fmt.Println("INFO: default_transaction_read_only is 'on'.")
	}

	return nil
}

func (c *pgClient) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

func (c *pgClient) Close() error {
	return c.db.Close()
}
