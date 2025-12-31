package resilience

import (
	"context"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/sony/gobreaker"
)

type RetryConfig struct {
	MaxRetries  int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	JitterDelay time.Duration
	ShouldRetry func(resp *http.Response, err error) bool
}

var DefaultRetryConfig = RetryConfig{
	MaxRetries:  3,
	BaseDelay:   500 * time.Millisecond,
	MaxDelay:    30 * time.Second,
	JitterDelay: 250 * time.Millisecond,
	ShouldRetry: func(resp *http.Response, err error) bool {
		if err != nil {
			return true
		}
		if resp == nil {
			return false
		}
		return resp.StatusCode == 429 || resp.StatusCode >= 500
	},
}

type BreakerConfig struct {
	Name             string
	MaxRequests      uint32
	Interval         time.Duration
	Timeout          time.Duration
	FailureThreshold uint32
	FailureRatio     float64
	MinRequests      uint32
	OnStateChange    func(name string, from, to gobreaker.State)
	IsSuccessful     func(err error) bool
}

// DefaultIsSuccessful is a callback to determine if an error should count as
// a circuit breaker failure. User errors should NOT trip the breaker.
// Set this from provider package during init to avoid import cycles.
var DefaultIsSuccessful func(err error) bool

func DefaultBreakerConfig(name string) BreakerConfig {
	isSuccessful := DefaultIsSuccessful
	if isSuccessful == nil {
		// Fallback: only nil errors are successful
		isSuccessful = func(err error) bool { return err == nil }
	}
	return BreakerConfig{
		Name:             name,
		MaxRequests:      3,
		Interval:         10 * time.Second,
		Timeout:          30 * time.Second,
		FailureThreshold: 5,
		FailureRatio:     0.5,
		MinRequests:      10,
		IsSuccessful:     isSuccessful,
	}
}

type CircuitBreaker struct {
	cb *gobreaker.CircuitBreaker
}

func NewCircuitBreaker(cfg BreakerConfig) *CircuitBreaker {
	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < cfg.MinRequests {
				return false
			}
			if counts.ConsecutiveFailures >= cfg.FailureThreshold {
				return true
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= cfg.FailureRatio
		},
		OnStateChange: cfg.OnStateChange,
		IsSuccessful:  cfg.IsSuccessful,
	}
	return &CircuitBreaker{cb: gobreaker.NewCircuitBreaker(settings)}
}

func (c *CircuitBreaker) Execute(fn func() (any, error)) (any, error) {
	return c.cb.Execute(fn)
}

func (c *CircuitBreaker) State() gobreaker.State {
	return c.cb.State()
}

func (c *CircuitBreaker) Counts() gobreaker.Counts {
	return c.cb.Counts()
}

func (c *CircuitBreaker) Name() string {
	return c.cb.Name()
}

func NewRetryPolicy[R any](cfg RetryConfig) retrypolicy.RetryPolicy[R] {
	builder := retrypolicy.NewBuilder[R]().
		WithMaxRetries(cfg.MaxRetries).
		WithBackoff(cfg.BaseDelay, cfg.MaxDelay)
	if cfg.JitterDelay > 0 {
		builder = builder.WithJitter(cfg.JitterDelay)
	}
	return builder.Build()
}

type Executor[R any] struct {
	executor failsafe.Executor[R]
	breaker  *CircuitBreaker
}

func NewExecutor[R any](retryConfig RetryConfig, breakerConfig *BreakerConfig) *Executor[R] {
	rp := NewRetryPolicy[R](retryConfig)

	var breaker *CircuitBreaker
	if breakerConfig != nil {
		breaker = NewCircuitBreaker(*breakerConfig)
	}

	return &Executor[R]{
		executor: failsafe.With(rp),
		breaker:  breaker,
	}
}

func (e *Executor[R]) Execute(ctx context.Context, fn func() (R, error)) (R, error) {
	if e.breaker != nil {
		result, err := e.breaker.Execute(func() (any, error) {
			return e.executor.WithContext(ctx).Get(fn)
		})
		if err != nil {
			var zero R
			return zero, err
		}
		return result.(R), nil
	}
	return e.executor.WithContext(ctx).Get(fn)
}

func (e *Executor[R]) CircuitBreaker() *CircuitBreaker {
	return e.breaker
}

func CalculateBackoff(attempt int, baseDelay, maxDelay, jitterDelay time.Duration) time.Duration {
	delay := baseDelay * time.Duration(1<<attempt)
	if delay > maxDelay {
		delay = maxDelay
	}
	if jitterDelay > 0 {
		jitterAmount := time.Duration(rand.Int64N(int64(jitterDelay)))
		delay += jitterAmount
		if delay > maxDelay {
			delay = maxDelay
		}
	}
	return delay
}

func WaitWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
