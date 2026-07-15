package service

import (
	"context"
	"errors"
	"testing"

	"github.com/zxchelik/ozon_bank_test_task/internal/model"
	"github.com/zxchelik/ozon_bank_test_task/internal/storage"
	"github.com/zxchelik/ozon_bank_test_task/internal/storage/memory"
)

func TestCreateIsIdempotentByOriginalURL(t *testing.T) {
	generator := &sequenceGenerator{codes: []string{"abcdefghij", "ABCDEFGHIJ"}}
	svc := New(memory.New(), generator, 5)
	first, created, err := svc.Create(context.Background(), "https://example.com/path")
	if err != nil || !created {
		t.Fatalf("first Create() = (%+v, %v, %v), want created", first, created, err)
	}
	second, created, err := svc.Create(context.Background(), "https://example.com/path")
	if err != nil || created {
		t.Fatalf("second Create() = (%+v, %v, %v), want existing", second, created, err)
	}
	if second.ShortCode != first.ShortCode {
		t.Fatalf("second code = %q, want %q", second.ShortCode, first.ShortCode)
	}
}

func TestCreateRetriesShortCodeCollision(t *testing.T) {
	store := &collisionStorage{collisions: 1}
	generator := &sequenceGenerator{codes: []string{"aaaaaaaaaa", "bbbbbbbbbb"}}
	svc := New(store, generator, 3)
	link, created, err := svc.Create(context.Background(), "https://example.com")
	if err != nil || !created {
		t.Fatalf("Create() = (%+v, %v, %v), want success", link, created, err)
	}
	if link.ShortCode != "bbbbbbbbbb" || store.calls != 2 {
		t.Fatalf("code/calls = %q/%d, want bbbbbbbbbb/2", link.ShortCode, store.calls)
	}
}

func TestCreateStopsAfterCollisionLimit(t *testing.T) {
	store := &collisionStorage{collisions: 10}
	svc := New(store, &sequenceGenerator{codes: []string{"aaaaaaaaaa", "bbbbbbbbbb"}}, 2)
	_, _, err := svc.Create(context.Background(), "https://example.com")
	if !errors.Is(err, ErrGenerationExhausted) {
		t.Fatalf("Create() error = %v, want %v", err, ErrGenerationExhausted)
	}
	if store.calls != 2 {
		t.Fatalf("storage calls = %d, want 2", store.calls)
	}
}

func TestValidation(t *testing.T) {
	svc := New(memory.New(), &sequenceGenerator{codes: []string{"abcdefghij"}}, 1)
	invalidURLs := []string{"", "example.com", "ftp://example.com", " https://example.com"}
	for _, raw := range invalidURLs {
		if _, _, err := svc.Create(context.Background(), raw); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("Create(%q) error = %v, want %v", raw, err, ErrInvalidURL)
		}
	}
	invalidCodes := []string{"short", "abcdefghijk", "abcdefghi-", "абвгдеёжзи"}
	for _, code := range invalidCodes {
		if _, err := svc.Resolve(context.Background(), code); !errors.Is(err, ErrInvalidShortCode) {
			t.Errorf("Resolve(%q) error = %v, want %v", code, err, ErrInvalidShortCode)
		}
	}
}

func TestResolveAndStatsSemantics(t *testing.T) {
	store := memory.New()
	svc := New(store, &sequenceGenerator{codes: []string{"abcdefghij"}}, 1)
	link, _, err := svc.Create(context.Background(), "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := svc.Resolve(context.Background(), link.ShortCode)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.AccessCount != 1 || resolved.LastAccessedAt == nil {
		t.Fatalf("Resolve() stats = count %d, last %v", resolved.AccessCount, resolved.LastAccessedAt)
	}
	stats, err := svc.Stats(context.Background(), link.ShortCode)
	if err != nil {
		t.Fatal(err)
	}
	statsAgain, err := svc.Stats(context.Background(), link.ShortCode)
	if err != nil {
		t.Fatal(err)
	}
	if stats.AccessCount != 1 || statsAgain.AccessCount != 1 {
		t.Fatalf("Stats() changed counter: %d then %d", stats.AccessCount, statsAgain.AccessCount)
	}
	if _, err := svc.Resolve(context.Background(), "zzzzzzzzzz"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Resolve(not found) error = %v", err)
	}
}

type sequenceGenerator struct {
	codes []string
	next  int
}

func (g *sequenceGenerator) Generate() (string, error) {
	if g.next >= len(g.codes) {
		return g.codes[len(g.codes)-1], nil
	}
	code := g.codes[g.next]
	g.next++
	return code, nil
}

type collisionStorage struct {
	collisions int
	calls      int
}

func (s *collisionStorage) CreateOrGet(_ context.Context, originalURL, shortCode string) (model.Link, bool, error) {
	s.calls++
	if s.calls <= s.collisions {
		return model.Link{}, false, storage.ErrShortCodeConflict
	}
	return model.Link{OriginalURL: originalURL, ShortCode: shortCode}, true, nil
}
func (*collisionStorage) ResolveByCode(context.Context, string) (model.Link, error) {
	return model.Link{}, storage.ErrNotFound
}
func (*collisionStorage) GetStatsByCode(context.Context, string) (model.Link, error) {
	return model.Link{}, storage.ErrNotFound
}
