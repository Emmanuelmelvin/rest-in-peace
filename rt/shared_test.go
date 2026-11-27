package rt

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// TestSharedLoadStore verifies basic load and store under mutual exclusion.
func TestSharedLoadStore(t *testing.T) {
	var s Shared[int64]

	if got := s.Load(); got != 0 {
		t.Fatalf("zero value Load() = %d, want 0", got)
	}
	s.Store(42)
	if got := s.Load(); got != 42 {
		t.Fatalf("Load() = %d, want 42", got)
	}
	s.Store(99)
	if got := s.Load(); got != 99 {
		t.Fatalf("Load() after second Store() = %d, want 99", got)
	}
}

// TestSharedModify verifies the read-modify-write cycle.
func TestSharedModify(t *testing.T) {
	var s Shared[int64]
	s.Store(10)
	s.Modify(func(v int64) int64 { return v + 5 })
	if got := s.Load(); got != 15 {
		t.Fatalf("Load() after Modify() = %d, want 15", got)
	}
}

// TestSharedConcurrentModify is a goroutine stress test that verifies no
// updates are lost under concurrent access. Each of N goroutines increments
// the counter M times; the final value must be N*M.
//
// Run with -race to detect data races.
func TestSharedConcurrentModify(t *testing.T) {
	const (
		numGoroutines = 50
		increments    = 200
	)

	var s Shared[int64]
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < increments; j++ {
				s.Modify(func(v int64) int64 { return v + 1 })
			}
		}()
	}
	wg.Wait()

	want := int64(numGoroutines * increments)
	if got := s.Load(); got != want {
		t.Fatalf("concurrent Modify: got %d, want %d", got, want)
	}
}

// TestSharedConcurrentStore checks that concurrent Store calls are safe
// (no panic, no data corruption visible via Load).
func TestSharedConcurrentStore(t *testing.T) {
	var s Shared[string]
	var wg sync.WaitGroup
	const n = 100
	wg.Add(n)
	var stored atomic.Int32
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			s.Store("hello")
			stored.Add(1)
		}()
	}
	wg.Wait()

	if got := s.Load(); got != "hello" {
		t.Fatalf("Load() after concurrent stores = %q, want %q", got, "hello")
	}
	if stored.Load() != n {
		t.Fatalf("not all goroutines ran")
	}
}

// TestSharedRTValue tests Shared[rt.Value] (the untyped BASIC case).
func TestSharedRTValue(t *testing.T) {
	var s Shared[Value]

	if s.Load() != nil {
		t.Fatal("zero Value should be nil")
	}
	s.Store("HELLO")
	if got := s.Load(); got != "HELLO" {
		t.Fatalf("Load() = %v, want HELLO", got)
	}
	// Compound update via Modify.
	s.Store(int64(5))
	s.Modify(func(v Value) Value {
		return Add(v, int64(3))
	})
	if got := s.Load(); got != int64(8) {
		t.Fatalf("Modify on rt.Value: Load() = %v (%T), want 8 (int64)", got, got)
	}
}

// TestSharedConcurrentMap verifies concurrent SetMember/GetMember and
// SetIndex/GetIndex on a Shared[Value] holding a *Map.
func TestSharedConcurrentMap(t *testing.T) {
	var s Shared[Value]
	s.Store(NewMap())

	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("k%d", i)
			s.SetIndex(key, int64(i))
			if got := s.GetIndex(key); got != int64(i) {
				t.Errorf("GetIndex(%q) = %v, want %d", key, got, i)
			}
			mkey := fmt.Sprintf("m%d", i)
			s.SetMember(mkey, key)
			if got := s.GetMember(mkey); got != key {
				t.Errorf("GetMember(%q) = %v, want %q", mkey, got, key)
			}
		}()
	}
	wg.Wait()
}

// TestSharedConcurrentArray verifies concurrent SetIndex/GetIndex on a
// Shared[Value] holding an *Array.
func TestSharedConcurrentArray(t *testing.T) {
	var s Shared[Value]
	const workers = 16
	init := make([]Value, workers)
	for i := range init {
		init[i] = int64(0)
	}
	s.Store(ArrayOf(init...))

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			s.SetIndex(int64(i), int64(i*10))
			if got := s.GetIndex(int64(i)); got != int64(i*10) {
				t.Errorf("GetIndex(%d) = %v, want %d", i, got, i*10)
			}
		}()
	}
	wg.Wait()
}
