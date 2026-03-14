package schema

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/iamankushpandit/db2-copilot-extension/database"
)

// Crawler manages the full Tier 1 schema cache with periodic auto-refresh.
type Crawler struct {
	client       database.Client
	maxAge       time.Duration
	autoRefresh  bool

	mu          sync.RWMutex
	info        *database.SchemaInfo
	lastCrawled time.Time
	crawlErr    error
}

// NewCrawler creates a Crawler backed by the provided database client.
// maxAgeHours is how long the cache is considered fresh; autoRefresh triggers
// background re-crawls on the same schedule.
func NewCrawler(client database.Client, maxAgeHours int, autoRefresh bool) *Crawler {
	return &Crawler{
		client:      client,
		maxAge:      time.Duration(maxAgeHours) * time.Hour,
		autoRefresh: autoRefresh,
	}
}

// Get returns the cached Tier 1 schema, refreshing if stale.
func (c *Crawler) Get(ctx context.Context) (*database.SchemaInfo, error) {
	c.mu.RLock()
	info := c.info
	lastCrawled := c.lastCrawled
	c.mu.RUnlock()

	if info != nil && time.Since(lastCrawled) < c.maxAge {
		return info, nil
	}

	return c.refresh(ctx)
}

// Start launches a background goroutine that refreshes the schema on schedule.
// The goroutine exits when ctx is cancelled.
func (c *Crawler) Start(ctx context.Context) {
	if !c.autoRefresh || c.maxAge <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(c.maxAge)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := c.refresh(ctx); err != nil {
					log.Printf("WARN schema auto-refresh failed: %v", err)
				}
			}
		}
	}()
}

// refresh performs an actual crawl and updates the cache.
func (c *Crawler) refresh(ctx context.Context) (*database.SchemaInfo, error) {
	info, err := c.client.CrawlSchema(ctx)

	c.mu.Lock()
	c.crawlErr = err
	if err == nil {
		c.info = info
		c.lastCrawled = time.Now()
	}
	c.mu.Unlock()

	return info, err
}
