# DB Copilot Connector

> A production-grade GitHub Copilot Extension that connects to **IBM DB2** or **PostgreSQL** databases, allowing users to ask natural language questions and get SQL-powered answers — directly from GitHub Copilot Chat or Microsoft Teams.

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-supported-336791?logo=postgresql)](https://www.postgresql.org)
[![IBM DB2](https://img.shields.io/badge/IBM-DB2-054ADA?logo=ibm)](https://www.ibm.com/products/db2)

---

## Table of Contents

1. [Overview](#overview)
2. [Key Features](#key-features)
3. [Architecture](#architecture)
4. [Prerequisites](#prerequisites)
5. [Project Structure](#project-structure)
6. [Setup and Installation](#setup-and-installation)
   - [Step 1: Clone the repository](#step-1-clone-the-repository)
   - [Step 2: Install database driver](#step-2-install-database-driver)
   - [Step 3: Create a GitHub App](#step-3-create-a-github-app)
   - [Step 4: Configure environment variables](#step-4-configure-environment-variables)
   - [Step 5: Start ngrok (local development)](#step-5-start-ngrok-local-development)
   - [Step 6: Run the application](#step-6-run-the-application)
7. [Configuration](#configuration)
   - [Environment variables](#environment-variables)
   - [JSON config files](#json-config-files)
8. [Usage](#usage)
9. [Using with Microsoft Teams](#using-with-microsoft-teams)
10. [Admin UI](#admin-ui)
11. [Security](#security)
    - [Read-only database user setup (PostgreSQL)](#postgresql)
    - [Read-only database user setup (IBM DB2)](#ibm-db2)
12. [Health Monitoring](#health-monitoring)
13. [Audit Logging](#audit-logging)
14. [Development](#development)
15. [Troubleshooting](#troubleshooting)
16. [Contributing](#contributing)
17. [License](#license)

---

## Overview

The **DB Copilot Connector** brings natural language querying of databases directly into GitHub Copilot Chat. Instead of writing SQL by hand, you simply ask questions in plain English:

```
@db-copilot What are the top 10 customers by total order value?
@db-copilot Show me all orders placed last month
@db-copilot How many employees are in each department?
```

The connector automatically:

1. Discovers your database schema (with two-tier awareness — full internal view, admin-approved external view)
2. Translates your question into SQL using a configurable LLM provider (Ollama, GitHub Copilot, or any OpenAI-compatible API)
3. Validates the generated SQL against the approved schema and sanitizes it (SELECT-only enforcement)
4. Checks query cost via `EXPLAIN` to prevent expensive queries
5. Executes the query with injected row limits
6. Detects the result shape and computes summary statistics
7. Explains the results in plain English via a second LLM call
8. Streams everything back to the user via SSE with real-time status messages

If the first query fails, the connector self-corrects with contextual error information and retries once — then saves the correction for future queries.

**Supported databases:** PostgreSQL, IBM DB2 (one database per deployment)

**Supported LLM providers:** Ollama (local), GitHub Copilot, OpenAI-compatible APIs — with automatic fallback

**Where you can use it:**
- **GitHub Copilot Chat** at [github.com/copilot](https://github.com/copilot) — mention `@your-agent-name`
- **Microsoft Teams** — via the [GitHub integration for Teams](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/coding-agent/integrate-coding-agent-with-teams)

---

## Key Features

| Feature | Description |
|---------|-------------|
| **Dual database support** | PostgreSQL and IBM DB2 via a unified `database.Client` interface |
| **Multi-LLM providers** | Ollama, GitHub Copilot, and OpenAI-compatible APIs with configurable fallback |
| **Two-tier schema awareness** | Full internal awareness (Tier 1) for recommendations; admin-approved subset (Tier 2) exposed to the LLM |
| **Post-generation SQL validation** | Every generated query is verified against the approved schema before execution |
| **EXPLAIN cost estimation** | Queries exceeding configurable row/cost thresholds are rejected with suggestions |
| **3-tier result limiting** | Database-level LIMIT injection → display-row cap → summary statistics from all rows |
| **Self-correction loop** | On SQL errors, the connector retries once with error context and column hints |
| **Learned corrections** | Successful corrections are saved and included in future prompts (rolling window with LRU eviction) |
| **Business glossary** | Admin-defined term mappings (e.g., "revenue" → `SUM(orders.total_amount)`) injected into prompts |
| **Real-time status streaming** | SSE status messages during pipeline execution (searching schema, generating SQL, running query…) |
| **Rate limiting** | Per-user and global request rate limits |
| **Admin UI** | Built-in web interface for schema access control, query safety, LLM config, and glossary management |
| **Hot-reload configuration** | All 5 JSON config files are watched for changes and reloaded automatically |
| **Audit logging** | Append-only JSONL audit trail with daily rotation — every query, validation, and error is logged |
| **Health monitoring** | Periodic background checks on database, LLM, and schema freshness; detailed `/health` endpoint |
| **Graceful shutdown** | Waits up to 30 seconds for in-flight requests on SIGTERM/SIGINT |
| **Request signature verification** | ECDSA verification of GitHub-signed payloads |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        DB Copilot Connector                              │
│                                                                          │
│  HTTP Endpoints:                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌─────────────┐ │
│  │ Admin UI     │  │ Agent        │  │ Health       │  │ OAuth       │ │
│  │ /admin/*     │  │ POST /agent  │  │ GET /health  │  │ /auth/*     │ │
│  └──────┬───────┘  └──────┬───────┘  └──────────────┘  └─────────────┘ │
│         │                  │                                             │
│         ▼                  ▼                                             │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    CORE PIPELINE                                 │    │
│  │                                                                  │    │
│  │  User Question                                                   │    │
│  │    → Rate Limit Check                                            │    │
│  │    → Build Approved Schema Context (Tier 2 only)                 │    │
│  │    → Include Business Glossary + Learned Corrections             │    │
│  │    → LLM 1: Text-to-SQL (Ollama / Copilot / OpenAI-compatible) │    │
│  │    → Post-Generation Validation (all tables/columns approved?)   │    │
│  │    → EXPLAIN Cost Check (reject if too expensive)                │    │
│  │    → Execute Query (with injected LIMIT safety)                  │    │
│  │    → Result Limiting (3-tier: DB cap → display → summary)       │    │
│  │    → Detect Result Shape (scalar/single/small/large)            │    │
│  │    → LLM 2: Explain Results (Copilot / Ollama / OpenAI-compat) │    │
│  │    → Stream Response to User via SSE                             │    │
│  │                                                                  │    │
│  │  On Failure:                                                     │    │
│  │    → Self-Correction Loop (max 1 retry with error context)       │    │
│  │    → If still fails → human-readable error with suggestion       │    │
│  │                                                                  │    │
│  │  On Table Not Approved:                                          │    │
│  │    → Check full awareness (Tier 1)                               │    │
│  │    → If table exists but not approved → recommend to user        │    │
│  │    → If table is in hidden list → say nothing about it           │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                  │
│  │ Audit Logger │  │ Config Mgr   │  │ Health Check │                  │
│  │ (JSONL daily)│  │ (hot-reload) │  │ (background) │                  │
│  └──────────────┘  └──────────────┘  └──────────────┘                  │
└─────────────────────────────────────────────────────────────────────────┘
         │                                          │
         ▼                                          ▼
┌──────────────────┐                    ┌────────────────────────┐
│  PostgreSQL      │                    │  LLM Provider          │
│  — or —          │                    │  (Ollama / Copilot /   │
│  IBM DB2         │                    │   OpenAI-compatible)   │
└──────────────────┘                    └────────────────────────┘
```

### Startup Sequence

1. Load all config files from `CONFIG_DIR`. Validate JSON. If invalid → refuse to start.
2. Connect to database. `SELECT 1` ping. If fails → refuse to start.
3. Verify read-only: check user privileges and `default_transaction_read_only` (PostgreSQL). If write perms detected → log verbose warning with liability disclaimer, continue.
4. Fetch GitHub Copilot public key for request signature verification.
5. Check LLM provider. If Ollama: ping server, check model availability.
6. Crawl full schema (Tier 1). Build approved subset (Tier 2). Cache both.
7. Load learned corrections from `learned_corrections.jsonl`.
8. Start background health checker and schema auto-refresh.
9. Start HTTP server. Log ready message with full status summary.

### Graceful Shutdown

On SIGTERM/SIGINT:
- Stop accepting new requests
- Wait for in-flight requests to complete (up to 30 seconds)
- Flush audit log buffer
- Close database connection pool
- Log shutdown summary

---

## Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| **Go** | 1.22+ | [download](https://go.dev/dl/) |
| **PostgreSQL** | 12+ | *or* IBM DB2 — one database per deployment |
| **IBM DB2** | Any | Required only if using DB2 (needs CGO + clidriver) |
| **GitHub account** | — | Permissions to create GitHub Apps |
| **ngrok** | Latest | For local development tunneling |
| **Docker + Docker Compose** | Latest | Optional — for containerized deployment |
| **Ollama** | Latest | Optional — for local LLM inference ([ollama.com](https://ollama.com)) |

---

## Project Structure

```
db2-copilot-extension/
├── main.go                        # HTTP server, startup sequence, graceful shutdown
├── go.mod / go.sum                # Go module definition and checksums
├── Dockerfile                     # Multi-stage Docker build (CGO + DB2 clidriver)
├── docker-compose.yml             # Docker Compose (agent + optional DB2 dev container)
├── .env.example                   # Example environment variables
├── .gitignore                     # Excludes .env, binaries, runtime data
├── README.md                      # This file
├── prompt.md                      # Full build specification
│
├── agent/
│   └── service.go                 # Copilot agent handler: signature verification, pipeline dispatch
│
├── config/
│   ├── config.go                  # Bootstrap config from environment variables
│   ├── loader.go                  # Config file loading, validation, hot-reload with fsnotify
│   ├── access.go                  # AccessConfig types and helpers
│   ├── safety.go                  # SafetyConfig types (limits, cost, rate, health, audit)
│   ├── llm.go                     # LLMConfig types (provider selection, model settings)
│   ├── glossary.go                # GlossaryConfig types (business term definitions)
│   ├── admin.go                   # AdminConfig types (UI settings, database type)
│   └── config_test.go             # Config validation tests
│
├── database/
│   ├── interface.go               # database.Client interface (Ping, ExecuteQuery, CrawlSchema, etc.)
│   ├── sanitizer.go               # SQL sanitizer (SELECT-only enforcement)
│   ├── validator.go               # Post-generation SQL validation against approved schema
│   ├── limiter.go                 # LIMIT injection and result row limiting
│   ├── stats.go                   # Summary statistics computation (numeric, categorical)
│   ├── sanitizer_test.go          # Sanitizer tests
│   ├── validator_test.go          # Validator tests
│   ├── limiter_test.go            # Limiter tests
│   └── stats_test.go              # Stats tests
│
├── postgres/
│   └── client.go                  # PostgreSQL database.Client implementation
│
├── db2/
│   ├── client.go                  # IBM DB2 database.Client implementation
│   ├── driver_cgo.go              # CGO-gated DB2 driver import (build tag)
│   ├── sanitizer.go               # DB2-specific SQL sanitizer
│   └── sanitizer_test.go          # DB2 sanitizer tests
│
├── schema/
│   ├── crawler.go                 # Full schema discovery (Tier 1) with background auto-refresh
│   ├── filter.go                  # Approved schema filtering (Tier 2)
│   └── recommender.go             # Schema recommendation engine (suggest unapproved tables)
│
├── llm/
│   ├── interface.go               # TextToSQLProvider + ExplanationProvider interfaces
│   ├── ollama.go                  # Ollama implementation
│   ├── copilot.go                 # GitHub Copilot implementation
│   ├── openai_compat.go           # OpenAI-compatible API implementation
│   ├── fallback.go                # Fallback wrapper (primary → secondary)
│   ├── prompts.go                 # Prompt templates for SQL generation and explanation
│   └── openai_compat_test.go      # OpenAI-compatible provider tests
│
├── pipeline/
│   ├── executor.go                # Core pipeline orchestrator (the full query lifecycle)
│   ├── learning.go                # Learned corrections store (rolling window, LRU eviction)
│   ├── presenter.go               # Result shape detection and response formatting
│   ├── ratelimit.go               # Per-user and global rate limiting
│   ├── health.go                  # Periodic health checks and /health endpoint
│   ├── presenter_test.go          # Presenter tests
│   ├── ratelimit_test.go          # Rate limiter tests
│   └── health_test.go             # Health checker tests
│
├── audit/
│   ├── logger.go                  # Append-only JSONL audit logger with daily rotation
│   └── events.go                  # Audit event type definitions
│
├── admin/
│   ├── handler.go                 # Admin UI HTTP handlers
│   ├── auth.go                    # Admin authentication (GitHub OAuth)
│   ├── auth_test.go               # Admin auth tests
│   └── static/                    # Embedded HTML/JS/CSS for admin UI
│       ├── base.html              # Base layout template
│       ├── index.html             # Dashboard
│       ├── schema.html            # Schema access control
│       ├── safety.html            # Query safety settings
│       ├── llm.html               # LLM provider configuration
│       ├── glossary.html          # Business glossary
│       ├── database.html          # Database settings
│       └── login.html             # Login page
│
├── copilot/
│   ├── endpoints.go               # Copilot API client (existing)
│   └── messages.go                # Copilot message types (existing)
│
├── oauth/
│   └── service.go                 # OAuth service (existing)
│
├── config/                        # Runtime config files (auto-created on first run)
│   ├── access_config.json         # Schema access control
│   ├── query_safety.json          # Row limits, cost thresholds, rate limits
│   ├── llm_config.json            # LLM provider selection and settings
│   ├── glossary.json              # Business term definitions
│   └── admin_config.json          # Admin UI settings and database type
│
└── data/                          # Runtime data (auto-created)
    ├── audit/
    │   └── audit-YYYY-MM-DD.jsonl # Daily audit log files
    └── learned_corrections.jsonl  # Saved query corrections
```

---

## Setup and Installation

### Step 1: Clone the repository

```bash
git clone https://github.com/iamankushpandit/db2-copilot-extension.git
cd db2-copilot-extension
```

### Step 2: Install database driver

#### PostgreSQL (recommended for quick start)

No extra driver installation needed — the `lib/pq` driver is pure Go.

```bash
go mod tidy
```

#### IBM DB2

The `go_ibm_db` Go package uses CGO and requires the **IBM DB2 CLI/ODBC driver (clidriver)**.

<details>
<summary><strong>macOS (Intel or Apple Silicon)</strong></summary>

```bash
go get github.com/ibmdb/go_ibm_db
cd $(go env GOPATH)/pkg/mod/github.com/ibmdb/go_ibm_db*/installer
go run setup.go

# Set environment variables (add to ~/.zshrc or ~/.bash_profile)
export IBM_DB_HOME=$HOME/Downloads/clidriver
export CGO_CFLAGS="-I$IBM_DB_HOME/include"
export CGO_LDFLAGS="-L$IBM_DB_HOME/lib"
export DYLD_LIBRARY_PATH=$IBM_DB_HOME/lib:$DYLD_LIBRARY_PATH
```
</details>

<details>
<summary><strong>Linux (Ubuntu / Debian)</strong></summary>

```bash
sudo apt-get install -y gcc libxml2-dev
go get github.com/ibmdb/go_ibm_db
cd $(go env GOPATH)/pkg/mod/github.com/ibmdb/go_ibm_db*/installer
go run setup.go

# Set environment variables (add to ~/.bashrc)
export IBM_DB_HOME=$HOME/Downloads/clidriver
export CGO_CFLAGS="-I$IBM_DB_HOME/include"
export CGO_LDFLAGS="-L$IBM_DB_HOME/lib"
export LD_LIBRARY_PATH=$IBM_DB_HOME/lib:$LD_LIBRARY_PATH
```
</details>

<details>
<summary><strong>Docker (recommended)</strong></summary>

Skip the driver installation entirely — the `Dockerfile` handles the clidriver installation inside the container. See [Step 6](#step-6-run-the-application).
</details>

### Step 3: Create a GitHub App

1. Go to **GitHub → Settings → Developer Settings → GitHub Apps → New GitHub App**

2. Fill in the basic details:
   - **GitHub App name**: `db-copilot` (or any name you prefer)
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
   - **Description**: `Query databases using natural language`
   - Click **Save**

8. Install the app: go to `https://github.com/apps/<your-app-name>` and click **Install**

### Step 4: Configure environment variables

```bash
cp .env.example .env
```

Edit `.env` with your actual values:

```dotenv
PORT=8080
CLIENT_ID=Iv1.your_github_app_client_id
CLIENT_SECRET=your_github_app_client_secret_here
FQDN=https://abc123.ngrok.app

# For PostgreSQL:
DATABASE_URL=postgres://db_copilot:password@localhost:5432/mydb?sslmode=require

# For IBM DB2 (set database.type to "db2" in config/admin_config.json):
# DB2_CONN_STRING=HOSTNAME=localhost;PORT=50000;DATABASE=sample;UID=db2inst1;PWD=password;PROTOCOL=TCPIP
```

Set the database type in `config/admin_config.json` (auto-created on first run):

```json
{
  "database": {
    "type": "postgres"
  }
}
```

### Step 5: Start ngrok (local development)

```bash
ngrok http http://localhost:8080
```

Copy the **Forwarding** address (e.g., `https://abc123.ngrok.app`) and:
- Update `FQDN` in your `.env` file
- Update the **Agent URL**, **Callback URL**, and **Homepage URL** in your GitHub App settings

### Step 6: Run the application

#### Option A — Direct Go run

```bash
go mod tidy
go run .
```

You should see:
```
INFO config loaded from config
INFO connected to postgres database
INFO read-only status verified
INFO fetched GitHub Copilot public key
INFO schema crawled: 3 schemas, 12 tables, 87 columns in 245ms
INFO learned corrections loaded
INFO health checker started
INFO server ready on :8080 (db=postgres, llm=ollama/sqlcoder:7b)
```

#### Option B — Docker Compose

```bash
docker-compose up --build
```

> **Note**: The first startup of the DB2 container takes 2–5 minutes as it initializes the database.

To use only the agent (connecting to an external database):

```bash
docker-compose up --build agent
```

---

## Configuration

### Environment variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | Port the HTTP server listens on | `8080` | No |
| `CLIENT_ID` | GitHub App Client ID | — | **Yes** |
| `CLIENT_SECRET` | GitHub App Client Secret | — | **Yes** |
| `FQDN` | Public URL of the agent (no trailing slash) | — | **Yes** |
| `DATABASE_URL` | PostgreSQL connection string | — | Yes (PostgreSQL) |
| `DB2_CONN_STRING` | DB2 ODBC connection string | — | Yes (DB2) |
| `CONFIG_DIR` | Directory containing JSON config files | `config` | No |
| `ADMIN_SESSION_SECRET` | Secret for signing admin session cookies | random | No (recommended) |

#### DB2 connection string format

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

### JSON config files

All configuration lives in JSON files inside `CONFIG_DIR` (default: `config/`). Files are auto-created with defaults on first run. Changes are picked up automatically via file watcher with 2-second debounce — no restart required.

#### 1. `access_config.json` — Schema Access Control

Controls which schemas, tables, and columns the agent can query. The agent is internally aware of all schemas (Tier 1) but only exposes approved ones to the LLM (Tier 2).

```json
{
  "version": "1.0",
  "approved_schemas": [
    {
      "schema": "public",
      "access_level": "partial",
      "approved_tables": [
        {
          "table": "customers",
          "access_level": "partial",
          "approved_columns": ["id", "name", "email", "company", "region", "created_at", "status"],
          "denied_columns": ["ssn", "credit_card"],
          "denied_reason": "PII columns excluded per compliance policy"
        },
        {
          "table": "orders",
          "access_level": "full"
        }
      ]
    }
  ],
  "hidden_schemas": [
    {
      "schema": "security",
      "reason": "Auth infrastructure - do not recommend to users"
    }
  ]
}
```

Access levels:
- `"full"` on a schema = all tables and all columns approved (including future ones)
- `"partial"` on a schema = only explicitly listed tables approved
- `"full"` on a table = all columns approved
- `"partial"` on a table = only `approved_columns` are queryable; `denied_columns` are invisible to the LLM

#### 2. `query_safety.json` — Limits and Safety

```json
{
  "query_limits": {
    "max_query_rows": 100,
    "display_rows": 10,
    "compute_summary_stats": true,
    "enforce_limit": true,
    "query_timeout_seconds": 30
  },
  "cost_estimation": {
    "explain_before_execute": true,
    "max_estimated_rows": 100000,
    "max_estimated_cost": 50000,
    "on_exceed": "reject_and_suggest"
  },
  "self_correction": {
    "enabled": true,
    "max_retries": 1,
    "include_error_context": true,
    "include_available_columns": true
  },
  "transparency": {
    "show_sql_to_user": true,
    "show_retries_to_user": true,
    "show_errors_to_user": true,
    "show_status_messages": true,
    "use_collapsible_details": true
  },
  "rate_limiting": {
    "enabled": true,
    "requests_per_minute_per_user": 10,
    "requests_per_minute_global": 50
  },
  "learning": {
    "enabled": true,
    "corrections_file": "data/learned_corrections.jsonl",
    "max_corrections": 100,
    "include_in_prompt": true,
    "max_corrections_in_prompt": 20,
    "eviction_policy": "least_recently_matched"
  },
  "audit": {
    "enabled": true,
    "directory": "data/audit",
    "rotation": "daily",
    "retention_days": 90,
    "log_user_questions": true,
    "log_generated_sql": true,
    "log_results": false,
    "log_row_counts": true,
    "log_latency": true
  },
  "health": {
    "check_interval_seconds": 60,
    "database_ping": true,
    "ollama_ping": true,
    "schema_max_age_hours": 6,
    "schema_auto_refresh": true
  }
}
```

#### 3. `llm_config.json` — LLM Provider Configuration

```json
{
  "sql_generator": {
    "provider": "ollama",
    "ollama": {
      "url": "http://localhost:11434",
      "model": "sqlcoder:7b",
      "timeout_seconds": 15,
      "temperature": 0.0,
      "auto_pull": true
    },
    "copilot": {
      "model": "gpt-4o"
    }
  },
  "explainer": {
    "provider": "copilot",
    "ollama": {
      "url": "http://localhost:11434",
      "model": "llama3.1:8b",
      "timeout_seconds": 30,
      "temperature": 0.3
    },
    "copilot": {
      "model": "gpt-4o"
    }
  },
  "fallback": {
    "enabled": true,
    "sql_generator_fallback": "copilot",
    "explainer_fallback": "copilot"
  }
}
```

Provider options: `"ollama"`, `"copilot"`, `"openai_compat"`. When fallback is enabled, the connector seamlessly switches if the primary provider is unavailable.

#### 4. `glossary.json` — Business Term Definitions

Map business terms to SQL expressions so the LLM generates accurate queries:

```json
{
  "terms": [
    {
      "term": "revenue",
      "definition": "SUM(orders.total_amount) - refers to the total_amount column in the orders table, not a standalone column"
    },
    {
      "term": "churn",
      "definition": "An account with status='cancelled' and cancelled_at within the last 90 days"
    },
    {
      "term": "last quarter",
      "definition": "The most recently completed fiscal quarter. Fiscal year starts in April."
    }
  ]
}
```

#### 5. `admin_config.json` — Admin UI and Database Settings

```json
{
  "admin_ui": {
    "enabled": true,
    "path": "/admin",
    "allowed_github_users": ["your-github-username"],
    "session_timeout_hours": 24
  },
  "database": {
    "type": "postgres",
    "connection_string_env": "DATABASE_URL",
    "read_only_enforce": true
  }
}
```

---

## Usage

Once deployed and installed, interact with the agent in GitHub Copilot Chat by mentioning `@your-app-name`:

### Example Prompts

| Prompt | What it does |
|--------|-------------|
| `@db-copilot show me all tables in the database` | Lists all user-defined tables with column counts |
| `@db-copilot what are the top 10 customers by total revenue?` | Generates and runs a GROUP BY + ORDER BY query |
| `@db-copilot how many orders were placed last month?` | Queries with a date range filter |
| `@db-copilot describe the customers table` | Returns column names, types, and relationships |
| `@db-copilot what is the average salary by department?` | Aggregation query with GROUP BY |
| `@db-copilot show me orders that are still pending` | Filters by status column |

### Example Response (successful query)

````markdown
📊 **Results: Top 5 Customers by Revenue**

Here are your top customers for last quarter:

| Customer | Revenue |
|----------|---------|
| Acme Corp | $89,400 |
| GlobalTech | $67,200 |
| ... | ... |

Total revenue across top 5: $245,800. Average: $49,160.

<details>
<summary>🔍 SQL Query Used</summary>

```sql
SELECT c.name, SUM(o.total_amount) as revenue
FROM customers c
JOIN orders o ON c.id = o.customer_id
WHERE o.created_at >= '2026-01-01'
GROUP BY c.name
ORDER BY revenue DESC
LIMIT 5
```

</details>
````

### Example Response (self-corrected query)

````markdown
📊 **Results: Top 5 Customers by Revenue**

> ℹ️ **Note:** My first query used a column that doesn't exist (`revenue`).
> I corrected it to use `SUM(total_amount)` instead. I've learned this for future queries.

[results...]

<details>
<summary>🔍 SQL Query Used (corrected)</summary>

**First attempt (failed):**
```sql
SELECT customer_name, revenue FROM customers ORDER BY revenue DESC LIMIT 5
-- Error: column "revenue" does not exist
```

**Corrected query:**
```sql
SELECT c.name, SUM(o.total_amount) as revenue ...
```

</details>
````

### Real-Time Status Messages

During query processing, the user sees live status updates via SSE:

```
🔍 Searching approved schema for relevant tables...
📝 Generating SQL query...
✅ Query validated against approved tables
⚡ Checking query cost...
⚡ Running query against database...
📊 Processing 47 rows (showing top 10)...
💬 Preparing your answer...
```

---

## Using with Microsoft Teams

You can use this extension from Microsoft Teams via the GitHub integration:

### Setup

1. Install the **GitHub app for Teams** from the [Microsoft Teams App Store](https://teams.microsoft.com/l/app/836ecc9e-6dca-4696-a2e9-15e252cd3f31)
2. Connect your GitHub account in Teams
3. Ensure your GitHub account has the DB Copilot Extension installed

### Usage in Teams

In any Teams chat or channel, mention `@GitHub` followed by your agent name:

```
@GitHub @db-copilot show me the latest 10 orders
```

### Current Limitations

- The Teams integration is designed primarily for the **GitHub Copilot coding agent** workflow
- For the best experience with custom extensions like this one, use GitHub Copilot Chat directly at [github.com/copilot](https://github.com/copilot)
- Real-time streaming may behave differently in Teams compared to GitHub Chat

---

## Admin UI

The connector includes a built-in admin interface at `/admin` for managing all configuration — no file editing required.

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
| Dashboard | `/admin` | System health overview and status bar |
| Schema Access | `/admin/schema` | Browse all schemas (Tier 1), approve/deny tables and columns |
| Query Safety | `/admin/safety` | Row limits, cost thresholds, rate limits, transparency toggles |
| LLM Configuration | `/admin/llm` | Provider selection, model settings, fallback configuration |
| Business Glossary | `/admin/glossary` | Add/edit/remove business term definitions |
| Database Settings | `/admin/database` | Connection status, read-only verification, DB type |

All changes write to the corresponding JSON config file → trigger hot-reload → create an audit log entry.

### Admin Session Security

- Sessions are signed with HMAC-SHA256 using the `ADMIN_SESSION_SECRET` environment variable
- If `ADMIN_SESSION_SECRET` is not set, a random secret is generated at startup (sessions will not survive restarts)
- For production deployments, always set `ADMIN_SESSION_SECRET` to a stable random value:
  ```bash
  ADMIN_SESSION_SECRET=$(openssl rand -hex 32)
  ```

---

## Security

### Read-Only Enforcement (4 Layers)

The connector enforces read-only access at four independent layers:

1. **Database user permissions** — The connector should connect as a user with only `SELECT` privileges (see setup below)
2. **Connection-level setting** — For PostgreSQL, `default_transaction_read_only=on` is verified at startup
3. **SQL sanitizer** — Only `SELECT` statements are permitted; all DDL/DML keywords are blocked
4. **Post-generation validation** — Every generated query is parsed and validated against the approved schema before execution

### SQL Sanitization

The `database/sanitizer.go` enforces a strict policy on all generated SQL:

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

- 🔒 **Read-only database user** — Use a user with `SELECT` privileges only (see below)
- 🔒 **Query timeout** — All queries have a configurable timeout (default 30 seconds)
- 🔒 **Cost estimation** — Expensive queries are rejected before execution via `EXPLAIN`
- 🔒 **Rate limiting** — Per-user and global request rate limits prevent abuse
- 🔒 **Schema access control** — Only admin-approved tables and columns are queryable
- 🔒 **Hidden schemas** — Sensitive schemas can be completely hidden from users
- 🔒 **No secrets in logs** — API tokens and database credentials are never logged
- 🔒 **Audit trail** — Every query, validation, and error is recorded in append-only logs
- 🔒 **Environment variables** — All secrets are loaded from environment (`.env` is gitignored)
- 🔒 **Non-root Docker container** — The runtime container runs as `appuser` (UID 1001)

### Read-Only Database User Setup

For maximum safety, create a dedicated read-only database user for the connector. The connector enforces read-only access in code, but using a read-only DB user provides an additional layer of protection at the database level.

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

Connect using:
```
DATABASE_URL=postgres://db_copilot:your-secure-password@localhost:5432/your_database?sslmode=require
```

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
-- GRANT SELECT ON ALL TABLES IN SCHEMA schema1 TO USER db_copilot;

-- Grant access to the catalog views (needed for schema discovery)
GRANT SELECT ON SYSIBM.SYSTABLES TO USER db_copilot;
GRANT SELECT ON SYSCAT.TABLES TO USER db_copilot;
GRANT SELECT ON SYSCAT.COLUMNS TO USER db_copilot;
GRANT SELECT ON SYSCAT.REFERENCES TO USER db_copilot;
GRANT SELECT ON SYSCAT.TABCOMMENTS TO USER db_copilot;
GRANT SELECT ON SYSCAT.COLCOMMENTS TO USER db_copilot;
```

Connect using:
```
DB2_CONN_STRING=HOSTNAME=localhost;PORT=50000;DATABASE=your_database;UID=db_copilot;PWD=your-secure-password;PROTOCOL=TCPIP
```

> **Important:** Do not grant `INSERT`, `UPDATE`, `DELETE`, `ALTER`, or `DROP` privileges to the connector user. The connector will still refuse to execute write queries in code, but using a read-only user prevents accidental or malicious writes at the database level.

---

## Health Monitoring

The connector runs periodic background health checks and exposes a detailed `/health` endpoint.

### Configuration

Health settings live in `query_safety.json` under the `"health"` key:

| Setting | Default | Description |
|---------|---------|-------------|
| `check_interval_seconds` | 60 | How often background checks run |
| `database_ping` | true | Ping the database on each check |
| `ollama_ping` | true | Check LLM provider availability |
| `schema_max_age_hours` | 6 | Schema cache considered stale after this |
| `schema_auto_refresh` | true | Automatically recrawl stale schemas |

### `/health` Endpoint

Returns HTTP 200 when all components are healthy, HTTP 503 when any component is unhealthy.

```json
{
  "status": "healthy",
  "timestamp": "2026-03-14T10:00:00Z",
  "components": [
    {
      "name": "database",
      "status": "healthy",
      "message": "postgres reachable",
      "latency_ms": 3
    },
    {
      "name": "llm",
      "status": "healthy",
      "message": "ollama/sqlcoder:7b available",
      "latency_ms": 45
    },
    {
      "name": "schema",
      "status": "healthy",
      "message": "last crawled 2026-03-14T09:55:00Z"
    }
  ]
}
```

### Status Levels

- **healthy** — all components are healthy
- **degraded** — at least one component is degraded (e.g., LLM fallback active) but none are unhealthy
- **unhealthy** — at least one critical component is down (e.g., database unreachable)

---

## Audit Logging

Every interaction produces audit entries in append-only JSONL files with daily rotation.

### Configuration

Audit settings live in `query_safety.json` under the `"audit"` key:

| Setting | Default | Description |
|---------|---------|-------------|
| `enabled` | true | Enable/disable audit logging |
| `directory` | `data/audit` | Directory for audit log files |
| `rotation` | `daily` | Log file rotation frequency |
| `retention_days` | 90 | How long to keep log files |
| `log_user_questions` | true | Log the user's natural language question |
| `log_generated_sql` | true | Log the SQL generated by the LLM |
| `log_results` | false | Log actual result data (**not recommended**) |
| `log_row_counts` | true | Log row counts returned |
| `log_latency` | true | Log query execution time |

### Events Logged

| Event | Description |
|-------|-------------|
| `SYSTEM_START` | Connector startup with config summary |
| `DB_CONNECTION_OK` / `DB_CONNECTION_FAILED` | Database connectivity |
| `READ_ONLY_VERIFIED` / `READ_ONLY_WARNING` | Read-only status with liability disclaimer |
| `SCHEMA_CRAWL_COMPLETE` | Schema discovery results |
| `CONFIG_LOADED` / `CONFIG_RELOADED` / `CONFIG_RELOAD_FAILED` | Configuration changes |
| `CORRECTIONS_LOADED` | Learned corrections loaded on startup |
| `QUERY_START` | User question with GitHub username and user ID |
| `SQL_GENERATED` | Attempt number, SQL, model used, generation latency |
| `SQL_VALIDATED` / `SQL_VALIDATION_FAILED` | Tables referenced, approval status |
| `EXPLAIN_CHECK` | Estimated rows, cost, within limits |
| `QUERY_EXECUTED` | Rows returned, rows displayed, execution latency |
| `QUERY_FAILED` | Error, will retry flag |
| `QUERY_BLOCKED` | Reason, recommendation if applicable |
| `QUERY_COST_EXCEEDED` | Estimates, limits, retry flag |
| `QUERY_COMPLETE` | Total latency, retry count, status |
| `CORRECTION_LEARNED` | Original SQL, corrected SQL, correction type |
| `LLM_FALLBACK` | Which provider failed, which fallback used |
| `RATE_LIMITED` | User, request count, limit |
| `RECOMMENDATION_MADE` | What was recommended and to whom |
| `HEALTH_CHECK` | Component statuses |

> **Note:** Actual query result data is never logged — only row counts and summary statistics.

---

## Development

### Building

```bash
# Build without CGO (excludes DB2 driver)
CGO_ENABLED=0 go build ./...

# Build with CGO (includes DB2 driver — requires IBM clidriver)
go build ./...
```

### Running Tests

```bash
# Run all tests without CGO
CGO_ENABLED=0 go test ./... -v

# Run specific package tests
CGO_ENABLED=0 go test ./database/... -v
CGO_ENABLED=0 go test ./pipeline/... -v
CGO_ENABLED=0 go test ./config/... -v
CGO_ENABLED=0 go test ./admin/... -v
CGO_ENABLED=0 go test ./llm/... -v

# Run DB2 sanitizer tests specifically
CGO_ENABLED=0 go test ./db2/... -run TestSanitizeSQL -v

# Run all tests with CGO (requires IBM clidriver installed)
go test ./... -v
```

### Code Walkthrough

**Pipeline execution lifecycle** (`pipeline/executor.go`):

1. **Rate limit check** — reject if user exceeds per-minute quota
2. **Schema context** — build Tier 2 approved schema, include glossary and learned corrections
3. **SQL generation** — call the configured LLM provider (Ollama/Copilot/OpenAI-compatible)
4. **Post-generation validation** — parse SQL, verify all tables/columns against approved list
5. **Cost estimation** — run `EXPLAIN`, reject if estimated rows or cost exceed thresholds
6. **LIMIT injection** — inject or cap LIMIT clause for safety
7. **Query execution** — run the validated SQL against the database
8. **Result processing** — compute summary statistics, detect result shape
9. **Explanation** — call explanation LLM with results and stats
10. **Response streaming** — stream formatted response with SQL in collapsible block

**On failure:** steps 3–6 are retried once with error context and column hints (self-correction loop). Successful corrections are saved to `learned_corrections.jsonl`.

### Formatting

```bash
gofmt -w .
```

---

## Troubleshooting

### Database connection fails

**Symptom:** `FATAL database ping failed` at startup

**Solutions:**
- **PostgreSQL:** Verify `DATABASE_URL` format: `postgres://user:pass@host:5432/dbname?sslmode=require`
- **DB2:** Verify `DB2_CONN_STRING` format: `HOSTNAME=...;PORT=...;DATABASE=...;UID=...;PWD=...;PROTOCOL=TCPIP`
- Check that the database server is running and the port is open
- Confirm the database user has `CONNECT` permissions
- For Docker Compose, wait for the database health check to pass

### IBM DB2 driver fails to build

**Symptom:** `undefined: SQLSMALLINT` or similar CGO errors

**Solution:**
1. Ensure the IBM DB2 clidriver is installed (see [Step 2](#step-2-install-database-driver))
2. Verify environment variables are set:
   ```bash
   echo $IBM_DB_HOME
   echo $LD_LIBRARY_PATH   # Linux
   echo $DYLD_LIBRARY_PATH # macOS
   ```
3. Try running `go run setup.go` in the `go_ibm_db/installer` directory
4. Alternatively, use `CGO_ENABLED=0 go build ./...` for PostgreSQL-only builds

### LLM provider not available

**Symptom:** `FATAL could not initialize LLM provider` or queries fail with generation errors

**Solutions:**
- **Ollama:** Ensure Ollama is running (`ollama serve`) and the model is pulled (`ollama pull sqlcoder:7b`)
- **Copilot:** The Copilot token comes from the user's request — no setup needed
- **OpenAI-compatible:** Check `llm_config.json` for correct URL and API key environment variable
- If fallback is enabled, the connector will automatically switch to the backup provider

### Schema not loading

**Symptom:** The LLM doesn't seem to know about your tables

**Solutions:**
- Check that the database user has `SELECT` on catalog views (`information_schema` for PostgreSQL, `SYSCAT.*` for DB2)
- Schema is cached and auto-refreshed — check the `/health` endpoint for schema age
- If a table exists but isn't queryable, it may not be approved in `access_config.json` — use the Admin UI at `/admin/schema` to review

### Rate limited

**Symptom:** `You've been asking a lot of questions` error

**Solutions:**
- Default limit is 10 requests per minute per user and 50 global
- Adjust limits in `config/query_safety.json` → `rate_limiting` section, or via the Admin UI at `/admin/safety`

### Query cost exceeded

**Symptom:** `That question would require scanning too much data`

**Solutions:**
- The connector runs `EXPLAIN` before executing and rejects queries exceeding cost thresholds
- Try adding date ranges, filters, or asking for aggregations (count, average, total)
- Adjust thresholds in `config/query_safety.json` → `cost_estimation` section

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
- Ensure your server clock is synchronized (NTP)

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
