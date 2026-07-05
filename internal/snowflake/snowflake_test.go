package snowflake

import (
	"sync"
	"testing"
	"time"
)

func TestNodeRange(t *testing.T) {
	if _, err := New(-1); err != ErrNodeRange {
		t.Fatalf("expected ErrNodeRange for -1, giot %v", err)
	}
	if _, err := New(1024); err != ErrNodeRange {
		t.Fatalf("expected ErrNodeRange for 1024, got %v", err)
	}
	if _, err := New(1023); err != nil {
		t.Fatalf("expected ok for 1023, got %v", err)
	}
}

func TestMonotonicAndUnique(t *testing.T) {
	g, _ := New(7)
	const n = 20_000
	seen := make(map[int64]struct{}, n)
	var prev int64
	for i := range n {
		id, err := g.Next()
		if err != nil {
			t.Fatalf("Next failed at %d: %v", i, err)
		}
		if id <= prev {
			t.Fatalf("ids not strictly increasing: %d then %d", prev, id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %d", id)
		}
		seen[id] = struct{}{}
		prev = id
	}
}

func TestConcurrentUnique(t *testing.T) {
	g, _ := New(3)
	const workers = 16
	const each = 20_000

	var mu sync.Mutex
	seen := make(map[int64]struct{}, workers*each)

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			local := make([]int64, each)
			for i := range each {
				id, err := g.Next()
				if err != nil {
					t.Errorf("Next failed: %v", err)
					return
				}
				local[i] = id
			}
			mu.Lock()
			for _, id := range local {
				if _, dup := seen[id]; dup {
					t.Errorf("duplicate id %d across goroutines", id)
				}
				seen[id] = struct{}{}
			}
			mu.Unlock()
		})
	}
	wg.Wait()

	if len(seen) != workers*each {
		t.Fatalf("expected %d unique ids, got %d", workers*each, len(seen))
	}
}

func TestFieldRoundTrip(t *testing.T) {
	g, _ := New(513)
	before := time.Now().UnixMilli()
	id, _ := g.Next()
	after := time.Now().UnixMilli()

	if got := NodeOf(id); got != 513 {
		t.Fatalf("NodeOf = %d, want 513", got)
	}
	ms := TimeOf(id).UnixMilli()
	if ms < before || ms > after {
		t.Fatalf("TimeOf %d outside [%d,%d]", ms, before, after)
	}
	if seq := SeqOf(id); seq < 0 || seq > maxSeq {
		t.Fatalf("SeqOf %d out of range", seq)
	}
}
