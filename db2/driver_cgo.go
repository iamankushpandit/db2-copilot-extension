//go:build cgo

package db2

// Import the IBM DB2 driver. This requires:
//   - CGO enabled (default)
//   - IBM DB2 CLI/ODBC driver (clidriver) installed
//
// See README.md for detailed installation instructions.
import _ "github.com/ibmdb/go_ibm_db"
