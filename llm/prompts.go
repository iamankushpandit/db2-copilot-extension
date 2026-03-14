package llm

const (
	SQLGenerationPrompt = `### Database: {{.DBType}}
### Schema:
{{.Schema}}

### Business Glossary:
{{.Glossary}}

### Learned Corrections (from past queries on this database):
{{.Corrections}}

### Rules:
- SELECT only. No INSERT, UPDATE, DELETE, DROP, ALTER, CREATE.
- Use LIMIT if no limit specified.
- {{.Syntax}} syntax.
- Only use tables and columns from the schema above.

### Question:
{{.Question}}

### SQL:
`

	ExplanationPrompt = `You are explaining database query results to a business user.

The user asked: "{{.Question}}"

Query executed ({{.Attempts}} attempts, {{.RetryInfo}}):
{{.SQL}}

Results ({{.Displayed}} of {{.Total}} rows):
{{.Results}}

Summary statistics:
{{.Summary}}

Instructions:
- Explain clearly in plain language
- Use markdown formatting
- Show the data table
- Highlight key insights
- Include the SQL query in a collapsible <details> block
- If there were retries, explain what was corrected
- If results were truncated, mention the total count and summarize using the stats
`
)
