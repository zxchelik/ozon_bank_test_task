package memory

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxchelik/ozon_bank_test_task/internal/storage"
)

func TestCreateOrGetAndCollision(t *testing.T) {
	s := New()
	first, created, err := s.CreateOrGet(context.Background(), "https://one.example", "abcdefghij")
	if err != nil || !created {
		t.Fatalf("first create = (%+v, %v, %v)", first, created, err)
	}
	existing, created, err := s.CreateOrGet(context.Background(), "https://one.example", "zzzzzzzzzz")
	if err != nil || created || existing.ShortCode != first.ShortCode {
		t.Fatalf("idempotent create = (%+v, %v, %v)", existing, created, err)
	}
	if _, _, err := s.CreateOrGet(context.Background(), "https://two.example", "abcdefghij"); !errors.Is(err, storage.ErrShortCodeConflict) {
		t.Fatalf("collision error = %v", err)
	}
}

func TestResolveAndStats(t *testing.T) {
	createdAt := time.Date(2026, 7, 14, 10, 0, 0, 0, time.FixedZone("test", 3*60*60))
	accessedAt := createdAt.Add(time.Minute)
	calls := 0
	s := NewWithClock(func() time.Time {
		calls++
		if calls == 1 {
			return createdAt
		}
		return accessedAt
	})
	_, _, _ = s.CreateOrGet(context.Background(), "https://example.com", "abcdefghij")
	before, err := s.GetStatsByCode(context.Background(), "abcdefghij")
	if err != nil {
		t.Fatal(err)
	}
	if before.AccessCount != 0 || before.LastAccessedAt != nil || before.CreatedAt.Location() != time.UTC {
		t.Fatalf("initial stats = %+v", before)
	}
	resolved, err := s.ResolveByCode(context.Background(), "abcdefghij")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.AccessCount != 1 || resolved.LastAccessedAt == nil || !resolved.LastAccessedAt.Equal(accessedAt) {
		t.Fatalf("resolved = %+v", resolved)
	}
	after, _ := s.GetStatsByCode(context.Background(), "abcdefghij")
	afterAgain, _ := s.GetStatsByCode(context.Background(), "abcdefghij")
	if after.AccessCount != 1 || afterAgain.AccessCount != 1 {
		t.Fatalf("stats incremented counter: %d/%d", after.AccessCount, afterAgain.AccessCount)
	}
	if _, err := s.GetStatsByCode(context.Background(), "missing0000"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("not found error = %v", err)
	}
}

func TestConcurrentResolve(t *testing.T) {
	s := New()
	_, _, _ = s.CreateOrGet(context.Background(), "https://example.com", "abcdefghij")
	const workers = 250
	var wg sync.WaitGroup
	var failures atomic.Int64
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			if _, err := s.ResolveByCode(context.Background(), "abcdefghij"); err != nil {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	stats, err := s.GetStatsByCode(context.Background(), "abcdefghij")
	if err != nil {
		t.Fatal(err)
	}
	if failures.Load() != 0 || stats.AccessCount != workers {
		t.Fatalf("failures/count = %d/%d, want 0/%d", failures.Load(), stats.AccessCount, workers)
	}
}

func TestConcurrentCreateSameOriginal(t *testing.T) {
	s := New()
	const workers = 100
	var created atomic.Int64
	var wg sync.WaitGroup
	codes := make(chan string, workers)
	wg.Add(workers)
	for i := range workers {
		go func() {
			defer wg.Done()
			code := "abcdefghij"
			if i > 0 {
				code = "ABCDEFGHIJ"
			}
			link, wasCreated, err := s.CreateOrGet(context.Background(), "https://example.com", code)
			if err != nil {
				t.Errorf("CreateOrGet() error = %v", err)
				return
			}
			if wasCreated {
				created.Add(1)
			}
			codes <- link.ShortCode
		}()
	}
	wg.Wait()
	close(codes)
	var first string
	for code := range codes {
		if first == "" {
			first = code
		}
		if code != first {
			t.Fatalf("different codes: %q and %q", first, code)
		}
	}
	if created.Load() != 1 {
		t.Fatalf("created count = %d, want 1", created.Load())
	}
}
