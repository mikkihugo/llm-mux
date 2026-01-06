package streamutil

import (
	"context"
	"sync"
)

// ResultRecorder provides async result recording to avoid blocking the hot path.
// Uses a buffered channel and worker goroutine pattern.
type ResultRecorder[T any] struct {
	queue    chan T
	handler  func(T)
	stopCh   chan struct{}
	wg       sync.WaitGroup
	workers  int
	queueLen int
}

// ResultRecorderConfig configures the result recorder.
type ResultRecorderConfig struct {
	// QueueSize is the buffer size for pending results (default: 1024)
	QueueSize int
	// Workers is the number of worker goroutines (default: 4)
	Workers int
}

// DefaultResultRecorderConfig returns sensible defaults.
func DefaultResultRecorderConfig() ResultRecorderConfig {
	return ResultRecorderConfig{
		QueueSize: 1024,
		Workers:   4,
	}
}

// NewResultRecorder creates a new async result recorder.
// The handler function is called for each result in a worker goroutine.
func NewResultRecorder[T any](cfg ResultRecorderConfig, handler func(T)) *ResultRecorder[T] {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1024
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}

	r := &ResultRecorder[T]{
		queue:    make(chan T, cfg.QueueSize),
		handler:  handler,
		stopCh:   make(chan struct{}),
		workers:  cfg.Workers,
		queueLen: cfg.QueueSize,
	}

	// Start worker goroutines
	r.wg.Add(cfg.Workers)
	for i := 0; i < cfg.Workers; i++ {
		go r.worker()
	}

	return r
}

// Record queues a result for async processing.
// Returns false if the recorder is stopped or queue is full.
func (r *ResultRecorder[T]) Record(result T) bool {
	select {
	case r.queue <- result:
		return true
	case <-r.stopCh:
		return false
	default:
		// Queue full - try non-blocking send with context awareness
		select {
		case r.queue <- result:
			return true
		case <-r.stopCh:
			return false
		}
	}
}

// RecordWithContext queues a result with context cancellation support.
func (r *ResultRecorder[T]) RecordWithContext(ctx context.Context, result T) bool {
	select {
	case r.queue <- result:
		return true
	case <-ctx.Done():
		return false
	case <-r.stopCh:
		return false
	}
}

func (r *ResultRecorder[T]) worker() {
	defer r.wg.Done()
	for {
		select {
		case result, ok := <-r.queue:
			if !ok {
				return
			}
			r.handler(result)
		case <-r.stopCh:
			// Drain remaining items
			for {
				select {
				case result, ok := <-r.queue:
					if !ok {
						return
					}
					r.handler(result)
				default:
					return
				}
			}
		}
	}
}

// Stop stops the recorder and waits for pending results to be processed.
func (r *ResultRecorder[T]) Stop() {
	close(r.stopCh)
	close(r.queue)
	r.wg.Wait()
}

// Pending returns the number of pending results in the queue.
func (r *ResultRecorder[T]) Pending() int {
	return len(r.queue)
}
