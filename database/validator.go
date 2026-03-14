package database

import (
	"fmt"

	"github.com/iamankushpandit/db2-copilot-extension/config"
	pgquery "github.com/pganalyze/pg_query_go/v5"
)

func ValidateSQL(sql string, accessConfig *config.AccessConfig) error {
	tree, err := pgquery.Parse(sql)
	if err != nil {
		return fmt.Errorf("parsing sql: %w", err)
	}

	tables, err := extractTables(tree)
	if err != nil {
		return err
	}

	for _, table := range tables {
		if !isTableApproved(table, accessConfig) {
			return fmt.Errorf("table not approved: %s", table)
		}
	}

	// TODO: extract and validate columns

	return nil
}

func isTableApproved(tableName string, accessConfig *config.AccessConfig) bool {
	for _, schema := range accessConfig.ApprovedSchemas {
		if schema.AccessLevel == "full" {
			// If schema access is full, all tables are approved.
			// This is a simplification. We should probably check if the table exists in the schema.
			return true
		}
		for _, table := range schema.ApprovedTables {
			if table.Table == tableName {
				return true
			}
		}
	}
	return false
}

func extractTables(tree *pgquery.ParseResult) ([]string, error) {
	var tables []string

	for _, stmt := range tree.Stmts {
		if selectStmt := stmt.Stmt.GetSelectStmt(); selectStmt != nil {
			// This is a simplified example. A real implementation would need to handle joins, subqueries, etc.
			for _, fromClause := range selectStmt.FromClause {
				if rangeVar := fromClause.GetRangeVar(); rangeVar != nil {
					tables = append(tables, rangeVar.Relname)
				}
			}
		} else {
			return nil, fmt.Errorf("only SELECT statements are allowed")
		}
	}
	return tables, nil
}
