package database

import (
	"context"
	"io"
)

// Client is the interface implemented by all database backends (DB2, PostgreSQL).
// All implementations must be safe for concurrent use.
type Client interface {
	// Ping verifies the database is reachable.
	Ping(ctx context.Context) error

	// ExecuteQuery runs a read-only SQL query and returns the results as a
	// slice of maps keyed by column name.
	ExecuteQuery(ctx context.Context, query string) ([]map[string]interface{}, error)

	// InjectLimit ensures the query has a LIMIT (or DB2 FETCH FIRST) clause
	// capped at maxRows. If the query already has a lower limit, it is preserved.
	InjectLimit(query string, maxRows int) string

	// ExplainCost returns the estimated row count and cost from EXPLAIN
	// WITHOUT executing the query.
	ExplainCost(ctx context.Context, query string) (estimatedRows int64, estimatedCost float64, err error)

	// VerifyReadOnly checks that the connected user cannot write data.
	// Returns (true, nil) if fully read-only, (false, nil) if write access is
	// detected (caller should log a warning), or an error if the check fails.
	VerifyReadOnly(ctx context.Context) (readOnly bool, err error)

	// CrawlSchema returns a full description of all schemas, tables, and
	// columns in the database (Tier 1 awareness). The result includes
	// PKs, FKs, column comments, and sample rows from the provided set of
	// approved table names.
	CrawlSchema(ctx context.Context) (*SchemaInfo, error)

	// Close releases all resources held by the client.
	Close() error

	// DBType returns a human-readable name for this database type ("postgres" / "db2").
	DBType() string
}

// SchemaInfo is the full Tier 1 schema representation.
type SchemaInfo struct {
	Schemas []SchemaDetail
}

// SchemaDetail describes a single schema.
type SchemaDetail struct {
	Name   string
	Tables []TableDetail
}

// TableDetail describes a single table within a schema.
type TableDetail struct {
	Name          string
	Comment       string
	Columns       []ColumnDetail
	PrimaryKeys   []string
	ForeignKeys   []ForeignKey
	RowCountEstimate int64
	SampleRows    []map[string]interface{}
}

// ColumnDetail describes a single column.
type ColumnDetail struct {
	Name       string
	DataType   string
	IsNullable bool
	IsPK       bool
	Comment    string
}

// ForeignKey describes a foreign key relationship.
type ForeignKey struct {
	// Column in this table.
	Column string
	// RefSchema and RefTable are the referenced schema.table.
	RefSchema string
	RefTable  string
	// RefColumn is the referenced column.
	RefColumn string
}

// ResultWriter is satisfied by io.Writer and http.ResponseWriter.
// It is used by status streaming helpers.
type ResultWriter interface {
	io.Writer
}
