package streamutil

import (
	"bufio"
	"context"
	"io"
	"time"
)

// StreamReaderConfig configures the optimized stream reader.
type StreamReaderConfig struct {
	// IdleTimeout for stalled connection detection (default: 5 minutes)
	IdleTimeout time.Duration
	// BufferSize for the scanner (default: 64KB)
	BufferSize int
	// MaxLineSize limit (default: 2MB)
	MaxLineSize int
	// Name for logging purposes
	Name string
}

// DefaultStreamReaderConfig returns sensible defaults.
func DefaultStreamReaderConfig() StreamReaderConfig {
	return StreamReaderConfig{
		IdleTimeout: 5 * time.Minute,
		BufferSize:  64 * 1024,
		MaxLineSize: 2 * 1024 * 1024,
		Name:        "stream",
	}
}

// OptimizedStreamReader wraps an io.ReadCloser with context awareness
// and idle detection using the shared IdleWatcher.
type OptimizedStreamReader struct {
	body      io.ReadCloser
	ctx       context.Context
	touch     func()
	done      func()
	closed    bool
	closeOnce func()
}

// NewOptimizedStreamReader creates a stream reader using the shared idle watcher.
// This eliminates the need for 2 goroutines per stream.
func NewOptimizedStreamReader(ctx context.Context, body io.ReadCloser, cfg StreamReaderConfig) *OptimizedStreamReader {
	watcher := DefaultIdleWatcher()

	r := &OptimizedStreamReader{
		body: body,
		ctx:  ctx,
	}

	// Register with shared idle watcher
	r.touch, r.done = watcher.Register(ctx, cfg.IdleTimeout, func() {
		// On idle timeout, close body to unblock Read
		body.Close()
	})

	// Setup close once
	var closed bool
	r.closeOnce = func() {
		if !closed {
			closed = true
			r.done()
			body.Close()
		}
	}

	return r
}

// Read implements io.Reader with activity tracking.
func (r *OptimizedStreamReader) Read(p []byte) (n int, err error) {
	// Check context before blocking read
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
	}

	n, err = r.body.Read(p)
	if n > 0 {
		r.touch() // Update activity timestamp
	}
	return n, err
}

// Close implements io.Closer.
func (r *OptimizedStreamReader) Close() error {
	r.closeOnce()
	return nil
}

// LineScanner provides line-by-line reading with pooled buffers.
type LineScanner struct {
	reader  *OptimizedStreamReader
	scanner *bufio.Scanner
	buf     *[]byte
}

// NewLineScanner creates a scanner for line-by-line reading.
func NewLineScanner(ctx context.Context, body io.ReadCloser, cfg StreamReaderConfig) *LineScanner {
	reader := NewOptimizedStreamReader(ctx, body, cfg)

	// Get pooled buffer
	buf := GetBuffer(cfg.BufferSize)

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(*buf, cfg.MaxLineSize)

	return &LineScanner{
		reader:  reader,
		scanner: scanner,
		buf:     buf,
	}
}

// Scan advances to the next line. Returns false when done or on error.
func (s *LineScanner) Scan() bool {
	return s.scanner.Scan()
}

// Bytes returns the current line bytes.
func (s *LineScanner) Bytes() []byte {
	return s.scanner.Bytes()
}

// Text returns the current line as string.
func (s *LineScanner) Text() string {
	return s.scanner.Text()
}

// Err returns any error that occurred during scanning.
func (s *LineScanner) Err() error {
	return s.scanner.Err()
}

// Close closes the scanner and returns the buffer to the pool.
func (s *LineScanner) Close() error {
	PutBuffer(s.buf)
	return s.reader.Close()
}
