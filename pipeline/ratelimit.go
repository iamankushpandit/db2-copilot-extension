package pipeline

import (
	"sync"
	"time"
)

// RateLimiter enforces per-user and global rate limits using a sliding window.
type RateLimiter struct {
	enabled           bool
	perUserLimit      int
	globalLimit       int
	window            time.Duration

	mu          sync.Mutex
	userWindows map[string][]time.Time
	globalTimes []time.Time
}

// NewRateLimiter creates a RateLimiter.
// requestsPerMinutePerUser and requestsPerMinuteGlobal are the limits.
// If enabled is false, Allow always returns true.
func NewRateLimiter(enabled bool, perUserLimit, globalLimit int) *RateLimiter {
	return &RateLimiter{
		enabled:      enabled,
		perUserLimit: perUserLimit,
		globalLimit:  globalLimit,
		window:       time.Minute,
		userWindows:  make(map[string][]time.Time),
	}
}

// Allow returns true if the user is within the rate limits.
// It also returns the current per-user request count for logging.
func (r *RateLimiter) Allow(userID string) (allowed bool, userCount int) {
	if !r.enabled {
		return true, 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Clean and count global requests.
	var globalFiltered []time.Time
	for _, t := range r.globalTimes {
		if t.After(cutoff) {
			globalFiltered = append(globalFiltered, t)
		}
	}
	r.globalTimes = globalFiltered
	if len(r.globalTimes) >= r.globalLimit {
		return false, len(r.userWindows[userID])
	}

	// Clean and count per-user requests.
	times := r.userWindows[userID]
	var filtered []time.Time
	for _, t := range times {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	r.userWindows[userID] = filtered
	userCount = len(filtered)

	if userCount >= r.perUserLimit {
		return false, userCount
	}

	// Record this request.
	r.userWindows[userID] = append(r.userWindows[userID], now)
	r.globalTimes = append(r.globalTimes, now)
	return true, userCount + 1
}
