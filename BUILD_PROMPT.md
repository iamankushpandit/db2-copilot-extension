# DB Copilot Connector — Complete Build Specification

## Project Overview

Build a production-grade GitHub Copilot Extension that connects to either IBM DB2 or PostgreSQL databases, allowing users to ask natural language questions and get SQL-powered answers. This is a **read-only database query tool** — it must never modify any data.

The existing codebase is at: https://github.com/iamankushpandit/db2-copilot-extension
Branch with current work: `copilot/add-postgresql-support`

The existing code is a basic proof-of-concept. This prompt describes the full production architecture to replace/refactor it.

---

## Core Principles

1. **Read-only, no exceptions.** Enforced at 4 layers: DB user permissions, connection-level `default_transaction_read_only=on` (PostgreSQL), SQL sanitizer (SELECT only), and post-generation SQL validation. On startup, verify read-only status. If write permissions are detected, log a VERBOSE warning with liability disclaimer but continue running.

2. **Nothing is accessible by default.** Admins must explicitly approve which schemas, tables, and columns the agent can query. The agent is internally AWARE of all schemas (for recommendations) but only EXPOSES approved ones to the LLM.

3. **Transparency.** The agent shows users what it's doing in real time (status messages via SSE), shows the SQL it generated (in collapsible details blocks), and tells users when it self-corrected.

4. **Admin-controlled boundaries, users operate within them.** All limits (row counts, display rows, rate limits, cost thresholds) are set by admins. Users cannot override them.

5. **Single database per deployment.** Each connector instance connects to ONE database (either DB2 or PostgreSQL, not both). Companies with multiple databases deploy multiple connector instances.

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
│  │    → LLM 1: Text-to-SQL (Ollama sqlcoder:7b or Copilot)        │    │
│  │    → Post-Generation Validation (all tables/columns approved?)   │    │
│  │    → EXPLAIN Cost Check (reject if too expensive)                │    │
│  │    → Execute Query (with injected LIMIT safety)                  │    │
│  │    → Result Limiting (3-tier: DB cap → display → summary)       │    │
│  │    → Detect Result Shape (scalar/single/small/large)            │    │
│  │    → LLM 2: Explain Results (Copilot GPT-4o or Ollama)         │    │
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
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Configuration Files

All configuration lives in JSON files that support hot-reload (file watcher with 2-second debounce, read-write mutex, validate JSON before accepting, never replace running config with failed parse).

### 1. `config/access_config.json` — Schema Access Control

```json
{
  "version": "1.0",
  "last_modified_by": "admin_username",
  "last_modified_at": "2026-03-14T10:00:00Z",
  
  "approved_schemas": [
    {
      "schema": "public",
      "approved_by": "admin_username",
      "approved_at": "2026-03-14T10:00:00Z",
      "reason": "Core business tables",
      "access_level": "partial",
      
      "approved_tables": [
        {
          "table": "customers",
          "approved_by": "admin_username",
          "approved_at": "2026-03-14T10:00:00Z",
          "reason": "Sales team needs customer lookup",
          "access_level": "partial",
          "approved_columns": ["id", "name", "email", "company", "region", "created_at", "status"],
          "denied_columns": ["ssn", "credit_card", "date_of_birth"],
          "denied_reason": "PII columns excluded per compliance policy"
        },
        {
          "table": "orders",
          "approved_by": "admin_username",
          "approved_at": "2026-03-14T10:00:00Z",
          "reason": "Revenue reporting",
          "access_level": "full"
        }
      ]
    },
    {
      "schema": "analytics",
      "approved_by": "admin_username",
      "approved_at": "2026-03-14T10:05:00Z",
      "reason": "Pre-aggregated views, safe for all users",
      "access_level": "full"
    }
  ],
  
  "hidden_schemas": [
    {
      "schema": "security",
      "reason": "Auth infrastructure - do not recommend to users",
      "hidden_by": "admin_username",
      "hidden_at": "2026-03-14T10:00:00Z"
    }
  ]
}
```

Access levels:
- `"full"` on a schema = all tables and all columns approved, including future ones
- `"partial"` on a schema = only explicitly listed tables approved
- `"full"` on a table = all columns approved
- `"partial"` on a table = only `approved_columns` are queryable, `denied_columns` are invisible to LLM

### 2. `config/query_safety.json` — Limits and Safety

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

### 3. `config/llm_config.json` — LLM Provider Configuration

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

### 4. `config/glossary.json` — Business Term Definitions

```json
{
  "terms": [
    {
      "term": "revenue",
      "definition": "SUM(orders.total_amount) - refers to the total_amount column in the orders table, not a standalone column",
      "added_by": "admin_username",
      "added_at": "2026-03-14T10:00:00Z"
    },
    {
      "term": "churn",
      "definition": "An account with status='cancelled' and cancelled_at within the last 90 days",
      "added_by": "admin_username",
      "added_at": "2026-03-14T10:00:00Z"
    },
    {
      "term": "last quarter",
      "definition": "The most recently completed fiscal quarter. Fiscal year starts in April. So in March 2026, last quarter = Oct 1 2025 to Dec 31 2025",
      "added_by": "admin_username",
      "added_at": "2026-03-14T10:00:00Z"
    }
  ]
}
```

### 5. `config/admin_config.json` — Admin UI Settings

```json
{
  "admin_ui": {
    "enabled": true,
    "path": "/admin",
    "allowed_github_users": ["ankush", "admin2"],
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

## Two-Tier Schema Awareness

The agent maintains two views of the database:

**Tier 1: Full Awareness (internal, never exposed to LLM or user)**
- Crawls ALL schemas, tables, columns, data types, PKs, FKs, table comments, column comments
- For PostgreSQL: queries `information_schema.columns`, `information_schema.table_constraints`, `information_schema.key_column_usage`, `information_schema.constraint_column_usage`, `pg_description`
- For DB2: queries `SYSCAT.TABLES`, `SYSCAT.COLUMNS`, `SYSCAT.REFERENCES`, `SYSCAT.TABCOMMENTS`, `SYSCAT.COLCOMMENTS`
- Includes sample data: 3-5 rows per table (from approved tables only)
- Includes row count estimates and column statistics
- Refreshes on schedule (default every 6 hours) AND reactively on query failure

**Tier 2: Approved Subset (exposed to LLM)**
- Filtered view based on `access_config.json`
- Only approved schemas → approved tables → approved columns
- This is what gets injected into the SQL generation prompt
- Formatted as structured text with types, PKs, FKs, sample data, and row counts

When a user asks about something not in Tier 2, the agent checks Tier 1:
- If the table exists but isn't approved → recommend it with full details (schema, table, columns, types, relationships, row count)
- If the table is in `hidden_schemas` → say nothing, respond as if it doesn't exist
- If the table genuinely doesn't exist → tell the user

---

## LLM Provider Abstraction

Two interfaces:

```go
type TextToSQLProvider interface {
    GenerateSQL(ctx context.Context, req SQLGenerationRequest) (string, error)
    Available() bool
    Name() string
}

type ExplanationProvider interface {
    ExplainResults(ctx context.Context, req ExplanationRequest, w io.Writer) error
    Available() bool
    Name() string
}
```

Three implementations:
1. **OllamaProvider** — connects to local Ollama server
2. **CopilotProvider** — uses the existing Copilot API
3. **OpenAICompatProvider** — generic OpenAI-format API (covers vLLM, LocalAI, LM Studio)

Fallback support: if primary provider is unavailable, seamlessly switch to fallback.

---

## Result Limiting (3-Tier)

**Tier 1: DB-Level Limit** — inject/cap LIMIT in SQL (default max: 100). Respect LLM's LIMIT if within bounds.
**Tier 2: Display Limit** — only first N rows (default: 10) sent to explanation LLM.
**Tier 3: Summary Statistics** — computed from ALL returned rows (numeric: min/max/avg/sum; categorical with <20 distinct values: value counts).

Use `pg_query_go` for PostgreSQL LIMIT injection. Regex for DB2.

---

## Post-Generation SQL Validation

Parse SQL with `pg_query_go`, extract all table/column references, validate against approved list. If not approved, check Tier 1 for recommendations.

---

## Query Cost Estimation

Run `EXPLAIN` (not `EXPLAIN ANALYZE`) before executing. Reject if estimated rows or cost exceeds thresholds. Self-correct once, then inform user.

---

## Self-Correction Loop

Max 1 retry. On failure, send error + available columns to LLM. On retry success, save correction to `learned_corrections.jsonl`. Rolling window of 100 corrections, LRU eviction (least recently matched drops off).

---

## Real-Time Status Streaming via SSE

Stream emoji-prefixed status messages during pipeline execution so user sees progress.

---

## Audit Log

Append-only JSONL, daily rotation. Log all events (system, config, query, access control, health). Never log actual result data. Log both GitHub username and user ID.

---

## Admin UI

Configuration-only for MVP. Same Go binary at `/admin`. Pages: Schema Access, Query Safety, LLM Config, Business Glossary, Database Settings. Status bar at top. Auth via GitHub OAuth, restricted to allowed_github_users.

---

## Startup Sequence

Load configs → connect DB → verify read-only → check LLM → crawl schema → load corrections → start HTTP server.

---

## Graceful Shutdown

SIGTERM/SIGINT → stop accepting → wait 30s for in-flight → flush audit → close DB → log summary.

---

## Key Dependencies

- `github.com/pganalyze/pg_query_go/v5`
- `github.com/lib/pq`
- `github.com/fsnotify/fsnotify`
- `github.com/ibmdb/go_ibm_db` (behind build tag)

---

## Build Order

1. Config system (loader, validation, hot-reload, all 5 config file types)
2. Audit logger (event system)
3. Database layer (interface, PostgreSQL client, read-only verification)
4. Schema system (Tier 1 crawler, Tier 2 filter, recommender)
5. SQL sanitizer + post-generation validator (pg_query_go)
6. Cost estimation (EXPLAIN check)
7. Result limiter (3-tier)
8. LLM provider abstraction (Ollama, Copilot, OpenAI-compat, fallback)
9. Core pipeline orchestration
10. Self-correction loop
11. Learning system (corrections store, LRU eviction)
12. Rate limiter
13. Status streaming
14. Presentation engine
15. Admin UI
16. Startup sequence
17. Graceful shutdown
18. README with full documentation
