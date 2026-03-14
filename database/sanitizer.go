package database

import (
	"fmt"
	"strings"
)

func SanitizeSQL(sql string) (string, error) {
	trimmedSQL := strings.TrimSpace(sql)
	if !strings.HasPrefix(strings.ToUpper(trimmedSQL), "SELECT") {
		return "", fmt.Errorf("only SELECT statements are allowed")
	}
	return trimmedSQL, nil
}
