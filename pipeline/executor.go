package pipeline

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/iamankushpandit/db2-copilot-extension/audit"
	"github.com/iamankushpandit/db2-copilot-extension/config"
	"github.com/iamankushpandit/db2-copilot-extension/database"
	"github.com/iamankushpandit/db2-copilot-extension/llm"
	"github.com/iamankushpandit/db2-copilot-extension/schema"
)

// Executor orchestrates the full query pipeline:
//
//	question → rate limit → schema → SQL gen → validate → cost → execute → limit → explain
type Executor struct {
	db          database.Client
	crawler     *schema.Crawler
	cfgMgr      *config.Manager
	sqlGen      llm.TextToSQLProvider
	explainer   llm.ExplanationProvider
	learning    *LearningStore
	rateLimiter *RateLimiter
	auditLogger *audit.Logger
}

// NewExecutor creates an Executor with all dependencies.
func NewExecutor(
	db database.Client,
	crawler *schema.Crawler,
	cfgMgr *config.Manager,
	sqlGen llm.TextToSQLProvider,
	explainer llm.ExplanationProvider,
	learning *LearningStore,
	rateLimiter *RateLimiter,
	auditLogger *audit.Logger,
) *Executor {
	return &Executor{
		db:          db,
		crawler:     crawler,
		cfgMgr:      cfgMgr,
		sqlGen:      sqlGen,
		explainer:   explainer,
		learning:    learning,
		rateLimiter: rateLimiter,
		auditLogger: auditLogger,
	}
}

// Request holds the input to the pipeline.
type Request struct {
	Question             string
	GitHubUser           string
	GitHubUID            string
	CopilotToken         string
	CopilotIntegrationID string
}

// Execute runs the full pipeline, streaming the response to w.
func (e *Executor) Execute(ctx context.Context, req Request, w io.Writer) error {
	start := time.Now()
	safety := e.cfgMgr.Safety()
	access := e.cfgMgr.Access()

	// 1. Rate limit check.
	if safety.RateLimiting.Enabled {
		allowed, count := e.rateLimiter.Allow(req.GitHubUID)
		if !allowed {
			e.auditLogger.Log(audit.EventRateLimited, req.GitHubUser, req.GitHubUID,
				audit.RateLimitedDetails{
					User:         req.GitHubUser,
					RequestCount: count,
					Limit:        safety.RateLimiting.RequestsPerMinutePerUser,
				})
			return streamStatus(w, fmt.Sprintf(
				"⚠️ You've reached the rate limit (%d requests/minute). Please wait a moment.",
				safety.RateLimiting.RequestsPerMinutePerUser))
		}
	}

	e.auditLogger.Log(audit.EventQueryStart, req.GitHubUser, req.GitHubUID,
		audit.QueryStartDetails{Question: req.Question})

	// 2. Fetch and build schema context.
	if safety.Transparency.ShowStatusMessages {
		_ = streamStatus(w, "🔍 Searching approved schema for relevant tables...")
	}

	fullSchema, err := e.crawler.Get(ctx)
	if err != nil {
		log.Printf("WARN schema crawl failed: %v", err)
	}

	schemaContext := schema.BuildApprovedContext(fullSchema, access)

	// 3. Collect glossary terms.
	glossary := e.cfgMgr.Glossary()
	var glossaryTerms []llm.GlossaryTerm
	for _, t := range glossary.Terms {
		glossaryTerms = append(glossaryTerms, llm.GlossaryTerm{
			Term:       t.Term,
			Definition: t.Definition,
		})
	}

	// 4. Get relevant learned corrections.
	var allTables []string
	if fullSchema != nil {
		for _, s := range fullSchema.Schemas {
			for _, t := range s.Tables {
				allTables = append(allTables, t.Name)
			}
		}
	}
	corrections := e.learning.Select(allTables, safety.Learning.MaxCorrectionsInPrompt)
	learnedCorrections := ToLLMCorrections(corrections)

	// 5. SQL generation + self-correction loop.
	if safety.Transparency.ShowStatusMessages {
		_ = streamStatus(w, "📝 Generating SQL query...")
	}

	var (
		attempt   int
		prevError string
		prevSQL   string
	)

	maxAttempts := 1
	if safety.SelfCorrection.Enabled {
		maxAttempts = 1 + safety.SelfCorrection.MaxRetries
	}

	for attempt = 1; attempt <= maxAttempts; attempt++ {
		sqlStart := time.Now()
		generatedSQL, genErr := e.sqlGen.GenerateSQL(ctx, llm.SQLGenerationRequest{
			Question:             req.Question,
			SchemaContext:        schemaContext,
			GlossaryTerms:        glossaryTerms,
			LearnedCorrections:   learnedCorrections,
			DBType:               e.db.DBType(),
			PreviousError:        prevError,
			PreviousSQL:          prevSQL,
			Attempt:              attempt,
			CopilotToken:         req.CopilotToken,
			CopilotIntegrationID: req.CopilotIntegrationID,
		})
		sqlLatency := time.Since(sqlStart).Milliseconds()

		if genErr != nil {
			log.Printf("ERROR SQL generation attempt %d failed: %v", attempt, genErr)
			e.auditLogger.Log(audit.EventQueryFailed, req.GitHubUser, req.GitHubUID,
				audit.QueryFailedDetails{Error: genErr.Error(), WillRetry: attempt < maxAttempts})
			if attempt >= maxAttempts {
				return streamError(w, "I couldn't generate a SQL query for that question. Please try rephrasing.")
			}
			prevError = genErr.Error()
			continue
		}

		// If no SQL was generated, stream directly.
		if generatedSQL == "" {
			e.auditLogger.Log(audit.EventQueryComplete, req.GitHubUser, req.GitHubUID,
				audit.QueryCompleteDetails{
					TotalLatencyMS: time.Since(start).Milliseconds(),
					RetryCount:     attempt - 1,
					Status:         "no_sql",
				})
			return e.explainer.ExplainResults(ctx, llm.ExplanationRequest{
				Question:             req.Question,
				SQL:                  "",
				Attempt:              attempt,
				ShowSQL:              false,
				CopilotToken:         req.CopilotToken,
				CopilotIntegrationID: req.CopilotIntegrationID,
			}, w)
		}

		e.auditLogger.Log(audit.EventSQLGenerated, req.GitHubUser, req.GitHubUID,
			audit.SQLGeneratedDetails{
				Attempt:   attempt,
				SQL:       generatedSQL,
				Model:     e.sqlGen.Name(),
				LatencyMS: sqlLatency,
			})

		// 6. Sanitize.
		sanitizedSQL, sanitizeErr := database.SanitizeSQL(generatedSQL)
		if sanitizeErr != nil {
			msg := fmt.Sprintf("SQL sanitization failed: %v", sanitizeErr)
			e.auditLogger.Log(audit.EventSQLValidationFailed, req.GitHubUser, req.GitHubUID,
				audit.SQLValidatedDetails{ApprovalStatus: "sanitize_failed", Reason: msg})
			if attempt >= maxAttempts {
				return streamError(w, msg)
			}
			prevError = msg
			prevSQL = generatedSQL
			continue
		}

		// 7. Post-generation validation.
		valResult := database.ValidateSQL(sanitizedSQL, access)
		if !valResult.Approved {
			// Check if tables are hidden.
			if len(valResult.HiddenTables) > 0 {
				e.auditLogger.Log(audit.EventQueryBlocked, req.GitHubUser, req.GitHubUID,
					audit.QueryBlockedDetails{Reason: "hidden_table"})
				return streamError(w, "I'm not able to help with that query.")
			}

			// Unapproved tables — check if they exist in Tier 1.
			if len(valResult.UnapprovedTables) > 0 {
				e.auditLogger.Log(audit.EventQueryBlocked, req.GitHubUser, req.GitHubUID,
					audit.QueryBlockedDetails{
						Reason:         "unapproved_table",
						Recommendation: strings.Join(valResult.UnapprovedTables, ", "),
					})

				recs := schema.FindRecommendations(req.Question, fullSchema, access)
				if len(recs) > 0 {
					e.auditLogger.Log(audit.EventRecommendationMade, req.GitHubUser, req.GitHubUID,
						audit.RecommendationDetails{
							Schema:     recs[0].Schema,
							Table:      recs[0].Table.Name,
							ReasonCode: "unapproved_access_requested",
						})
					return streamMessage(w, schema.FormatRecommendation(recs[0]))
				}
				return streamError(w, database.UnapprovedTableError(valResult.UnapprovedTables))
			}
		}

		e.auditLogger.Log(audit.EventSQLValidated, req.GitHubUser, req.GitHubUID,
			audit.SQLValidatedDetails{
				TablesReferenced: valResult.TablesReferenced,
				ApprovalStatus:   "approved",
			})

		// 8. Cost estimation.
		if safety.CostEstimation.ExplainBeforeExecute {
			if safety.Transparency.ShowStatusMessages {
				_ = streamStatus(w, "⚡ Checking query cost...")
			}

			estimatedRows, estimatedCost, explainErr := e.db.ExplainCost(ctx, sanitizedSQL)
			if explainErr == nil {
				withinLimits := estimatedRows <= safety.CostEstimation.MaxEstimatedRows &&
					estimatedCost <= safety.CostEstimation.MaxEstimatedCost

				e.auditLogger.Log(audit.EventExplainCheck, req.GitHubUser, req.GitHubUID,
					audit.ExplainCheckDetails{
						EstimatedRows: estimatedRows,
						EstimatedCost: estimatedCost,
						WithinLimits:  withinLimits,
					})

				if !withinLimits {
					e.auditLogger.Log(audit.EventQueryCostExceeded, req.GitHubUser, req.GitHubUID,
						audit.QueryCostExceededDetails{
							EstimatedRows: estimatedRows,
							EstimatedCost: estimatedCost,
							MaxRows:       safety.CostEstimation.MaxEstimatedRows,
							MaxCost:       safety.CostEstimation.MaxEstimatedCost,
							WillRetry:     attempt < maxAttempts,
						})

					if attempt < maxAttempts {
						prevError = fmt.Sprintf(
							"This query would scan ~%d rows (limit: %d). Add more filters or use aggregation.",
							estimatedRows, safety.CostEstimation.MaxEstimatedRows)
						prevSQL = sanitizedSQL
						if safety.Transparency.ShowStatusMessages {
							_ = streamStatus(w, fmt.Sprintf("⚠️ Query too expensive (~%d rows), refining...", estimatedRows))
						}
						continue
					}
					return streamError(w, "That question would require scanning too much data. Try narrowing with a date range, specific filter, or ask for a count/average/total instead.")
				}
			}
		}

		// 9. Inject LIMIT.
		limitedSQL := e.db.InjectLimit(sanitizedSQL, safety.QueryLimits.MaxQueryRows)

		// 10. Execute.
		if safety.Transparency.ShowStatusMessages {
			_ = streamStatus(w, "⚡ Running query against database...")
		}

		queryStart := time.Now()
		rows, execErr := e.db.ExecuteQuery(ctx, limitedSQL)
		queryLatency := time.Since(queryStart).Milliseconds()

		if execErr != nil {
			translatedErr := translateDBError(execErr)
			e.auditLogger.Log(audit.EventQueryFailed, req.GitHubUser, req.GitHubUID,
				audit.QueryFailedDetails{Error: execErr.Error(), WillRetry: attempt < maxAttempts})

			if attempt < maxAttempts {
				prevError = translatedErr
				prevSQL = sanitizedSQL
				if safety.Transparency.ShowStatusMessages && safety.Transparency.ShowRetriesToUser {
					_ = streamStatus(w, fmt.Sprintf("⚠️ Query had an error: %s", translatedErr))
					_ = streamStatus(w, fmt.Sprintf("🔄 Correcting query (attempt %d of %d)...", attempt+1, maxAttempts))
				}
				continue
			}

			return streamError(w, fmt.Sprintf("The query failed to execute: %s", translatedErr))
		}

		// If we corrected successfully, record it.
		if attempt > 1 {
			if safety.Learning.Enabled && e.learning != nil {
				corrType := classifyCorrectionType(prevError)
				_ = e.learning.Add(&Correction{
					Timestamp:        time.Now().UTC(),
					OriginalQuestion: req.Question,
					FailedSQL:        prevSQL,
					Error:            prevError,
					CorrectedSQL:     sanitizedSQL,
					Tables:           valResult.TablesReferenced,
					CorrectionType:   corrType,
					LastMatchedAt:    time.Now().UTC(),
					MatchCount:       1,
				})
				e.auditLogger.Log(audit.EventCorrectionLearned, req.GitHubUser, req.GitHubUID,
					audit.CorrectionLearnedDetails{
						OriginalSQL:    prevSQL,
						CorrectedSQL:   sanitizedSQL,
						CorrectionType: corrType,
					})
			}
		}

		displayRows, total := database.LimitResults(rows, safety.QueryLimits.DisplayRows)

		if safety.Transparency.ShowStatusMessages {
			_ = streamStatus(w, fmt.Sprintf("📊 Processing %d rows (showing top %d)...", total, len(displayRows)))
		}

		e.auditLogger.Log(audit.EventQueryExecuted, req.GitHubUser, req.GitHubUID,
			audit.QueryExecutedDetails{
				RowsReturned:  total,
				RowsDisplayed: len(displayRows),
				LatencyMS:     queryLatency,
			})

		// 11. Compute summary stats.
		var summaryStatsText string
		if safety.QueryLimits.ComputeSummaryStats {
			stats := database.ComputeStats(rows)
			summaryStatsText = formatStats(stats)
		}

		finalSQL := sanitizedSQL

		// 12. Update learned corrections match stats.
		if len(corrections) > 0 {
			_ = e.learning.UpdateMatchStats(corrections)
		}

		if safety.Transparency.ShowStatusMessages {
			_ = streamStatus(w, "💬 Preparing your answer...")
		}

		// 13. Stream explanation.
		e.auditLogger.Log(audit.EventQueryComplete, req.GitHubUser, req.GitHubUID,
			audit.QueryCompleteDetails{
				TotalLatencyMS: time.Since(start).Milliseconds(),
				RetryCount:     attempt - 1,
				Status:         "success",
			})

		return e.explainer.ExplainResults(ctx, llm.ExplanationRequest{
			Question:              req.Question,
			SQL:                   finalSQL,
			DisplayRows:           displayRows,
			TotalRows:             total,
			SummaryStats:          summaryStatsText,
			Attempt:               attempt,
			PreviousSQL:           prevSQL,
			PreviousError:         prevError,
			ShowSQL:               safety.Transparency.ShowSQLToUser,
			UseCollapsibleDetails: safety.Transparency.UseCollapsibleDetails,
			CopilotToken:          req.CopilotToken,
			CopilotIntegrationID:  req.CopilotIntegrationID,
		}, w)
	}

	// Exhausted all attempts.
	return streamError(w, "❌ I couldn't complete this query after multiple attempts. Please try rephrasing your question.")
}

// streamStatus writes a status SSE event.
func streamStatus(w io.Writer, msg string) error {
	_, err := fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", msg+"\n")
	return err
}

// streamMessage writes an informational message as SSE.
func streamMessage(w io.Writer, msg string) error {
	payload := fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\ndata: [DONE]\n\n", msg)
	_, err := fmt.Fprint(w, payload)
	return err
}

// streamError writes an error SSE event.
func streamError(w io.Writer, msg string) error {
	payload := fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\ndata: [DONE]\n\n", msg)
	_, err := fmt.Fprint(w, payload)
	return err
}

// translateDBError converts a raw database error to a user-friendly message.
func translateDBError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "does not exist") && strings.Contains(lower, "column"):
		return fmt.Sprintf("The database doesn't have a column mentioned in the query: %v", err)
	case strings.Contains(lower, "does not exist") && strings.Contains(lower, "relation"):
		return fmt.Sprintf("Table referenced in the query doesn't exist: %v", err)
	case strings.Contains(lower, "permission denied"):
		return "I don't have access to that table. This might be a configuration issue — please let your admin know."
	case strings.Contains(lower, "statement timeout") || strings.Contains(lower, "context deadline"):
		return "That query took too long and was stopped. Try narrowing your question with a date range or specific filter."
	case strings.Contains(lower, "connection refused"):
		return "Unable to reach the database. Please try again later."
	default:
		return msg
	}
}

// classifyCorrectionType returns a short label for the type of correction.
func classifyCorrectionType(errMsg string) string {
	lower := strings.ToLower(errMsg)
	switch {
	case strings.Contains(lower, "column"):
		return "wrong_column_name"
	case strings.Contains(lower, "relation") || strings.Contains(lower, "table"):
		return "wrong_table_name"
	case strings.Contains(lower, "syntax"):
		return "syntax_error"
	case strings.Contains(lower, "cost") || strings.Contains(lower, "rows"):
		return "query_too_expensive"
	default:
		return "unknown"
	}
}

// formatStats converts SummaryStats to a human-readable string.
func formatStats(stats database.SummaryStats) string {
	if stats.TotalRows == 0 {
		return ""
	}
	var sb strings.Builder
	for _, col := range stats.Columns {
		if col.NullCount == stats.TotalRows {
			continue // All nulls, skip.
		}
		if col.Avg != 0 || col.Sum != 0 {
			sb.WriteString(fmt.Sprintf("- %s: min=%.2f, max=%.2f, avg=%.2f, sum=%.2f\n",
				col.ColumnName, col.Min, col.Max, col.Avg, col.Sum))
		} else if len(col.ValueCounts) > 0 {
			sb.WriteString(fmt.Sprintf("- %s (%d distinct values)\n", col.ColumnName, col.DistinctCount))
		}
	}
	return sb.String()
}
