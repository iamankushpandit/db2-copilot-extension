package llm

import (
	"fmt"
	"strings"
)

// BuildSQLPrompt builds the SQL generation prompt for Ollama/sqlcoder style models.
func BuildSQLPrompt(req SQLGenerationRequest) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("### Database: %s\n", dbTypeFull(req.DBType)))
	sb.WriteString("### Schema:\n")
	sb.WriteString(req.SchemaContext)
	sb.WriteString("\n")

	if len(req.GlossaryTerms) > 0 {
		sb.WriteString("### Business Glossary:\n")
		for _, t := range req.GlossaryTerms {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Term, t.Definition))
		}
		sb.WriteString("\n")
	}

	if len(req.LearnedCorrections) > 0 {
		sb.WriteString("### Learned Corrections (from past queries on this database):\n")
		for _, c := range req.LearnedCorrections {
			if c.Error != "" {
				sb.WriteString(fmt.Sprintf("- %s\n", c.Error))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("### Rules:\n")
	sb.WriteString("- SELECT only. No INSERT, UPDATE, DELETE, DROP, ALTER, CREATE.\n")
	sb.WriteString("- Use LIMIT if no limit specified.\n")
	sb.WriteString(fmt.Sprintf("- %s syntax.\n", dbTypeFull(req.DBType)))
	sb.WriteString("- Only use tables and columns from the schema above.\n")
	sb.WriteString("\n")

	if req.PreviousError != "" {
		sb.WriteString("### Correction Context:\n")
		sb.WriteString(fmt.Sprintf("The previous query failed: %s\n", req.PreviousError))
		if req.PreviousSQL != "" {
			sb.WriteString(fmt.Sprintf("Failed SQL: %s\n", req.PreviousSQL))
		}
		sb.WriteString("Please fix the query.\n\n")
	}

	sb.WriteString("### Question:\n")
	sb.WriteString(req.Question)
	sb.WriteString("\n\n### SQL:\n")

	return sb.String()
}

// BuildSystemPrompt builds the system prompt for chat-based models (Copilot/GPT-4o).
func BuildSystemPrompt(req SQLGenerationRequest) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are a helpful %s database assistant. ", dbTypeFull(req.DBType)))
	sb.WriteString("Your job is to translate natural language questions into SQL queries.\n\n")

	sb.WriteString("## Database Schema\n\n")
	sb.WriteString(req.SchemaContext)
	sb.WriteString("\n")

	if len(req.GlossaryTerms) > 0 {
		sb.WriteString("## Business Glossary\n\n")
		for _, t := range req.GlossaryTerms {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", t.Term, t.Definition))
		}
		sb.WriteString("\n")
	}

	if len(req.LearnedCorrections) > 0 {
		sb.WriteString("## Learned Corrections\n\n")
		for _, c := range req.LearnedCorrections {
			if c.Error != "" {
				sb.WriteString(fmt.Sprintf("- %s\n", c.Error))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Rules\n\n")
	sb.WriteString("- Only generate SELECT statements — never INSERT, UPDATE, DELETE, or DDL.\n")
	sb.WriteString("- Wrap any SQL query in <sql>...</sql> tags.\n")
	sb.WriteString("- If the user asks a question requiring a query, produce exactly one <sql>...</sql> block.\n")
	sb.WriteString("- If no query is needed (e.g. a general question), answer directly without <sql> tags.\n")
	sb.WriteString(fmt.Sprintf("- Use %s-compatible SQL syntax.\n", dbTypeFull(req.DBType)))

	if req.PreviousError != "" {
		sb.WriteString("\n## Correction Required\n\n")
		sb.WriteString(fmt.Sprintf("The previous query failed with: %s\n", req.PreviousError))
		if req.PreviousSQL != "" {
			sb.WriteString(fmt.Sprintf("Failed SQL:\n```sql\n%s\n```\n", req.PreviousSQL))
		}
		sb.WriteString("Please generate a corrected query.\n")
	}

	return sb.String()
}

// BuildExplanationSystemPrompt builds the system prompt for the explanation LLM.
func BuildExplanationSystemPrompt(req ExplanationRequest) string {
	var sb strings.Builder

	sb.WriteString("You are explaining database query results to a business user.\n\n")
	sb.WriteString(fmt.Sprintf("The user asked: %q\n\n", req.Question))

	sb.WriteString(fmt.Sprintf("Query executed (%d attempt(s)):\n```sql\n%s\n```\n\n", req.Attempt, req.SQL))

	if req.TotalRows == 0 {
		sb.WriteString("The query returned no rows.\n\n")
	} else {
		sb.WriteString(fmt.Sprintf("Results (%d of %d rows shown):\n", len(req.DisplayRows), req.TotalRows))
		if len(req.DisplayRows) > 0 {
			sb.WriteString(formatTable(req.DisplayRows))
			sb.WriteString("\n")
		}
		if req.SummaryStats != "" {
			sb.WriteString("Summary statistics:\n")
			sb.WriteString(req.SummaryStats)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("## Instructions\n\n")
	sb.WriteString("- Explain clearly in plain language\n")
	sb.WriteString("- Use markdown formatting\n")
	sb.WriteString("- Show the data table\n")
	sb.WriteString("- Highlight key insights\n")

	if req.ShowSQL {
		if req.UseCollapsibleDetails {
			sb.WriteString("- Include the SQL query in a collapsible <details> block\n")
		} else {
			sb.WriteString("- Include the SQL query used\n")
		}
	}

	if req.Attempt > 1 {
		sb.WriteString("- Mention that the first attempt failed and was corrected\n")
	}

	if req.TotalRows > len(req.DisplayRows) {
		sb.WriteString(fmt.Sprintf("- Note that results are truncated: showing %d of %d total rows, summarize using the stats\n",
			len(req.DisplayRows), req.TotalRows))
	}

	return sb.String()
}

func dbTypeFull(dbType string) string {
	switch dbType {
	case "postgres":
		return "PostgreSQL"
	case "db2":
		return "IBM DB2"
	default:
		return dbType
	}
}

func formatTable(rows []map[string]interface{}) string {
	if len(rows) == 0 {
		return ""
	}
	var cols []string
	for k := range rows[0] {
		cols = append(cols, k)
	}
	var sb strings.Builder
	sb.WriteString("| " + strings.Join(cols, " | ") + " |\n")
	sb.WriteString("|" + strings.Repeat("---|", len(cols)) + "\n")
	for _, row := range rows {
		sb.WriteString("|")
		for _, col := range cols {
			v := row[col]
			if v == nil {
				sb.WriteString(" NULL |")
			} else {
				sb.WriteString(fmt.Sprintf(" %v |", v))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
