package streamutil

import (
	"sync"
)

// BufferPool provides reusable byte buffers to reduce allocations.
// Uses sync.Pool with size classes for efficient buffer reuse.
type BufferPool struct {
	small  sync.Pool // <= 4KB
	medium sync.Pool // <= 64KB
	large  sync.Pool // <= 1MB
}

const (
	smallBufferSize  = 4 * 1024    // 4KB
	mediumBufferSize = 64 * 1024   // 64KB
	largeBufferSize  = 1024 * 1024 // 1MB
)

// NewBufferPool creates a new buffer pool with size classes.
func NewBufferPool() *BufferPool {
	return &BufferPool{
		small: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, 0, smallBufferSize)
				return &buf
			},
		},
		medium: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, 0, mediumBufferSize)
				return &buf
			},
		},
		large: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, 0, largeBufferSize)
				return &buf
			},
		},
	}
}

// Get returns a buffer of at least the requested size.
// The returned buffer has length 0 but sufficient capacity.
func (p *BufferPool) Get(size int) *[]byte {
	if size <= smallBufferSize {
		return p.small.Get().(*[]byte)
	}
	if size <= mediumBufferSize {
		return p.medium.Get().(*[]byte)
	}
	if size <= largeBufferSize {
		return p.large.Get().(*[]byte)
	}
	// For very large buffers, allocate directly
	buf := make([]byte, 0, size)
	return &buf
}

// Put returns a buffer to the pool.
// The buffer is reset to zero length before being stored.
func (p *BufferPool) Put(buf *[]byte) {
	if buf == nil {
		return
	}

	cap := cap(*buf)
	*buf = (*buf)[:0] // Reset length

	if cap <= smallBufferSize {
		p.small.Put(buf)
	} else if cap <= mediumBufferSize {
		p.medium.Put(buf)
	} else if cap <= largeBufferSize {
		p.large.Put(buf)
	}
	// Very large buffers are not pooled
}

// Global default buffer pool
var defaultBufferPool = NewBufferPool()

// GetBuffer returns a buffer from the default pool.
func GetBuffer(size int) *[]byte {
	return defaultBufferPool.Get(size)
}

// PutBuffer returns a buffer to the default pool.
func PutBuffer(buf *[]byte) {
	defaultBufferPool.Put(buf)
}

// ChunkPool provides reusable Chunk structs.
type ChunkPool struct {
	pool sync.Pool
}

// NewChunkPool creates a new chunk pool.
func NewChunkPool() *ChunkPool {
	return &ChunkPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &Chunk{}
			},
		},
	}
}

// Get returns a chunk from the pool.
func (p *ChunkPool) Get() *Chunk {
	return p.pool.Get().(*Chunk)
}

// Put returns a chunk to the pool after resetting it.
func (p *ChunkPool) Put(c *Chunk) {
	if c == nil {
		return
	}
	c.Data = nil
	c.Err = nil
	p.pool.Put(c)
}

// Global chunk pool
var defaultChunkPool = NewChunkPool()

// GetChunk returns a chunk from the default pool.
func GetChunk() *Chunk {
	return defaultChunkPool.Get()
}

// PutChunk returns a chunk to the default pool.
func PutChunk(c *Chunk) {
	defaultChunkPool.Put(c)
}
