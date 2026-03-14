package database

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/iamankushpandit/db2-copilot-extension/config"
)

func InjectLimit(sql string, queryLimits *config.QueryLimits) (string, error) {
	if !queryLimits.EnforceLimit {
		return sql, nil
	}

	// This is a simplified implementation using regex.
	// A proper implementation should use a SQL parser to safely inject/modify the LIMIT clause.
	// TODO: replace with a parser-based solution.

	limitRegex := regexp.MustCompile(`(?i)\sLIMIT\s+(\d+)`)
	fetchRegex := regexp.MustCompile(`(?i)\sFETCH\s+FIRST\s+(\d+)\s+ROWS?\s+ONLY`)

	if limitRegex.MatchString(sql) {
		match := limitRegex.FindStringSubmatch(sql)
		limit, _ := strconv.Atoi(match[1])
		if limit > queryLimits.MaxQueryRows {
			return limitRegex.ReplaceAllString(sql, fmt.Sprintf(" LIMIT %d", queryLimits.MaxQueryRows)), nil
		}
		return sql, nil
	}

	if fetchRegex.MatchString(sql) {
		match := fetchRegex.FindStringSubmatch(sql)
		limit, _ := strconv.Atoi(match[1])
		if limit > queryLimits.MaxQueryRows {
			return fetchRegex.ReplaceAllString(sql, fmt.Sprintf(" FETCH FIRST %d ROWS ONLY", queryLimits.MaxQueryRows)), nil
		}
		return sql, nil
	}

	// No limit found, inject one.
	// This is not safe for all queries (e.g., with existing ORDER BY, GROUP BY, etc.)
	return fmt.Sprintf("%s LIMIT %d", strings.TrimRight(sql, ";"), queryLimits.MaxQueryRows), nil
}
