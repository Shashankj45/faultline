package proxy

import "sync"

// BytePool implements httputil.BufferPool using sync.Pool for zero-allocation stream copying.
type BytePool struct {
	pool sync.Pool
	size int
}

// NewBytePool creates a buffer pool with a fixed buffer size (typically 32KB).
func NewBytePool(size int) *BytePool {
	if size <= 0 {
		size = 32 * 1024 // 32KB default
	}
	return &BytePool{
		size: size,
		pool: sync.Pool{
			New: func() any {
				b := make([]byte, size)
				return b
			},
		},
	}
}

// Get returns a buffer from the pool.
func (p *BytePool) Get() []byte {
	return p.pool.Get().([]byte)
}

// Put returns a buffer to the pool.
func (p *BytePool) Put(b []byte) {
	if len(b) == p.size {
		p.pool.Put(b)
	}
}
