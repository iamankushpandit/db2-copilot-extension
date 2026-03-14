//go:build db2

// Package db2 provides a DB2 database client.
// This file registers the go_ibm_db SQL driver when built with the 'db2' build tag.
//
// Prerequisites:
//   - IBM DB2 CLI driver installed (see https://github.com/ibmdb/go_ibm_db)
//   - CGO enabled (CGO_ENABLED=1)
//   - IBM_DB_HOME environment variable pointing to the DB2 CLI installation
//
// Build example:
//
//	CGO_ENABLED=1 IBM_DB_HOME=/opt/ibm/db2 go build -tags db2 .
package db2

import _ "github.com/ibmdb/go_ibm_db"
