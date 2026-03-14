# Multi-Database Copilot Extension (DB2 & PostgreSQL)

A **GitHub Copilot Extension** that lets you query **IBM DB2** and **PostgreSQL** databases using natural language directly in GitHub Copilot Chat.

Ask questions like:
- _"Show me the top 10 customers by revenue this quarter"_
- _"How many orders were placed last month?"_
- _"Which products have inventory below the reorder threshold?"_

The agent translates your question into a safe, read-only SQL query, executes it against your database, and explains the results in plain English — all within the Copilot chat interface.

---

## Table of Contents

1. [Architecture](#architecture)
2. [Project Structure](#project-structure)
3. [Prerequisites](#prerequisites)
4. [Setup & Installation](#setup--installation)
5. [Configuration Reference](#configuration-reference)
6. [Running the Agent](#running-the-agent)
7. [Usage Examples](#usage-examples)
8. [Docker & docker-compose](#docker--docker-compose)
9. [Security](#security)
10. [Troubleshooting](#troubleshooting)
11. [Adding New Database Backends](#adding-new-database-backends)

---

## Architecture

```
User (GitHub Copilot Chat / Microsoft Teams)
        │
        │  Natural language question
        ▼
Copilot Platform (GitHub)
        │
        │  Signed HTTP POST /agent (SSE)
        ▼
┌─────────────────────────────────────────────────────┐
│  Agent Server (this application)                    │
│                                                     │
│  1. Verify request signature (ECDSA)                │
│  2. Load DB schema  ──────────────┐                 │
│  3. Build LLM context             │                 │
│  4. Generate SQL  ◄───────────────┘                 │
│     (Copilot LLM API, non-streaming)                │
│  5. Sanitize SQL                                    │
│     • SELECT-only                                   │
│     • No comments / injection                       │
│  6. Execute query                                   │
│         │                                           │
│         ▼                                           │
│  ┌──────────────────────┐                           │
│  │  database.Client     │  (interface)              │
│  │  ┌────────┐          │                           │
│  │  │  DB2   │◄─────────┤ DB_TYPE=db2               │
│  │  └────────┘          │                           │
│  │  ┌──────────┐        │                           │
│  │  │PostgreSQL│◄───────┤ DB_TYPE=postgres          │
│  │  └──────────┘        │                           │
│  └──────────────────────┘                           │
│  7. Stream results + explanation (SSE)              │
└─────────────────────────────────────────────────────┘
        │
        │  Server-Sent Events (streamed response)
        ▼
User sees formatted results & explanation
```

### Key Design Decisions

| Decision | Rationale |
|---|---|
| `database.Client` interface | Allows any database backend to be swapped in by implementing six methods |
| Two-step LLM call | First call generates SQL; second call explains results — keeps logic clean |
| SQL sanitizer | Server-side enforcement of SELECT-only queries prevents data modification |
| Schema caching (`sync.Once`) | Schema is fetched once per process lifetime to minimise DB load |
| ECDSA signature verification | All requests are verified against GitHub's public key |

---

## Project Structure

```
db2-copilot-extension/
├── main.go                      # HTTP server, routing, DB client factory
├── agent/
│   └── service.go               # Chat completion handler, LLM orchestration
├── copilot/
│   ├── endpoints.go             # Copilot LLM API client (streaming & non-streaming)
│   └── messages.go              # Request/response types, SSE helpers
├── database/
│   ├── interface.go             # database.Client interface (common contract)
│   └── sanitizer.go             # SQL sanitizer (SELECT-only, injection prevention)
├── db2/
│   ├── client.go                # DB2 client implementing database.Client
│   └── driver.go                # go_ibm_db driver registration (build tag: db2)
├── postgres/
│   ├── client.go                # PostgreSQL client implementing database.Client
│   └── schema.go                # information_schema introspection
├── config/
│   └── config.go                # Environment variable management & validation
├── oauth/
│   └── service.go               # GitHub App OAuth pre-authorization flow
├── Dockerfile                   # Multi-stage build with libpq support
├── docker-compose.yml           # Agent + optional PostgreSQL container
├── .env.example                 # Environment variable template
├── go.mod
└── go.sum
```

---

## Prerequisites

### Common (all databases)
- **Go 1.22+** — <https://go.dev/dl/>
- **GitHub App** with Copilot permissions (see [Create a GitHub App](#1-create-a-github-app))
- A public URL for your agent (e.g. [ngrok](https://ngrok.com), Cloudflare Tunnel, or a cloud VM)

### PostgreSQL
- PostgreSQL 12 or newer
- `libpq-dev` (build) / `libpq5` (runtime) — usually available via `apt`
- Go PostgreSQL driver is downloaded automatically by `go mod tidy`

### IBM DB2 _(only if DB_TYPE=db2)_
- IBM DB2 11.5 or newer
- IBM DB2 CLI driver (clidriver) installed and `IBM_DB_HOME` set
- CGO enabled (`CGO_ENABLED=1`)
- See <https://github.com/ibmdb/go_ibm_db> for detailed installation

---

## Setup & Installation

### 1. Create a GitHub App

1. Go to **Settings → Developer Settings → GitHub Apps → New GitHub App**
2. Fill in:
   - **GitHub App name**: `my-db-copilot` (must be unique)
   - **Homepage URL**: your FQDN (e.g. `https://abc123.ngrok.app`)
   - **Callback URL**: `https://abc123.ngrok.app/auth/callback`
3. Under **Copilot** tab:
   - **Agent**: Enabled
   - **URL**: `https://abc123.ngrok.app/agent`
   - **Pre-Authorization URL**: `https://abc123.ngrok.app/auth/authorization`
4. Under **Permissions & Events**:
   - `Account Permissions → Copilot Chat → Access: Read Only`
5. Save and note the **Client ID** and **Client Secret**

### 2. Clone & Configure

```bash
git clone https://github.com/iamankushpandit/db2-copilot-extension
cd db2-copilot-extension

cp .env.example .env
# Edit .env with your values
```

### 3a. Quick Start with PostgreSQL _(recommended for testing)_

```bash
# Install dependencies
go mod tidy

# Set environment variables
export CLIENT_ID=your_github_app_client_id
export CLIENT_SECRET=your_github_app_client_secret
export FQDN=https://abc123.ngrok.app
export DB_TYPE=postgres
export POSTGRES_CONN_STRING=postgres://postgres:password@localhost:5432/mydb?sslmode=disable

# Run
go run .
```

Or use docker-compose for a complete local environment:

```bash
# Copy and edit .env
cp .env.example .env
# Set CLIENT_ID, CLIENT_SECRET, FQDN in .env

docker-compose up
```

### 3b. IBM DB2 Setup

> **Note**: DB2 support requires the IBM DB2 CLI driver and CGO.

```bash
# 1. Install IBM DB2 CLI driver (see https://github.com/ibmdb/go_ibm_db)
export IBM_DB_HOME=/path/to/clidriver

# 2. Install go_ibm_db
CGO_ENABLED=1 go get github.com/ibmdb/go_ibm_db

# 3. Build with DB2 support
CGO_ENABLED=1 go build -tags db2 -o db2-copilot-extension .

# 4. Run
export DB_TYPE=db2
export DB2_CONN_STRING="HOSTNAME=myhost;PORT=50000;DATABASE=mydb;UID=user;PWD=pass;PROTOCOL=TCPIP"
./db2-copilot-extension
```

### 4. Expose Your Agent Publicly

```bash
# Using ngrok
ngrok http 8080

# Note the HTTPS URL and update FQDN in your .env and GitHub App settings
```

### 5. Install the GitHub App

Visit `https://github.com/apps/<your-app-name>` and click **Install**.

### 6. Use the Extension

In GitHub Copilot Chat, mention your extension:

```
@my-db-copilot What are the top 5 products by sales this month?
```

---

## Configuration Reference

| Variable | Required | Default | Description |
|---|---|---|---|
| `PORT` | No | `8080` | HTTP server port |
| `CLIENT_ID` | **Yes** | — | GitHub App Client ID |
| `CLIENT_SECRET` | **Yes** | — | GitHub App Client Secret |
| `FQDN` | **Yes** | — | Public base URL (no trailing slash) |
| `DB_TYPE` | No | `db2` | Database backend: `db2` or `postgres` |
| `DB2_CONN_STRING` | If `DB_TYPE=db2` | — | `HOSTNAME=...;PORT=...;DATABASE=...;UID=...;PWD=...;PROTOCOL=TCPIP` |
| `POSTGRES_CONN_STRING` | If `DB_TYPE=postgres` | — | `postgres://user:pass@host:port/db?sslmode=disable` |

---

## Running the Agent

### Development

```bash
# Load environment from .env
set -a && source .env && set +a

go run .
```

### Production Binary

```bash
# PostgreSQL build (no CGO needed)
go build -ldflags="-s -w" -o db2-copilot-extension .

# DB2 build (requires IBM DB2 CLI + CGO)
CGO_ENABLED=1 go build -tags db2 -ldflags="-s -w" -o db2-copilot-extension .

./db2-copilot-extension
```

### Health Check

```bash
curl http://localhost:8080/health
# {"status":"ok","database":"postgres"}
```

---

## Usage Examples

### PostgreSQL

```
@my-db-copilot Show me all employees in the engineering department
@my-db-copilot What are the monthly sales totals for 2024?
@my-db-copilot Which customers have not placed an order in the last 90 days?
@my-db-copilot List the top 10 products by revenue, grouped by category
```

### IBM DB2

```
@my-db-copilot List all active accounts in schema SALES
@my-db-copilot Show me the first 20 rows from the ORDERS table
@my-db-copilot What is the average order value by region?
@my-db-copilot Count distinct customers per country
```

---

## Docker & docker-compose

### PostgreSQL (full stack)

```bash
cp .env.example .env
# Set CLIENT_ID, CLIENT_SECRET, FQDN in .env
# DB_TYPE is already set to postgres in .env.example

docker-compose up --build
```

The `docker-compose.yml` starts:
- **agent**: the Copilot extension (port 8080)
- **postgres**: PostgreSQL 16 with a `sample` database (port 5432)

### DB2 Docker Build

```dockerfile
# Custom Dockerfile for DB2 (requires IBM DB2 CLI)
# Build:
docker build --build-arg BUILD_TAGS=db2 -t my-db2-agent .
```

---

## Security

### What is protected

| Threat | Mitigation |
|---|---|
| Forged requests | ECDSA signature verification against GitHub's public key |
| SQL injection | Server-side sanitizer: SELECT-only, no comments, no multi-statements |
| Data modification | All non-SELECT operations are rejected before reaching the database |
| Credential exposure | Secrets are loaded from environment variables, never hardcoded |
| Runaway queries | 30-second query timeout enforced at the driver level |

### SQL Sanitizer Rules

The `database.SanitizeSQL` function enforces:
- Must begin with `SELECT`
- Forbidden keywords: `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `CREATE`, `TRUNCATE`, `EXEC`, `EXECUTE`, `MERGE`, `GRANT`, `REVOKE`, `REPLACE`, `UPSERT`, `CALL`
- No SQL comments (`--` or `/* */`)
- No multiple statements (no `;` after trimming the trailing one)

### Recommended Database Setup

Create a **read-only database user** for this agent:

**PostgreSQL:**
```sql
CREATE USER copilot_agent WITH PASSWORD 'strong_password';
GRANT CONNECT ON DATABASE mydb TO copilot_agent;
GRANT USAGE ON SCHEMA public TO copilot_agent;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO copilot_agent;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO copilot_agent;
```

**DB2:**
```sql
GRANT CONNECT ON DATABASE TO USER copilot_agent;
GRANT SELECT ON TABLE myschema.mytable TO USER copilot_agent;
```

---

## Troubleshooting

### General

| Problem | Solution |
|---|---|
| `CLIENT_ID environment variable is required` | Set `CLIENT_ID` in your environment or `.env` |
| `signature verification failed` | Ensure your FQDN is correct and the agent URL is set properly in your GitHub App |
| Schema shows as empty | Check the database user has SELECT permission on information_schema / SYSCAT.COLUMNS |

### PostgreSQL

| Problem | Solution |
|---|---|
| `connection refused` | Ensure PostgreSQL is running: `pg_isready -h localhost` |
| `password authentication failed` | Verify `POSTGRES_CONN_STRING` credentials |
| `SSL connection required` | Add `sslmode=require` or `sslmode=disable` to your DSN |
| `pq: SSL is not enabled on the server` | Use `sslmode=disable` in your DSN |
| No tables in schema | Grant SELECT on information_schema to your DB user |

### IBM DB2

| Problem | Solution |
|---|---|
| `unknown driver "go_ibm_db"` | Build with `-tags db2`: `go build -tags db2 .` |
| `sqlcli1.h: No such file or directory` | Install IBM DB2 CLI driver and set `IBM_DB_HOME` |
| `SQL1013N` (database not found) | Check `DATABASE=` in your `DB2_CONN_STRING` |
| Connection timeout | Verify `HOSTNAME` and `PORT` in `DB2_CONN_STRING` |

---

## Adding New Database Backends

This extension uses a clean interface-based architecture. Adding support for a new database (e.g. MySQL, SQL Server, SQLite) requires implementing six methods:

```go
// database/interface.go
type Client interface {
    ExecuteQuery(ctx context.Context, query string) ([]map[string]interface{}, error)
    GetSchemaInfo() (string, error)
    FormatResults(results []map[string]interface{}) string
    Ping(ctx context.Context) error
    Close() error
    DatabaseType() string
}
```

### Step-by-step example: Adding MySQL

1. **Create `mysql/client.go`** implementing all six methods
2. **Add driver import** (`_ "github.com/go-sql-driver/mysql"`)
3. **Update `main.go`** `newDatabaseClient()` switch:
   ```go
   case "mysql":
       client, err := mysql.NewClient(cfg.MySQLConnStr)
   ```
4. **Update `config/config.go`** to add `MySQLConnStr` and validate it
5. **Update `.env.example`** and `README.md`

The SQL sanitizer in `database/sanitizer.go` works for all databases — you may extend it for dialect-specific rules if needed.

---

## License

MIT — see [LICENSE](LICENSE) for details.
