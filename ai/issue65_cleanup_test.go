package ai_test

import (
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestSessionResourceCleanupOrderAndFailures(t *testing.T) {
	var calls []string
	failure := errors.New("resource failed")
	register := func(cleanup ai.SessionResourceCleanup) func() {
		t.Helper()
		remove, err := ai.RegisterSessionResourceCleanup(cleanup)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(remove)
		return remove
	}
	register(func(ids ...string) { calls = append(calls, "first:"+ids[0]) })
	remove := register(func(...string) { calls = append(calls, "failure"); panic(failure) })
	register(func(ids ...string) { calls = append(calls, "last:"+ids[0]) })
	for range 2 {
		if err := ai.CleanupSessionResources("one"); !errors.Is(err, failure) {
			t.Fatalf("cleanup error = %v", err)
		}
	}
	remove()
	remove()
	if err := ai.CleanupSessionResources("two"); err != nil {
		t.Fatal(err)
	}
	want := []string{"first:one", "failure", "last:one", "first:one", "failure", "last:one", "first:two", "last:two"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestSessionResourceCleanupSnapshotAndGlobal(t *testing.T) {
	var calls []string
	var removeLast, removeAdded func()
	removeFirst, err := ai.RegisterSessionResourceCleanup(func(ids ...string) {
		if len(ids) != 0 {
			t.Fatalf("global cleanup ids = %v", ids)
		}
		calls = append(calls, "first")
		removeLast()
		if removeAdded == nil {
			removeAdded, _ = ai.RegisterSessionResourceCleanup(func(...string) { calls = append(calls, "added") })
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer removeFirst()
	defer func() {
		if removeAdded != nil {
			removeAdded()
		}
	}()
	removeLast, err = ai.RegisterSessionResourceCleanup(func(...string) { calls = append(calls, "last") })
	if err != nil {
		t.Fatal(err)
	}
	defer removeLast()
	for range 2 {
		if err := ai.CleanupSessionResources(); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(calls, []string{"first", "last", "first", "added"}) {
		t.Fatalf("calls = %v", calls)
	}
	if remove, err := ai.RegisterSessionResourceCleanup(nil); err == nil || remove != nil {
		t.Fatal("nil cleanup accepted")
	}
}

func TestSessionResourceCleanupConcurrentRegistration(t *testing.T) {
	var calls atomic.Int64
	remove, err := ai.RegisterSessionResourceCleanup(func(...string) { calls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	defer remove()
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 25 {
				remove, err := ai.RegisterSessionResourceCleanup(func(...string) {})
				if err != nil {
					t.Error(err)
					return
				}
				if err := ai.CleanupSessionResources("concurrent"); err != nil {
					t.Error(err)
				}
				remove()
			}
		}()
	}
	workers.Wait()
	if calls.Load() != 200 {
		t.Fatalf("calls = %d", calls.Load())
	}
}
