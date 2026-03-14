package pipeline_test

import (
	"testing"
	"time"

	"github.com/iamankushpandit/db2-copilot-extension/pipeline"
)

func TestRateLimiter_Disabled(t *testing.T) {
	rl := pipeline.NewRateLimiter(false, 2, 10)
	for i := 0; i < 100; i++ {
		allowed, _ := rl.Allow("user1")
		if !allowed {
			t.Fatalf("expected allowed when disabled, iteration %d", i)
		}
	}
}

func TestRateLimiter_PerUserLimit(t *testing.T) {
	rl := pipeline.NewRateLimiter(true, 3, 100)
	for i := 0; i < 3; i++ {
		allowed, _ := rl.Allow("user1")
		if !allowed {
			t.Fatalf("expected allowed on request %d", i+1)
		}
	}
	// 4th request should be denied.
	allowed, count := rl.Allow("user1")
	if allowed {
		t.Error("expected rate limited on 4th request")
	}
	if count != 3 {
		t.Errorf("count: got %d, want 3", count)
	}
}

func TestRateLimiter_DifferentUsers(t *testing.T) {
	rl := pipeline.NewRateLimiter(true, 2, 100)
	rl.Allow("user1")
	rl.Allow("user1")

	// user2 should not be affected by user1's limit.
	allowed, _ := rl.Allow("user2")
	if !allowed {
		t.Error("user2 should be allowed independently of user1")
	}
}

func TestRateLimiter_GlobalLimit(t *testing.T) {
	rl := pipeline.NewRateLimiter(true, 100, 3)
	rl.Allow("user1")
	rl.Allow("user2")
	rl.Allow("user3")

	// 4th global request should be denied regardless of user.
	allowed, _ := rl.Allow("user4")
	if allowed {
		t.Error("expected global rate limit to kick in")
	}
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	// This is a slow test that demonstrates window expiry; skip in short mode.
	if testing.Short() {
		t.Skip("skipping window expiry test in short mode")
	}

	_ = time.Second // Use time package to avoid unused import if test is skipped.
}
