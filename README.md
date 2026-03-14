# DB2 Copilot Extension

> A GitHub Copilot Extension (agent) that connects to IBM DB2 databases, enabling users to query DB2 using natural language directly from GitHub Copilot Chat or Microsoft Teams.

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![IBM DB2](https://img.shields.io/badge/IBM-DB2-054ADA?logo=ibm)](https://www.ibm.com/products/db2)

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Prerequisites](#prerequisites)
4. [Project Structure](#project-structure)
5. [Setup and Installation](#setup-and-installation)
   - [Step 1: Clone the repository](#step-1-clone-the-repository)
   - [Step 2: Install IBM DB2 Driver](#step-2-install-ibm-db2-driver)
   - [Step 3: Create a GitHub App](#step-3-create-a-github-app)
   - [Step 4: Configure Environment Variables](#step-4-configure-environment-variables)
   - [Step 5: Start ngrok (local development)](#step-5-start-ngrok-local-development)
   - [Step 6: Run the Application](#step-6-run-the-application)
6. [Usage](#usage)
7. [Using with Microsoft Teams](#using-with-microsoft-teams)
8. [Security](#security)
9. [Configuration Reference](#configuration-reference)
10. [Development](#development)
11. [Troubleshooting](#troubleshooting)
12. [Contributing](#contributing)
13. [License](#license)

---

## Overview

The **DB2 Copilot Extension** brings natural language querying of IBM DB2 databases directly into GitHub Copilot Chat. Instead of writing SQL by hand, you simply ask questions in plain English:

```
@db2-agent What are the top 10 customers by total order value?
@db2-agent Show me all orders placed last month
@db2-agent How many employees are in each department?
```

The extension automatically:
1. Reads your DB2 schema to give the LLM context
2. Translates your question into a DB2-compatible SQL query using GitHub Copilot's LLM
3. Sanitizes the generated SQL (SELECT-only enforcement, injection prevention)
4. Executes the query against your DB2 database
5. Streams the formatted results and a plain-English explanation back to you

**Where you can use it:**
- **GitHub Copilot Chat** at [github.com/copilot](https://github.com/copilot) — mention `@your-agent-name`
- **Microsoft Teams** — via the [GitHub integration for Teams](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/coding-agent/integrate-coding-agent-with-teams)

---

## Architecture

### Request Flow

```
┌──────────────────────────────────────────────────────────────────────┐
│  User (GitHub Copilot Chat / Microsoft Teams)                        │
│                                                                      │
│  @db2-agent "show me all orders from last month"                     │
└──────────────────────────────┬───────────────────────────────────────┘
                               │  HTTPS POST
                               ▼
┌──────────────────────────────────────────────────────────────────────┐
│  GitHub Copilot Platform                                             │
│  • Authenticates the user                                            │
│  • Forwards message to agent server with JWT + signature             │
└──────────────────────────────┬───────────────────────────────────────┘
                               │  HTTPS POST /agent
                               ▼
┌──────────────────────────────────────────────────────────────────────┐
│  Agent Server  (this project)                                        │
│                                                                      │
│  1. Verify ECDSA request signature (GitHub public key)               │
│  2. Fetch DB2 schema from catalog (SYSCAT.COLUMNS) — cached          │
│  3. Build system prompt with schema context                          │
│  4. Call Copilot LLM (non-streaming) → get SQL in <sql> tags         │
│  5. Sanitize SQL (SELECT-only, no comments, no injection)            │
│  6. Execute query on DB2 with 30-second timeout                      │
│  7. Format results as markdown table                                 │
│  8. Call Copilot LLM (streaming) → explain results                   │
│  9. Stream explanation back via SSE                                  │
└───────────────────┬──────────────────────────┬───────────────────────┘
                    │  database/sql             │  HTTPS (Copilot API)
                    ▼                           ▼
         ┌──────────────────┐       ┌────────────────────────┐
         │  IBM DB2         │       │  GitHub Copilot LLM    │
         │  (TCPIP/ODBC)    │       │  (GPT-4)               │
         └──────────────────┘       └────────────────────────┘
```

### Sequence Diagram

```
User          GitHub Platform      Agent Server        DB2       Copilot LLM
 │                 │                    │               │              │
 │──"top 10 customers"──►│              │               │              │
 │                 │──POST /agent──────►│               │              │
 │                 │                    │──schema info?─►│             │
 │                 │                    │◄──schema text──│             │
 │                 │                    │──build prompt──────────────►│
 │                 │                    │◄──SQL in <sql> tags─────────│
 │                 │                    │──sanitize SQL              │
 │                 │                    │──execute query─►│           │
 │                 │                    │◄──rows──────────│           │
 │                 │                    │──format results──────────►│
 │                 │                    │◄──SSE explanation stream───│
 │◄────────SSE stream──────────────────│               │              │
```

### Component Responsibilities

| Component | File | Responsibility |
|-----------|------|----------------|
| HTTP Server | `main.go` | Routes, public key fetching, startup |
| Agent | `agent/service.go` | Orchestration, signature verification, SSE streaming |
| Copilot Client | `copilot/endpoints.go` | LLM API calls (streaming + non-streaming) |
| Copilot Types | `copilot/messages.go` | Request/response type definitions |
| DB2 Client | `db2/client.go` | Connection pool, query execution, schema introspection |
| SQL Sanitizer | `db2/sanitizer.go` | Security layer — SELECT-only enforcement |
| Configuration | `config/config.go` | Environment variable loading and validation |
| OAuth | `oauth/service.go` | GitHub App OAuth flow |

---

## Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| **Go** | 1.22+ | [download](https://go.dev/dl/) |
| **IBM DB2** | Any | Local or remote instance |
| **IBM DB2 CLI/ODBC driver** | Latest | Required for `go_ibm_db` (CGO) |
| **GitHub account** | — | Permissions to create GitHub Apps |
| **ngrok** | Latest | For local development tunneling |
| **Docker + Docker Compose** | Latest | Optional — for containerized deployment |

---

## Project Structure

```
db2-copilot-extension/
├── main.go                    # HTTP server setup, routes, public key fetching
├── go.mod                     # Go module definition
├── go.sum                     # Go module checksums
├── Dockerfile                 # Multi-stage Docker build
├── docker-compose.yml         # Docker Compose (agent + optional DB2 dev container)
├── .env.example               # Example environment variables
├── .gitignore                 # Go gitignore (excludes .env, binaries)
├── README.md                  # This file
│
├── agent/
│   └── service.go             # Core handler: signature verification, SQL orchestration, SSE streaming
│
├── copilot/
│   ├── endpoints.go           # Copilot API client (POST to chat completions)
│   └── messages.go            # Request/response types (ChatRequest, ChatMessage, etc.)
│
├── db2/
│   ├── client.go              # DB2 connection pool, ExecuteQuery, GetSchemaInfo, FormatResults
│   ├── driver_cgo.go          # CGO-gated IBM DB2 driver blank import
│   ├── sanitizer.go           # SQL security layer (SELECT-only, injection prevention)
│   └── sanitizer_test.go      # Unit tests for the SQL sanitizer
│
├── config/
│   └── config.go              # Environment variable loading and validation
│
└── oauth/
    └── service.go             # GitHub OAuth pre-auth and callback handlers
```

---

## Setup and Installation

### Step 1: Clone the repository

```bash
git clone https://github.com/iamankushpandit/db2-copilot-extension.git
cd db2-copilot-extension
```

### Step 2: Install IBM DB2 Driver

The `go_ibm_db` Go package uses CGO and requires the **IBM DB2 CLI/ODBC driver (clidriver)** to be installed on your machine.

#### Option A — macOS (Intel or Apple Silicon)

```bash
# Install the clidriver via the go_ibm_db installer
go get github.com/ibmdb/go_ibm_db
cd $(go env GOPATH)/pkg/mod/github.com/ibmdb/go_ibm_db*/installer
go run setup.go

# Set environment variables (add to ~/.zshrc or ~/.bash_profile)
export IBM_DB_HOME=$HOME/Downloads/clidriver        # adjust to your install path
export CGO_CFLAGS="-I$IBM_DB_HOME/include"
export CGO_LDFLAGS="-L$IBM_DB_HOME/lib"
export DYLD_LIBRARY_PATH=$IBM_DB_HOME/lib:$DYLD_LIBRARY_PATH
```

#### Option B — Linux (Ubuntu / Debian)

```bash
# Install build dependencies
sudo apt-get install -y gcc libxml2-dev

# Download and run the go_ibm_db installer
go get github.com/ibmdb/go_ibm_db
cd $(go env GOPATH)/pkg/mod/github.com/ibmdb/go_ibm_db*/installer
go run setup.go

# Set environment variables (add to ~/.bashrc)
export IBM_DB_HOME=$HOME/Downloads/clidriver        # adjust to your install path
export CGO_CFLAGS="-I$IBM_DB_HOME/include"
export CGO_LDFLAGS="-L$IBM_DB_HOME/lib"
export LD_LIBRARY_PATH=$IBM_DB_HOME/lib:$LD_LIBRARY_PATH
```

#### Option C — Using Docker (recommended for quick start)

Skip this step entirely and use the Docker Compose setup in [Step 6](#step-6-run-the-application) — the `Dockerfile` handles the clidriver installation inside the container.

### Step 3: Create a GitHub App

1. Go to **GitHub → Settings → Developer Settings → GitHub Apps → New GitHub App**

2. Fill in the basic details:
   - **GitHub App name**: `db2-agent` (or any name you prefer)
   - **Homepage URL**: Your public URL (e.g., `https://your-ngrok-url.ngrok.app`)
   - **Callback URL**: `https://your-ngrok-url.ngrok.app/auth/callback`
   - Uncheck **"Expire user authorization tokens"** if you want persistent sessions

3. Set **Permissions**:
   - Under **Account Permissions** → **Copilot Chat** → **Access: Read Only**

4. Uncheck **"Active"** under Webhooks (not needed for this extension)

5. Click **Create GitHub App** and note down the **Client ID**

6. Scroll down and click **Generate a new client secret** — save the secret securely

7. Click the **Copilot** tab in your app settings and fill in:
   - **Agent URL**: `https://your-ngrok-url.ngrok.app/agent`
   - **Pre-Authorization URL**: `https://your-ngrok-url.ngrok.app/auth/authorization`
   - **Description**: `Query IBM DB2 databases using natural language`
   - Click **Save**

8. Install the app: go to `https://github.com/apps/<your-app-name>` and click **Install**

### Step 4: Configure Environment Variables

```bash
cp .env.example .env
```

Edit `.env` with your actual values:

```dotenv
PORT=8080
CLIENT_ID=Iv1.your_github_app_client_id
CLIENT_SECRET=your_github_app_client_secret_here
FQDN=https://abc123.ngrok.app
DB2_CONN_STRING=HOSTNAME=localhost;PORT=50000;DATABASE=sample;UID=db2inst1;PWD=password;PROTOCOL=TCPIP
```

### Step 5: Start ngrok (local development)

```bash
ngrok http http://localhost:8080
```

Copy the **Forwarding** address (e.g., `https://abc123.ngrok.app`) and:
- Update `FQDN` in your `.env` file
- Update the **Agent URL**, **Callback URL**, and **Homepage URL** in your GitHub App settings

### Step 6: Run the Application

#### Option A — Direct Go run

```bash
# Install dependencies
go mod tidy

# Run the server
go run .
```

You should see:
```
INFO fetched GitHub Copilot public key
INFO DB2 client initialised
INFO starting server on :8080
```

#### Option B — Docker Compose

```bash
# Start the agent + an optional local DB2 Community Edition instance
docker-compose up --build
```

> **Note**: The first startup of the DB2 container takes 2–5 minutes as it initializes the database.

To use only the agent (connecting to an external DB2):

```bash
docker-compose up --build agent
```

---

## Usage

Once deployed and installed, interact with the agent in GitHub Copilot Chat by mentioning `@your-app-name`:

### Example Prompts

| Prompt | What it does |
|--------|-------------|
| `@db2-agent show me all tables in the database` | Lists all user-defined tables with column counts |
| `@db2-agent what are the top 10 customers by total revenue?` | Generates and runs a GROUP BY + ORDER BY query |
| `@db2-agent how many orders were placed last month?` | Queries with a date range filter |
| `@db2-agent describe the EMPLOYEES table` | Returns column names, types, and lengths |
| `@db2-agent what is the average salary by department?` | Aggregation query with GROUP BY |
| `@db2-agent show me orders that are still pending` | Filters by status column |

### Example Conversation

```
You: @db2-agent What are the top 5 departments by headcount?

🤖 db2-agent:
I'll query the database to find the departments with the most employees.

**SQL executed:**
```sql
SELECT WORKDEPT, COUNT(*) AS HEADCOUNT
FROM EMPLOYEE
GROUP BY WORKDEPT
ORDER BY HEADCOUNT DESC
FETCH FIRST 5 ROWS ONLY
```

**Results:**

| WORKDEPT | HEADCOUNT |
|----------|-----------|
| A00      | 5         |
| B01      | 1         |
| C01      | 3         |
| D01      | 2         |
| D11      | 9         |

**Department D11** has the largest team with **9 employees**, followed by **Department A00** with 5.
```

---

## Using with Microsoft Teams

You can use this extension from Microsoft Teams via the GitHub integration:

### Setup

1. Install the **GitHub app for Teams** from the [Microsoft Teams App Store](https://teams.microsoft.com/l/app/836ecc9e-6dca-4696-a2e9-15e252cd3f31)
2. Connect your GitHub account in Teams
3. Ensure your GitHub account has the DB2 Copilot Extension installed

### Usage in Teams

In any Teams chat or channel, mention `@GitHub` followed by `@db2-agent`:

```
@GitHub @db2-agent show me the latest 10 orders
```

### Current Limitations

- The Teams integration is designed primarily for the **GitHub Copilot coding agent** workflow
- For the best experience with custom extensions like this one, use GitHub Copilot Chat directly at [github.com/copilot](https://github.com/copilot)
- Real-time streaming may behave differently in Teams compared to GitHub Chat

---

## Security

### SQL Sanitization

The `db2/sanitizer.go` enforces a strict allowlist policy on all generated SQL:

- ✅ **Only `SELECT` statements are permitted**
- ❌ Blocked: `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `CREATE`, `TRUNCATE`, `EXEC`, `MERGE`, `GRANT`, `REVOKE`
- ❌ SQL comments are blocked (`--` and `/* */`) to prevent comment-based injection
- ❌ Multiple statements (`;` separated) are rejected

### Request Signature Verification

Every incoming request is verified to originate from GitHub using **ECDSA with SHA-256**:

- The signature is read from the `Github-Public-Key-Signature` header
- The payload hash is verified against GitHub's current public key (fetched at startup from `https://api.github.com/meta/public_keys/copilot_api`)
- Requests with missing or invalid signatures receive a `401 Unauthorized` response

### Other Security Practices

- 🔒 **Read-only DB2 credentials** — Use a DB2 user with `SELECT` privileges only
- 🔒 **Query timeout** — All DB2 queries have a 30-second hard timeout
- 🔒 **Schema caching** — Schema info is cached in memory, not exposed in API responses
- 🔒 **No secrets in logs** — API tokens and DB2 credentials are never logged
- 🔒 **Environment variables** — All secrets are loaded from environment (`.env` is gitignored)
- 🔒 **Non-root Docker container** — The runtime container runs as `appuser` (UID 1001)

### Read-Only Database User Setup

For maximum safety, create a dedicated read-only database user for the connector. The connector enforces read-only access in code, but using a read-only DB user provides an additional layer of protection.

#### PostgreSQL

```sql
-- Create a read-only role for the connector
CREATE ROLE db_copilot_readonly;

-- Grant CONNECT on the target database
GRANT CONNECT ON DATABASE your_database TO db_copilot_readonly;

-- Grant USAGE on the schemas you want to expose
GRANT USAGE ON SCHEMA public TO db_copilot_readonly;
GRANT USAGE ON SCHEMA analytics TO db_copilot_readonly;

-- Grant SELECT on existing tables
GRANT SELECT ON ALL TABLES IN SCHEMA public TO db_copilot_readonly;
GRANT SELECT ON ALL TABLES IN SCHEMA analytics TO db_copilot_readonly;

-- Automatically grant SELECT on future tables
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT ON TABLES TO db_copilot_readonly;
ALTER DEFAULT PRIVILEGES IN SCHEMA analytics
  GRANT SELECT ON TABLES TO db_copilot_readonly;

-- Create the user and assign the role
CREATE USER db_copilot WITH PASSWORD 'your-secure-password';
GRANT db_copilot_readonly TO db_copilot;

-- Enforce read-only at the session level (belt-and-suspenders)
ALTER USER db_copilot SET default_transaction_read_only = on;
```

Connect using: `DATABASE_URL=postgres://db_copilot:your-secure-password@localhost:5432/your_database`

#### IBM DB2

```sql
-- Create the read-only user at the OS level first (DB2 uses OS authentication)
-- On Linux: sudo useradd -m db_copilot && sudo passwd db_copilot

-- Connect to the database as an admin
CONNECT TO your_database USER admin USING admin_password;

-- Grant CONNECT privilege
GRANT CONNECT ON DATABASE TO USER db_copilot;

-- Grant SELECT on the schemas you want to expose
GRANT SELECT ON TABLE schema1.customers TO USER db_copilot;
GRANT SELECT ON TABLE schema1.orders TO USER db_copilot;
-- Or grant SELECT on all tables in a schema:
-- GRANT SELECT ON ALL TABLES IN SCHEMA public TO USER db_copilot;

-- Grant access to the catalog views (needed for schema discovery)
GRANT SELECT ON SYSIBM.SYSTABLES TO USER db_copilot;
GRANT SELECT ON SYSCAT.TABLES TO USER db_copilot;
GRANT SELECT ON SYSCAT.COLUMNS TO USER db_copilot;
GRANT SELECT ON SYSCAT.REFERENCES TO USER db_copilot;
GRANT SELECT ON SYSCAT.TABCOMMENTS TO USER db_copilot;
GRANT SELECT ON SYSCAT.COLCOMMENTS TO USER db_copilot;
```

Connect using: `DB2_CONN_STRING=HOSTNAME=localhost;PORT=50000;DATABASE=your_database;UID=db_copilot;PWD=your-secure-password;PROTOCOL=TCPIP`

> **Important:** Do not grant `INSERT`, `UPDATE`, `DELETE`, `ALTER`, or `DROP` privileges to the connector user. The connector will still refuse to execute write queries in code, but using a read-only user prevents accidental or malicious writes at the database level.

---

## Admin UI

The connector includes a built-in admin interface at `/admin` for managing schema access, query limits, and LLM configuration.

### Accessing the Admin UI

1. Set at least one admin user in `config/admin_config.json`:
   ```json
   {
     "admin_ui": {
       "allowed_github_users": ["your-github-username"]
     }
   }
   ```
2. Navigate to `https://your-fqdn/admin`
3. Sign in with your GitHub account
4. Only users listed in `allowed_github_users` can access the admin panel

### Admin Pages

| Page | Path | Description |
|------|------|-------------|
| Dashboard | `/admin` | System health overview |
| Schema Access | `/admin/schema` | Approve/deny schemas, tables, and columns |
| Query Safety | `/admin/safety` | Row limits, cost thresholds, rate limits |
| LLM Configuration | `/admin/llm` | SQL generation and explanation providers |
| Business Glossary | `/admin/glossary` | Add domain-specific term definitions |
| Database Settings | `/admin/database` | Connection status and configuration |

### Admin Session Security

- Sessions are signed with HMAC-SHA256 using the `ADMIN_SESSION_SECRET` environment variable
- If `ADMIN_SESSION_SECRET` is not set, a random secret is generated at startup (sessions will not survive restarts)
- For production deployments, always set `ADMIN_SESSION_SECRET` to a stable random value:
  ```bash
  ADMIN_SESSION_SECRET=$(openssl rand -hex 32)
  ```

---

## Configuration Reference

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | Port the HTTP server listens on | `8080` | No |
| `CLIENT_ID` | GitHub App Client ID | — | **Yes** |
| `CLIENT_SECRET` | GitHub App Client Secret | — | **Yes** |
| `FQDN` | Public URL of the agent (no trailing slash) | — | **Yes** |
| `DB2_CONN_STRING` | DB2 ODBC connection string | — | Yes (DB2) |
| `DATABASE_URL` | PostgreSQL connection string | — | Yes (PostgreSQL) |
| `CONFIG_DIR` | Directory containing JSON config files | `config` | No |
| `ADMIN_SESSION_SECRET` | Secret for signing admin session cookies | random | No (recommended) |

### DB2 Connection String Format

```
HOSTNAME=<host>;PORT=<port>;DATABASE=<dbname>;UID=<user>;PWD=<password>;PROTOCOL=TCPIP
```

| Component | Description | Example |
|-----------|-------------|---------|
| `HOSTNAME` | DB2 server IP or hostname | `db2.example.com` |
| `PORT` | DB2 server port | `50000` |
| `DATABASE` | Database name | `SAMPLE` |
| `UID` | DB2 username | `db2readonly` |
| `PWD` | DB2 password | `s3cr3t` |
| `PROTOCOL` | Always `TCPIP` for network | `TCPIP` |

---

## Development

### Running Tests

```bash
# Run all tests that don't require the IBM DB2 driver
CGO_ENABLED=0 go test ./... -v

# Run the SQL sanitizer tests specifically
CGO_ENABLED=0 go test ./db2/... -run TestSanitizeSQL -v

# Run all tests with CGO (requires IBM clidriver installed)
go test ./... -v
```

### Code Walkthrough

**Request lifecycle:**

1. `main.go` → Fetches the GitHub public key and sets up routes
2. `POST /agent` → `agent.Service.ChatCompletion()`
3. Signature is verified with `validPayload()` using ECDSA
4. Request body is decoded into `copilot.ChatRequest`
5. `generateCompletion()` is called:
   - DB2 schema fetched (cached via `sync.Once`)
   - System prompt built with schema context
   - Non-streaming call to Copilot LLM to get SQL
   - SQL extracted from `<sql>...</sql>` tags
   - SQL sanitized by `db2.SanitizeSQL()`
   - SQL executed on DB2 with 30s timeout
   - Results formatted as markdown table
   - Streaming call to Copilot LLM to explain results
   - SSE stream proxied back to the client

### Adding New Features

**Adding a new route:**
```go
// main.go
mux.HandleFunc("GET /my-route", myHandler)
```

**Modifying the system prompt:**
Edit the `systemPrompt` constant in `agent/service.go`.

**Extending SQL validation:**
Add keywords to the `blockedKeywords` slice in `db2/sanitizer.go`.

**Adding connection pool tuning:**
Modify the constants in `db2/client.go`:
```go
const (
    maxOpenConns    = 10
    maxIdleConns    = 5
    connMaxLifetime = 5 * time.Minute
    queryTimeout    = 30 * time.Second
)
```

---

## Troubleshooting

### IBM DB2 driver fails to build

**Symptom:** `undefined: SQLSMALLINT` or similar CGO errors

**Solution:**
1. Ensure the IBM DB2 clidriver is installed (see [Step 2](#step-2-install-ibm-db2-driver))
2. Verify environment variables are set:
   ```bash
   echo $IBM_DB_HOME
   echo $LD_LIBRARY_PATH   # Linux
   echo $DYLD_LIBRARY_PATH # macOS
   ```
3. Try running `go run setup.go` in the `go_ibm_db/installer` directory

### DB2 connection fails

**Symptom:** `failed to connect to DB2` or `ping failed`

**Solutions:**
- Verify the connection string format: `HOSTNAME=...;PORT=...;DATABASE=...;UID=...;PWD=...;PROTOCOL=TCPIP`
- Check that the DB2 server is running and the port is open: `telnet <hostname> 50000`
- Confirm the DB2 user has connect permissions: `CONNECT TO <db> USER <uid> USING <pwd>`
- For Docker Compose, wait for the DB2 health check to pass (can take 2–5 minutes)

### ngrok URL not working

**Symptom:** `404` or `502` errors from GitHub when calling the agent

**Solutions:**
1. Ensure ngrok is running: `ngrok http http://localhost:8080`
2. Update `FQDN` in `.env` with the new ngrok URL (it changes on restart unless you have a paid plan)
3. Update the **Agent URL** and **Callback URL** in the GitHub App settings
4. Restart the Go server after changing `.env`

### Signature verification fails

**Symptom:** `401 Unauthorized` — invalid signature

**Solutions:**
- This usually means the request is not coming from GitHub (e.g., a direct `curl` test)
- For local testing, set `GITHUB_COPILOT_DISABLE_SIGNATURE_CHECK=true` in your `.env` (add this feature if needed)
- Ensure your server clock is synchronized (NTP)

### No SQL generated / agent answers without running a query

**Behavior:** The agent responds conversationally without executing SQL

**This is expected** when:
- The question doesn't require a database query (e.g., "how are you?")
- The LLM determines no query is needed

If SQL should have been generated, try rephrasing the question to be more specific about the data you need.

### Schema not loading

**Symptom:** The LLM doesn't seem to know about your tables

**Solutions:**
- The schema is loaded from `SYSCAT.COLUMNS` — verify your DB2 user has `SELECT` on this catalog
- Schema is cached in memory; restart the server after schema changes
- Check server logs for `WARN could not fetch schema info`

---

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes with tests where applicable
4. Run `CGO_ENABLED=0 go test ./...` to verify non-CGO tests pass
5. Run `gofmt -w .` to format your code
6. Submit a pull request with a clear description

---

## License

This project is licensed under the **MIT License**.

```
MIT License

Copyright (c) 2024 Ankush Pandit

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```
