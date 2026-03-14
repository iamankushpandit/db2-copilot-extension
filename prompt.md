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
- Formatted as structured text:
  ```
  Schema: public
    Table: customers
      - id (integer, NOT NULL, PK)
      - name (varchar(255), NOT NULL)
      - email (varchar(255))
      - company (varchar(255))
      - region (varchar(50))
      - created_at (timestamp, NOT NULL)
      - status (varchar(20))
      Relationships: orders.customer_id → customers.id
      Row count: ~15,000
      Sample: id=1 name="Acme Corp" region="NA" status="active"
    Table: orders
      - [all columns with types, constraints, relationships]
  ```

When a user asks about something not in Tier 2, the agent checks Tier 1:
- If the table exists but isn't approved → recommend it with full details (schema, table, columns, types, relationships, row count)
- If the table is in `hidden_schemas` → say nothing, respond as if it doesn't exist
- If the table genuinely doesn't exist → tell the user

---

## LLM Provider Abstraction

Two interfaces:

```go
// TextToSQLProvider generates SQL from natural language
type TextToSQLProvider interface {
    GenerateSQL(ctx context.Context, req SQLGenerationRequest) (string, error)
    Available() bool  // health check
    Name() string     // "ollama/sqlcoder:7b" or "copilot/gpt-4o"
}

// ExplanationProvider explains query results in natural language
type ExplanationProvider interface {
    ExplainResults(ctx context.Context, req ExplanationRequest, w io.Writer) error
    Available() bool
    Name() string
}
```

Three implementations:
1. **OllamaProvider** — connects to local Ollama server, uses structured prompts optimized for code models
2. **CopilotProvider** — uses the existing Copilot API with the user's token from the request
3. **OpenAICompatProvider** — generic OpenAI-format API (covers vLLM, LocalAI, LM Studio, etc.)

Fallback: if the primary provider is unavailable, seamlessly switch to the fallback provider. Log the fallback event in audit.

**SQL generation prompt structure (for Ollama/sqlcoder):**
```
### Database: PostgreSQL
### Schema:
[Tier 2 approved schema with types, PKs, FKs, sample data]

### Business Glossary:
- revenue: SUM(orders.total_amount)
- churn: status='cancelled' within 90 days
[from glossary.json]

### Learned Corrections (from past queries on this database):
- "revenue" is not a column. Use SUM(orders.total_amount)
- Customer name is in "name" column, not "customer_name"
[top 20 most relevant from learned_corrections.jsonl, selected by table overlap]

### Rules:
- SELECT only. No INSERT, UPDATE, DELETE, DROP, ALTER, CREATE.
- Use LIMIT if no limit specified.
- PostgreSQL syntax.
- Only use tables and columns from the schema above.

### Question:
[user's question]

### SQL:
```

**Explanation prompt structure (for Copilot/GPT-4o):**
```
You are explaining database query results to a business user.

The user asked: "[question]"

Query executed ([N attempts], [retry info]):
[SQL]

Results ([displayed] of [total] rows):
[markdown table of display_rows]

Summary statistics:
[computed stats for numeric/categorical columns]

Instructions:
- Explain clearly in plain language
- Use markdown formatting
- Show the data table
- Highlight key insights
- Include the SQL query in a collapsible <details> block
- If there were retries, explain what was corrected
- If results were truncated, mention the total count and summarize using the stats
```

---

## Result Limiting (3-Tier)

**Tier 1: DB-Level Limit**
- If LLM's SQL has no LIMIT → inject `LIMIT {max_query_rows}` (default 100)
- If LLM's SQL has `LIMIT N` where N ≤ max_query_rows → respect it as-is
- If LLM's SQL has `LIMIT N` where N > max_query_rows → cap to max_query_rows
- For DB2: inject `FETCH FIRST {max_query_rows} ROWS ONLY`
- Use `pg_query_go` SQL parser for PostgreSQL to inject/modify LIMIT safely. For DB2 use regex-based injection.

**Tier 2: Display Limit**
- Only first `display_rows` (default 10) rows are formatted and sent to the explanation LLM
- LLM is told: "Showing {display_rows} of {total_returned} rows"

**Tier 3: Summary Statistics (computed from ALL returned rows)**
- Numeric columns: min, max, avg, sum, count
- Categorical columns (fewer than 20 distinct values): value counts
- All columns: null count, distinct count
- Sent to explanation LLM alongside display rows
- This lets the LLM say "average salary is $75K across 87 employees" even though it only sees 10 rows

---

## Post-Generation SQL Validation

After the LLM generates SQL, BEFORE executing:

1. Parse the SQL using `pg_query_go` (PostgreSQL) or regex (DB2)
2. Extract all referenced table names and column names
3. Check every table against `access_config.json` approved list
4. Check every column against approved columns (if table is `"partial"` access)
5. If any table/column is not approved → DO NOT EXECUTE. Instead:
   - Check Tier 1 (full awareness) for the table
   - If exists but not approved → recommend to user with full details
   - If in hidden_schemas → respond as if table doesn't exist
6. If all approved → proceed to EXPLAIN cost check

---

## Query Cost Estimation

Before executing any validated query:

1. Run `EXPLAIN` (PostgreSQL) or `EXPLAIN PLAN` (DB2) — NOT `EXPLAIN ANALYZE`
2. Parse estimated rows and cost from the output
3. If `estimated_rows > max_estimated_rows` OR `estimated_cost > max_estimated_cost`:
   - First attempt → self-correct: send back to LLM with "This query would scan ~{N} rows. Add more filters or use aggregation."
   - Second attempt still exceeds → tell user: "That question would require scanning too much data. Try narrowing with a date range, specific customer, or ask for a count/average/total instead."
4. If within limits → execute

---

## Self-Correction Loop

On any failure (DB error, validation error, cost exceeded):

1. Attempt 1 fails → build correction context:
   - The exact error message
   - The available columns for referenced tables (from Tier 2)
   - The specific issue ("column 'revenue' does not exist — available columns in orders: id, customer_id, total_amount, status, created_at")
2. Send back to the SQL generation LLM with the correction context
3. Attempt 2 succeeds → save correction to `learned_corrections.jsonl`:
   ```jsonl
   {"timestamp":"...","original_question":"...","failed_sql":"...","error":"...","corrected_sql":"...","tables":["..."],"correction_type":"wrong_column_name","last_matched_at":"...","match_count":1}
   ```
4. Attempt 2 fails → tell user with both attempts shown, suggest rephrasing

Corrections file: rolling window of 100 entries (configurable). Eviction policy: least recently matched. Each correction has `last_matched_at` timestamp updated when used as a prompt hint. When at capacity, evict the correction with oldest `last_matched_at`.

---

## Learned Corrections Selection

When building the SQL generation prompt, select the most relevant corrections:
1. Load all corrections from `learned_corrections.jsonl`
2. For each correction, check if any of its `tables` overlap with tables in the approved schema
3. Rank by relevance (table overlap) then by `match_count` (most frequently useful first)
4. Include top `max_corrections_in_prompt` (default 20) in the prompt
5. When a correction is included and the resulting query succeeds, update its `last_matched_at` and increment `match_count`

---

## Real-Time Status Streaming

During query processing, stream status messages to the user via SSE before the final response:

```
🔍 Searching approved schema for relevant tables...
📝 Generating SQL query...
✅ Query validated against approved tables
⚡ Checking query cost...
⚡ Running query against database...
📊 Processing 47 rows (showing top 10)...
💬 Preparing your answer...
```

On retry:
```
📝 Generating SQL query...
⚠️ Query had an error: column 'revenue' does not exist
🔄 Correcting query (attempt 2 of 2)...
📝 Regenerating SQL with corrected column names...
✅ Query validated
⚡ Running corrected query...
📊 Processing 47 rows...
💬 Preparing your answer...
```

---

## Response Format

**Successful query (first attempt):**
```markdown
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

​```sql
SELECT c.name, SUM(o.total_amount) as revenue
FROM customers c
JOIN orders o ON c.id = o.customer_id
WHERE o.created_at >= '2026-01-01'
GROUP BY c.name
ORDER BY revenue DESC
LIMIT 5
​```

</details>
```

**Successful query after retry:**
```markdown
📊 **Results: Top 5 Customers by Revenue**

> ℹ️ **Note:** My first query used a column that doesn't exist (`revenue`). I corrected it to use `SUM(total_amount)` instead. I've learned this for future queries.

[results...]

<details>
<summary>🔍 SQL Query Used (corrected)</summary>

**First attempt (failed):**
​```sql
SELECT customer_name, revenue FROM customers ORDER BY revenue DESC LIMIT 5
-- Error: column "revenue" does not exist
​```

**Corrected query:**
​```sql
SELECT c.name, SUM(o.total_amount) as revenue ...
​```

</details>
```

**Table not approved:**
```markdown
I found a table that might have what you need, but it's not currently approved for querying.

**Recommended addition for your admin:**
- Schema: `warehouse`
- Table: `inventory_levels`
- Columns: `sku` (varchar, NOT NULL), `warehouse_id` (integer, NOT NULL), `quantity_on_hand` (integer), `reorder_point` (integer), `last_updated` (timestamp)
- Relationships: `sku` → `products.sku` (FK), `warehouse_id` → `warehouses.id` (FK)
- Approximate rows: ~45,000

Please ask your admin to add this to the approved configuration.
```

**Failed after all retries:**
```markdown
❌ **Couldn't complete this query**

I tried two approaches but both failed:

<details>
<summary>🔍 Attempt 1</summary>
​```sql
[sql]
​```
Error: [human-readable error]
</details>

<details>
<summary>🔍 Attempt 2</summary>
​```sql
[sql]
​```
Error: [human-readable error]
</details>

**Suggestion:** Try rephrasing your question — for example, add a date range or ask for a specific metric (count, average, total).
```

---

## Error Message Translation

Every internal error is translated to plain language:

| Internal Error | User Sees |
|---|---|
| `column "X" does not exist` | "The database doesn't have a column called 'X'. I'll try a different approach..." (triggers retry) |
| `relation "X" does not exist` | "Table 'X' doesn't exist in the database." |
| `permission denied` | "I don't have access to that table. This might be a configuration issue — please let your admin know." |
| `statement timeout` | "That query took too long and was stopped. Try narrowing your question with a date range or specific filter." |
| Connection refused (Ollama) | "My SQL generation service is temporarily unavailable. Using a backup service." (use fallback) |
| Table not approved | "I found [table] but it's not approved yet. Here are details for your admin: ..." |
| Rate limited | "You've been asking a lot of questions (limit: 10/minute). Please wait a moment." |
| Cost exceeded after retry | "That question would require scanning too much data. Try adding filters or asking for a summary." |

---

## Audit Log

Every interaction produces audit entries. Append-only JSONL files, daily rotation.

Events logged:
- `SYSTEM_START` — connector startup with config summary
- `DB_CONNECTION_OK` / `DB_CONNECTION_FAILED`
- `READ_ONLY_VERIFIED` / `READ_ONLY_WARNING` (with full liability disclaimer)
- `SCHEMA_CRAWL_COMPLETE`
- `CONFIG_LOADED` / `CONFIG_RELOADED` / `CONFIG_RELOAD_FAILED`
- `CORRECTIONS_LOADED`
- `QUERY_START` — user question with GitHub username AND user ID
- `SQL_GENERATED` — attempt number, SQL, model used, generation latency
- `SQL_VALIDATED` / `SQL_VALIDATION_FAILED` — tables referenced, approval status
- `EXPLAIN_CHECK` — estimated rows, cost, within limits
- `QUERY_EXECUTED` — rows returned, rows displayed, execution latency
- `QUERY_FAILED` — error, will retry flag
- `QUERY_BLOCKED` — reason, recommendation if applicable
- `QUERY_COST_EXCEEDED` — estimates, limits, retry flag
- `QUERY_COMPLETE` — total latency, retry count, status
- `CORRECTION_LEARNED` — original SQL, corrected SQL, correction type
- `LLM_FALLBACK` — which provider failed, which fallback used
- `RATE_LIMITED` — user, request count, limit
- `RECOMMENDATION_MADE` — what was recommended and to whom
- `HEALTH_CHECK` — component statuses

**Never log actual query result data.** Only row counts and summary stats.

Log both GitHub username and GitHub user ID for every user event.

---

## Admin UI

Served by the same Go binary at `/admin`. Configuration-only for MVP. No dashboards, no metrics charts, no live tails.

**Tech stack:** Go HTTP handlers + embedded static HTML/JS/CSS (single binary deployment). Use Go `html/template` or lightweight JS (htmx or Alpine.js). Auth via GitHub OAuth (reuse existing `oauth/service.go`). Only users in `admin_config.json → allowed_github_users` can access.

**Status bar** at top of every page (not a dashboard, just current state):
```
🟢 All systems healthy | DB: postgres ✅ | LLM: ollama/sqlcoder:7b ✅ | Tables: 8 approved
```

**Pages:**

1. **Schema Access** — browse all schemas/tables from Tier 1, approve/deny with granularity (schema/table/column level), set access_level, add reasons. Shows which are approved, which are available, which are hidden.

2. **Query Safety** — edit all fields from `query_safety.json`: row limits, display limits, cost thresholds, retry settings, transparency toggles, rate limits.

3. **LLM Configuration** — select provider (Ollama/Copilot/OpenAI-compatible), set URLs, models, timeouts, temperatures, fallback settings.

4. **Business Glossary** — add/edit/remove business terms with definitions.

5. **Database Settings** — view connection status, read-only verification status, DB type. Connection string is NOT shown (security), only the env var name.

All changes write to the corresponding JSON config file → triggers hot-reload → audit log entry.

---

## Startup Sequence

1. Load all config files. Validate JSON. If invalid → REFUSE TO START.
2. Connect to database. `SELECT 1`. If fails → REFUSE TO START.
3. Verify read-only: check user privileges, check `default_transaction_read_only`. If write perms detected → LOG VERBOSE WARNING with liability disclaimer, continue.
4. Check LLM provider. If Ollama: ping server, check model availability, auto-pull if missing. If pull fails and no fallback → REFUSE TO START.
5. Crawl full schema (Tier 1). Build approved subset (Tier 2). Cache both.
6. Load learned corrections.
7. Start HTTP server. Log ready message with full status summary.

---

## Graceful Shutdown

On SIGTERM/SIGINT:
- Stop accepting new requests
- Wait for in-flight requests to complete (up to 30 seconds)
- Flush audit log buffer
- Close database connection pool
- Log shutdown summary
- If in-flight requests don't finish in 30s → force shutdown, log interrupted requests

---

## File Structure

```
db2-copilot-extension/
├── main.go
├── agent/
│   └── service.go                 # Core pipeline: question → SQL → execute → explain
├── config/
│   ├── loader.go                  # Config file loading, validation, hot-reload with fsnotify
│   ├── access.go                  # Access config types and helpers
│   ├── safety.go                  # Query safety config types
│   ├── llm.go                     # LLM config types
│   ├── glossary.go                # Glossary types
│   └── admin.go                   # Admin config types
├── database/
│   ├── interface.go               # Client interface
│   ├── sanitizer.go               # SQL sanitizer (SELECT only)
│   ├── validator.go               # Post-generation SQL validation against approved list
│   ├── cost.go                    # EXPLAIN-based cost estimation
│   ├── limiter.go                 # LIMIT injection and result limiting
│   └── stats.go                   # Summary statistics computation
├── schema/
│   ├── crawler.go                 # Full schema discovery (Tier 1)
│   ├── filter.go                  # Approved schema filtering (Tier 2)
│   └── recommender.go            # Schema recommendation engine
├── db2/
│   ├── client.go                  # DB2 Client implementation
│   ├── driver.go                  # DB2 driver registration (build tag)
│   └── schema.go                  # DB2-specific schema queries
├── postgres/
│   ├── client.go                  # PostgreSQL Client implementation
│   └── schema.go                  # PostgreSQL-specific schema queries
├── llm/
│   ├── interface.go               # TextToSQLProvider + ExplanationProvider interfaces
│   ├── ollama.go                  # Ollama implementation
│   ├── copilot.go                 # Copilot API implementation
│   ├── openai_compat.go          # Generic OpenAI-compatible implementation
│   ├── fallback.go                # Fallback wrapper
│   └── prompts.go                 # Prompt templates for SQL gen and explanation
├── pipeline/
│   ├── executor.go                # Orchestrates the full query pipeline
│   ├── correction.go              # Self-correction loop
│   ├── learning.go                # Learned corrections store (100 rolling, LRU eviction)
│   ├── presenter.go               # Result shape detection + presentation
│   └── ratelimit.go              # Per-user and global rate limiting
├── audit/
│   ├── logger.go                  # Append-only JSONL audit logger with daily rotation
│   └── events.go                  # Audit event type definitions
├── admin/
│   ├── handler.go                 # Admin UI HTTP handlers
│   ├── auth.go                    # Admin authentication (GitHub OAuth)
│   └── static/                    # Embedded HTML/JS/CSS for admin UI
├── copilot/
│   ├── endpoints.go               # Copilot API client (existing)
│   └── messages.go                # Copilot message types (existing)
├── oauth/
│   └── service.go                 # OAuth service (existing)
├── config/                        # Runtime config files (created on first run)
│   ├── access_config.json
│   ├── query_safety.json
│   ├── llm_config.json
│   ├── glossary.json
│   └── admin_config.json
├── data/                          # Runtime data (created automatically)
│   ├── audit/
│   │   └── audit-YYYY-MM-DD.jsonl
│   └── learned_corrections.jsonl
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── go.sum
├── .env.example
├── .gitignore
└── README.md                      # Comprehensive setup guide including read-only user SQL for both PG and DB2
```

---

## Key Dependencies

- `github.com/pganalyze/pg_query_go/v5` — PostgreSQL SQL parser (for post-generation validation and LIMIT injection)
- `github.com/lib/pq` — PostgreSQL driver
- `github.com/fsnotify/fsnotify` — File watcher for config hot-reload
- `github.com/ibmdb/go_ibm_db` — DB2 driver (behind build tag)

---

## Build Order

Implement in this order, as each layer depends on the previous:

1. Config system (loader, validation, hot-reload, all 5 config file types)
2. Audit logger (event system that everything writes to)
3. Database layer (interface, PostgreSQL client, read-only verification, connection health)
4. Schema system (Tier 1 crawler with PKs/FKs/comments/samples, Tier 2 filter, recommender)
5. SQL sanitizer + post-generation validator (using pg_query_go)
6. Cost estimation (EXPLAIN check)
7. Result limiter (3-tier: LIMIT injection, display limit, summary stats)
8. LLM provider abstraction (interface, Ollama client, Copilot client, fallback wrapper)
9. Core pipeline (orchestrate: schema → generate → validate → cost → execute → limit → explain)
10. Self-correction loop
11. Learning system (corrections store, selection, LRU eviction)
12. Rate limiter
13. Status streaming (real-time SSE status messages during pipeline)
14. Presentation engine (result shape detection, response formatting with transparency)
15. Admin UI (configuration screens only)
16. Startup sequence (verification chain)
17. Graceful shutdown
18. README with full documentation including read-only user setup SQL for PostgreSQL and DB2