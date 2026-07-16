// Package ring provides a fixed-capacity FIFO ring buffer that
// overwrites its oldest element once full. It is deliberately
// unsynchronised: callers needing concurrency safety wrap it in their
// own mutex (as the logs and sysmon packages do).
package ring

// Ring holds at most Cap() elements, evicting the oldest on overflow.
// The zero value is unusable — construct with New.
type Ring[T any] struct {
	buf  []T
	head int // index of the oldest element once the buffer has filled.
	len  int
}

// New returns a Ring holding at most capacity elements. A capacity < 1
// is clamped to 1 so the buffer is always usable.
func New[T any](capacity int) *Ring[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &Ring[T]{buf: make([]T, capacity)}
}

// Len reports how many elements the ring currently holds (0..Cap).
func (r *Ring[T]) Len() int { return r.len }

// Cap reports the ring's fixed capacity.
func (r *Ring[T]) Cap() int { return len(r.buf) }

// Push appends v, evicting the oldest element once the ring is full.
func (r *Ring[T]) Push(v T) {
	if r.len < len(r.buf) {
		r.buf[(r.head+r.len)%len(r.buf)] = v
		r.len++
		return
	}
	r.buf[r.head] = v
	r.head = (r.head + 1) % len(r.buf)
}

// Items returns the ring contents oldest-first as a fresh slice; the
// caller owns the result.
func (r *Ring[T]) Items() []T {
	out := make([]T, 0, r.len)
	for i := 0; i < r.len; i++ {
		out = append(out, r.buf[(r.head+i)%len(r.buf)])
	}
	return out
}
