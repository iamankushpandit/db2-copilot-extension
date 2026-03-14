package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/iamankushpandit/db2-copilot-extension/database"
	_ "github.com/lib/pq" // PostgreSQL driver
)

const (
	maxOpenConns    = 10
	maxIdleConns    = 5
	connMaxLifetime = 5 * time.Minute
	queryTimeout    = 30 * time.Second
)

// Client implements database.Client for PostgreSQL.
type Client struct {
	db *sql.DB
}

// NewClient opens a PostgreSQL connection pool using the provided connection string.
func NewClient(connStr string) (*Client, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("opening postgres connection: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	return &Client{db: db}, nil
}

// Ping verifies the PostgreSQL server is reachable.
func (c *Client) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// Close shuts down the connection pool.
func (c *Client) Close() error {
	return c.db.Close()
}

// DBType returns "postgres".
func (c *Client) DBType() string {
	return "postgres"
}

// ExecuteQuery runs a read-only SQL query and returns rows as maps.
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

// InjectLimit ensures the query has a LIMIT clause capped at maxRows.
func (c *Client) InjectLimit(query string, maxRows int) string {
	return database.InjectPostgresLimit(query, maxRows)
}

// VerifyReadOnly checks that the connected session and user are read-only.
func (c *Client) VerifyReadOnly(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var setting string
	err := c.db.QueryRowContext(ctx,
		"SELECT current_setting('default_transaction_read_only')").Scan(&setting)
	if err != nil {
		return false, fmt.Errorf("checking default_transaction_read_only: %w", err)
	}

	if strings.EqualFold(setting, "on") || strings.EqualFold(setting, "true") {
		return true, nil
	}

	// Check whether the current user has INSERT/UPDATE/DELETE privileges on
	// any table as a proxy for write access.
	var canWrite bool
	err = c.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.role_table_grants
			WHERE grantee = current_user
			  AND privilege_type IN ('INSERT','UPDATE','DELETE')
			LIMIT 1
		)`).Scan(&canWrite)
	if err != nil {
		return false, fmt.Errorf("checking user privileges: %w", err)
	}

	return !canWrite, nil
}

// ExplainCost returns estimated rows and cost from EXPLAIN (not ANALYZE).
func (c *Client) ExplainCost(ctx context.Context, query string) (int64, float64, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	explainSQL := "EXPLAIN " + query
	rows, err := c.db.QueryContext(ctx, explainSQL)
	if err != nil {
		return 0, 0, fmt.Errorf("running EXPLAIN: %w", err)
	}
	defer rows.Close()

	// Parse the first line of EXPLAIN output which contains the root node's
	// cost and rows estimate:
	//   Seq Scan on t  (cost=0.00..431.00 rows=1000 width=4)
	var firstLine string
	if rows.Next() {
		if err := rows.Scan(&firstLine); err != nil {
			return 0, 0, fmt.Errorf("scanning EXPLAIN output: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("iterating EXPLAIN rows: %w", err)
	}

	estimatedRows, estimatedCost := parseExplainLine(firstLine)
	return estimatedRows, estimatedCost, nil
}

// costRe matches the cost=N..N rows=N portion of EXPLAIN output.
var costRe = regexp.MustCompile(`cost=[\d.]+\.\.([\d.]+)\s+rows=(\d+)`)

func parseExplainLine(line string) (rows int64, cost float64) {
	m := costRe.FindStringSubmatch(line)
	if m == nil {
		return 0, 0
	}
	cost, _ = strconv.ParseFloat(m[1], 64)
	rows64, _ := strconv.ParseInt(m[2], 10, 64)
	return rows64, cost
}

// CrawlSchema performs full Tier 1 schema discovery for PostgreSQL.
func (c *Client) CrawlSchema(ctx context.Context) (*database.SchemaInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// Query all columns from information_schema, excluding system schemas.
	const colQuery = `
		SELECT
			c.table_schema,
			c.table_name,
			c.column_name,
			c.data_type,
			c.is_nullable,
			COALESCE(pgd.description, '') AS column_comment
		FROM information_schema.columns c
		LEFT JOIN pg_catalog.pg_statio_all_tables st
			ON st.schemaname = c.table_schema AND st.relname = c.table_name
		LEFT JOIN pg_catalog.pg_description pgd
			ON pgd.objoid = st.relid
			AND pgd.objsubid = c.ordinal_position
		WHERE c.table_schema NOT IN ('pg_catalog','information_schema','pg_toast')
		ORDER BY c.table_schema, c.table_name, c.ordinal_position`

	rows, err := c.db.QueryContext(ctx, colQuery)
	if err != nil {
		return nil, fmt.Errorf("querying information_schema.columns: %w", err)
	}
	defer rows.Close()

	type tableKey struct{ schema, table string }
	tableMap := make(map[tableKey]*database.TableDetail)
	var tableOrder []tableKey

	for rows.Next() {
		var schemaName, tableName, colName, dataType, isNullable, colComment string
		if err := rows.Scan(&schemaName, &tableName, &colName, &dataType, &isNullable, &colComment); err != nil {
			return nil, fmt.Errorf("scanning columns row: %w", err)
		}
		k := tableKey{schema: schemaName, table: tableName}
		if _, ok := tableMap[k]; !ok {
			tableMap[k] = &database.TableDetail{Name: tableName}
			tableOrder = append(tableOrder, k)
		}
		tableMap[k].Columns = append(tableMap[k].Columns, database.ColumnDetail{
			Name:       colName,
			DataType:   dataType,
			IsNullable: isNullable == "YES",
			Comment:    colComment,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating columns: %w", err)
	}

	// Query primary keys.
	const pkQuery = `
		SELECT kcu.table_schema, kcu.table_name, kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'PRIMARY KEY'
		  AND tc.table_schema NOT IN ('pg_catalog','information_schema','pg_toast')`

	pkRows, err := c.db.QueryContext(ctx, pkQuery)
	if err != nil {
		return nil, fmt.Errorf("querying primary keys: %w", err)
	}
	defer pkRows.Close()

	pkSet := make(map[tableKey]map[string]bool)
	for pkRows.Next() {
		var schemaName, tableName, colName string
		if err := pkRows.Scan(&schemaName, &tableName, &colName); err != nil {
			return nil, fmt.Errorf("scanning pk row: %w", err)
		}
		k := tableKey{schema: schemaName, table: tableName}
		if pkSet[k] == nil {
			pkSet[k] = make(map[string]bool)
		}
		pkSet[k][colName] = true
	}
	if err := pkRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pks: %w", err)
	}

	// Mark PK columns and build PrimaryKeys lists.
	for k, td := range tableMap {
		pks := pkSet[k]
		var pkList []string
		for i, col := range td.Columns {
			if pks[col.Name] {
				tableMap[k].Columns[i].IsPK = true
				pkList = append(pkList, col.Name)
			}
		}
		tableMap[k].PrimaryKeys = pkList
	}

	// Query foreign keys.
	const fkQuery = `
		SELECT
			kcu.table_schema,
			kcu.table_name,
			kcu.column_name,
			ccu.table_schema AS ref_schema,
			ccu.table_name  AS ref_table,
			ccu.column_name AS ref_column
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
			ON ccu.constraint_name = tc.constraint_name
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND tc.table_schema NOT IN ('pg_catalog','information_schema','pg_toast')`

	fkRows, err := c.db.QueryContext(ctx, fkQuery)
	if err != nil {
		return nil, fmt.Errorf("querying foreign keys: %w", err)
	}
	defer fkRows.Close()

	for fkRows.Next() {
		var schemaName, tableName, colName, refSchema, refTable, refColumn string
		if err := fkRows.Scan(&schemaName, &tableName, &colName, &refSchema, &refTable, &refColumn); err != nil {
			return nil, fmt.Errorf("scanning fk row: %w", err)
		}
		k := tableKey{schema: schemaName, table: tableName}
		if td, ok := tableMap[k]; ok {
			td.ForeignKeys = append(td.ForeignKeys, database.ForeignKey{
				Column:    colName,
				RefSchema: refSchema,
				RefTable:  refTable,
				RefColumn: refColumn,
			})
		}
	}
	if err := fkRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating fks: %w", err)
	}

	// Query row count estimates from pg_class.
	const rowCountQuery = `
		SELECT schemaname, relname, n_live_tup
		FROM pg_stat_user_tables`

	rcRows, err := c.db.QueryContext(ctx, rowCountQuery)
	if err != nil {
		return nil, fmt.Errorf("querying row counts: %w", err)
	}
	defer rcRows.Close()

	for rcRows.Next() {
		var schemaName, tableName string
		var count int64
		if err := rcRows.Scan(&schemaName, &tableName, &count); err != nil {
			return nil, fmt.Errorf("scanning row count: %w", err)
		}
		k := tableKey{schema: schemaName, table: tableName}
		if td, ok := tableMap[k]; ok {
			td.RowCountEstimate = count
		}
	}
	if err := rcRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating row counts: %w", err)
	}

	// Organize into SchemaInfo.
	schemaMap := make(map[string]*database.SchemaDetail)
	var schemaOrder []string
	for _, k := range tableOrder {
		if _, ok := schemaMap[k.schema]; !ok {
			schemaMap[k.schema] = &database.SchemaDetail{Name: k.schema}
			schemaOrder = append(schemaOrder, k.schema)
		}
		schemaMap[k.schema].Tables = append(schemaMap[k.schema].Tables, *tableMap[k])
	}

	info := &database.SchemaInfo{}
	for _, s := range schemaOrder {
		info.Schemas = append(info.Schemas, *schemaMap[s])
	}
	return info, nil
}
