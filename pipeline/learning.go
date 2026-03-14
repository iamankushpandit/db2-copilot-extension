package pipeline

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/iamankushpandit/db2-copilot-extension/llm"
)

// Correction records a past SQL generation failure and its fix.
type Correction struct {
	Timestamp        time.Time `json:"timestamp"`
	OriginalQuestion string    `json:"original_question"`
	FailedSQL        string    `json:"failed_sql"`
	Error            string    `json:"error"`
	CorrectedSQL     string    `json:"corrected_sql"`
	Tables           []string  `json:"tables"`
	CorrectionType   string    `json:"correction_type"`
	LastMatchedAt    time.Time `json:"last_matched_at"`
	MatchCount       int       `json:"match_count"`
}

// LearningStore manages the rolling window of learned corrections.
type LearningStore struct {
	mu          sync.Mutex
	path        string
	maxEntries  int
	corrections []*Correction
}

// NewLearningStore creates a LearningStore backed by the given JSONL file.
func NewLearningStore(path string, maxEntries int) (*LearningStore, error) {
	ls := &LearningStore{
		path:       path,
		maxEntries: maxEntries,
	}
	if err := ls.load(); err != nil {
		return nil, err
	}
	return ls, nil
}

// load reads all corrections from the JSONL file.
func (ls *LearningStore) load() error {
	f, err := os.Open(ls.path)
	if os.IsNotExist(err) {
		return nil // File will be created on first write.
	}
	if err != nil {
		return fmt.Errorf("opening corrections file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var c Correction
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue // Skip malformed lines.
		}
		ls.corrections = append(ls.corrections, &c)
	}
	return scanner.Err()
}

// Add saves a new correction, evicting the oldest if at capacity.
func (ls *LearningStore) Add(c *Correction) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	ls.corrections = append(ls.corrections, c)

	// Evict least-recently-matched entries when over capacity.
	if len(ls.corrections) > ls.maxEntries {
		sort.Slice(ls.corrections, func(i, j int) bool {
			return ls.corrections[i].LastMatchedAt.Before(ls.corrections[j].LastMatchedAt)
		})
		ls.corrections = ls.corrections[len(ls.corrections)-ls.maxEntries:]
	}

	return ls.save()
}

// Select returns the most relevant corrections for the given set of tables.
// Up to maxCount corrections are returned, ranked by table overlap then match count.
func (ls *LearningStore) Select(tables []string, maxCount int) []*Correction {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	tableSet := make(map[string]bool, len(tables))
	for _, t := range tables {
		tableSet[t] = true
	}

	type scored struct {
		c     *Correction
		score int
	}
	var candidates []scored
	for _, c := range ls.corrections {
		overlap := 0
		for _, t := range c.Tables {
			if tableSet[t] {
				overlap++
			}
		}
		if overlap > 0 {
			candidates = append(candidates, scored{c, overlap*100 + c.MatchCount})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if maxCount > len(candidates) {
		maxCount = len(candidates)
	}
	result := make([]*Correction, maxCount)
	for i := range result {
		result[i] = candidates[i].c
	}
	return result
}

// UpdateMatchStats increments MatchCount and updates LastMatchedAt for each
// correction that was actually included in the prompt.
func (ls *LearningStore) UpdateMatchStats(corrections []*Correction) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	now := time.Now().UTC()
	for _, c := range corrections {
		c.LastMatchedAt = now
		c.MatchCount++
	}
	return ls.save()
}

// ToLLMCorrections converts Corrections to the llm.LearnedCorrection format.
func ToLLMCorrections(corrections []*Correction) []llm.LearnedCorrection {
	result := make([]llm.LearnedCorrection, len(corrections))
	for i, c := range corrections {
		result[i] = llm.LearnedCorrection{
			OriginalQuestion: c.OriginalQuestion,
			FailedSQL:        c.FailedSQL,
			Error:            c.Error,
			CorrectedSQL:     c.CorrectedSQL,
		}
	}
	return result
}

// save rewrites the entire JSONL file (while holding ls.mu).
func (ls *LearningStore) save() error {
	// Ensure parent directory exists.
	dir := dirOf(ls.path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating corrections directory: %w", err)
		}
	}

	f, err := os.Create(ls.path)
	if err != nil {
		return fmt.Errorf("creating corrections file: %w", err)
	}
	defer f.Close()

	for _, c := range ls.corrections {
		data, err := json.Marshal(c)
		if err != nil {
			continue
		}
		fmt.Fprintf(f, "%s\n", data)
	}
	return nil
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return ""
}
