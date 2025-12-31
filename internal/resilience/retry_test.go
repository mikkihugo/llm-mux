package resilience

import (
	"errors"
	"testing"
	"time"

	"github.com/sony/gobreaker"
)

func TestCircuitBreakerOpensAfterConsecutiveFailures(t *testing.T) {
	stateChanges := make([]gobreaker.State, 0)
	cfg := DefaultBreakerConfig("test")
	cfg.MinRequests = 3
	cfg.FailureThreshold = 3
	cfg.OnStateChange = func(_ string, _, to gobreaker.State) {
		stateChanges = append(stateChanges, to)
	}

	breaker := NewCircuitBreaker(cfg)

	for i := 0; i < 5; i++ {
		breaker.Execute(func() (any, error) { return nil, errors.New("fail") })
	}

	if breaker.State() != gobreaker.StateOpen {
		t.Errorf("expected StateOpen, got %v", breaker.State())
	}

	if len(stateChanges) == 0 || stateChanges[len(stateChanges)-1] != gobreaker.StateOpen {
		t.Errorf("expected state change to Open, got %v", stateChanges)
	}
}

func TestCircuitBreakerStaysClosedOnSuccess(t *testing.T) {
	cfg := DefaultBreakerConfig("test-success")
	cfg.MinRequests = 3
	cfg.FailureThreshold = 5

	breaker := NewCircuitBreaker(cfg)

	for i := 0; i < 10; i++ {
		breaker.Execute(func() (any, error) { return "ok", nil })
	}

	if breaker.State() != gobreaker.StateClosed {
		t.Errorf("expected StateClosed, got %v", breaker.State())
	}
}

func TestCircuitBreakerHalfOpenAfterTimeout(t *testing.T) {
	cfg := DefaultBreakerConfig("test-timeout")
	cfg.MinRequests = 2
	cfg.FailureThreshold = 2
	cfg.Timeout = 50 * time.Millisecond

	breaker := NewCircuitBreaker(cfg)

	for i := 0; i < 3; i++ {
		breaker.Execute(func() (any, error) { return nil, errors.New("fail") })
	}

	if breaker.State() != gobreaker.StateOpen {
		t.Fatalf("expected StateOpen, got %v", breaker.State())
	}

	time.Sleep(60 * time.Millisecond)

	if breaker.State() != gobreaker.StateHalfOpen {
		t.Errorf("expected StateHalfOpen after timeout, got %v", breaker.State())
	}
}

func TestCircuitBreakerReturnsCountsCorrectly(t *testing.T) {
	cfg := DefaultBreakerConfig("test-counts")
	breaker := NewCircuitBreaker(cfg)

	breaker.Execute(func() (any, error) { return "ok", nil })
	breaker.Execute(func() (any, error) { return nil, errors.New("fail") })
	breaker.Execute(func() (any, error) { return "ok", nil })

	counts := breaker.Counts()
	if counts.Requests != 3 {
		t.Errorf("expected 3 requests, got %d", counts.Requests)
	}
	if counts.TotalSuccesses != 2 {
		t.Errorf("expected 2 successes, got %d", counts.TotalSuccesses)
	}
	if counts.TotalFailures != 1 {
		t.Errorf("expected 1 failure, got %d", counts.TotalFailures)
	}
}

func TestCircuitBreakerName(t *testing.T) {
	cfg := DefaultBreakerConfig("my-breaker")
	breaker := NewCircuitBreaker(cfg)

	if breaker.Name() != "my-breaker" {
		t.Errorf("expected name 'my-breaker', got %s", breaker.Name())
	}
}

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		name        string
		attempt     int
		baseDelay   time.Duration
		maxDelay    time.Duration
		jitterDelay time.Duration
		wantMin     time.Duration
		wantMax     time.Duration
	}{
		{
			name:        "first attempt no jitter",
			attempt:     0,
			baseDelay:   100 * time.Millisecond,
			maxDelay:    10 * time.Second,
			jitterDelay: 0,
			wantMin:     100 * time.Millisecond,
			wantMax:     100 * time.Millisecond,
		},
		{
			name:        "second attempt doubles",
			attempt:     1,
			baseDelay:   100 * time.Millisecond,
			maxDelay:    10 * time.Second,
			jitterDelay: 0,
			wantMin:     200 * time.Millisecond,
			wantMax:     200 * time.Millisecond,
		},
		{
			name:        "capped at max",
			attempt:     10,
			baseDelay:   100 * time.Millisecond,
			maxDelay:    1 * time.Second,
			jitterDelay: 0,
			wantMin:     1 * time.Second,
			wantMax:     1 * time.Second,
		},
		{
			name:        "with jitter",
			attempt:     0,
			baseDelay:   100 * time.Millisecond,
			maxDelay:    10 * time.Second,
			jitterDelay: 50 * time.Millisecond,
			wantMin:     100 * time.Millisecond,
			wantMax:     150 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateBackoff(tt.attempt, tt.baseDelay, tt.maxDelay, tt.jitterDelay)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("CalculateBackoff() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestDefaultBreakerConfigFallback(t *testing.T) {
	original := DefaultIsSuccessful
	DefaultIsSuccessful = nil
	defer func() { DefaultIsSuccessful = original }()

	cfg := DefaultBreakerConfig("fallback-test")
	if cfg.IsSuccessful == nil {
		t.Fatal("expected IsSuccessful to have fallback")
	}

	if !cfg.IsSuccessful(nil) {
		t.Error("fallback should return true for nil error")
	}
	if cfg.IsSuccessful(errors.New("fail")) {
		t.Error("fallback should return false for non-nil error")
	}
}
