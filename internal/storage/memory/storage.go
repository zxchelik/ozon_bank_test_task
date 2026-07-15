package memory

import (
	"context"
	"sync"
	"time"

	"github.com/zxchelik/ozon_bank_test_task/internal/model"
	"github.com/zxchelik/ozon_bank_test_task/internal/storage"
)

type Storage struct {
	mu             sync.RWMutex
	byCode         map[string]model.Link
	codeByOriginal map[string]string
	now            func() time.Time
}

func New() *Storage {
	return NewWithClock(time.Now)
}

func NewWithClock(now func() time.Time) *Storage {
	return &Storage{
		byCode:         make(map[string]model.Link),
		codeByOriginal: make(map[string]string),
		now:            now,
	}
}

func (s *Storage) CreateOrGet(_ context.Context, originalURL, shortCode string) (model.Link, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existingCode, ok := s.codeByOriginal[originalURL]; ok {
		return clone(s.byCode[existingCode]), false, nil
	}
	if _, ok := s.byCode[shortCode]; ok {
		return model.Link{}, false, storage.ErrShortCodeConflict
	}
	link := model.Link{
		OriginalURL: originalURL,
		ShortCode:   shortCode,
		CreatedAt:   s.now().UTC(),
	}
	s.byCode[shortCode] = link
	s.codeByOriginal[originalURL] = shortCode
	return clone(link), true, nil
}

func (s *Storage) ResolveByCode(_ context.Context, shortCode string) (model.Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, ok := s.byCode[shortCode]
	if !ok {
		return model.Link{}, storage.ErrNotFound
	}
	now := s.now().UTC()
	link.LastAccessedAt = &now
	link.AccessCount++
	s.byCode[shortCode] = link
	return clone(link), nil
}

func (s *Storage) GetStatsByCode(_ context.Context, shortCode string) (model.Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	link, ok := s.byCode[shortCode]
	if !ok {
		return model.Link{}, storage.ErrNotFound
	}
	return clone(link), nil
}

func clone(link model.Link) model.Link {
	if link.LastAccessedAt != nil {
		lastAccessed := *link.LastAccessedAt
		link.LastAccessedAt = &lastAccessed
	}
	return link
}
