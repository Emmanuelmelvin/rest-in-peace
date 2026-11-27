package rt

import "sync"

// Shared is a mutex-guarded holder for a module-level variable that is safe
// for concurrent access from multiple goroutines (Requirement 14.4, task 4.3,
// design §4.11).
//
// Generated code declares a Shared[T] package variable for every SHARED DIM,
// then calls Load, Store, Modify, GetMember, SetMember, GetIndex, or SetIndex
// through these methods so reads and writes stay race-free across concurrent
// HTTP handlers.
//
// Callers: codegen/shared.go.
type Shared[T any] struct {
	mu  sync.Mutex
	val T
}

// Load returns the current value under the mutex.
func (s *Shared[T]) Load() T {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.val
}

// Store sets the value under the mutex.
func (s *Shared[T]) Store(v T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.val = v
}

// Modify calls f while holding the mutex. f receives the current value and
// returns the new one. Use this for compound updates (x = x + 1) so the
// read-modify-write cycle is atomic under concurrent requests.
func (s *Shared[T]) Modify(f func(T) T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.val = f(s.val)
}

// GetMember reads a named member of the shared value while holding the mutex.
func (s *Shared[T]) GetMember(key string) Value {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Get(s.val, key)
}

// SetMember sets a named member on the shared value while holding the mutex.
func (s *Shared[T]) SetMember(key string, val Value) {
	s.mu.Lock()
	defer s.mu.Unlock()
	SetMember(s.val, key, val)
}

// GetIndex reads an indexed element of the shared value while holding the mutex.
func (s *Shared[T]) GetIndex(i Value) Value {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Index(s.val, i)
}

// SetIndex sets an indexed element on the shared value while holding the mutex.
func (s *Shared[T]) SetIndex(i Value, val Value) {
	s.mu.Lock()
	defer s.mu.Unlock()
	SetIndex(s.val, i, val)
}
