package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/zxchelik/ozon_bank_test_task/internal/model"
	"github.com/zxchelik/ozon_bank_test_task/internal/storage"
)

const DefaultCreateAttempts = 5

var (
	ErrInvalidURL          = errors.New("invalid URL")
	ErrInvalidShortCode    = errors.New("invalid short code")
	ErrGenerationExhausted = errors.New("short code generation attempts exhausted")
)

type Service struct {
	storage   storage.LinkStorage
	generator CodeGenerator
	attempts  int
}

func New(linkStorage storage.LinkStorage, generator CodeGenerator, attempts int) *Service {
	if attempts <= 0 {
		attempts = DefaultCreateAttempts
	}
	return &Service{storage: linkStorage, generator: generator, attempts: attempts}
}

func (s *Service) Create(ctx context.Context, originalURL string) (model.Link, bool, error) {
	if !validURL(originalURL) {
		return model.Link{}, false, ErrInvalidURL
	}
	for range s.attempts {
		code, err := s.generator.Generate()
		if err != nil {
			return model.Link{}, false, fmt.Errorf("generate short code: %w", err)
		}
		link, created, err := s.storage.CreateOrGet(ctx, originalURL, code)
		if err == nil {
			return link, created, nil
		}
		if !errors.Is(err, storage.ErrShortCodeConflict) {
			return model.Link{}, false, fmt.Errorf("create or get link: %w", err)
		}
	}
	return model.Link{}, false, ErrGenerationExhausted
}

func (s *Service) Resolve(ctx context.Context, shortCode string) (model.Link, error) {
	if !ValidShortCode(shortCode) {
		return model.Link{}, ErrInvalidShortCode
	}
	link, err := s.storage.ResolveByCode(ctx, shortCode)
	if err != nil {
		return model.Link{}, fmt.Errorf("resolve link: %w", err)
	}
	return link, nil
}

func (s *Service) Stats(ctx context.Context, shortCode string) (model.Link, error) {
	if !ValidShortCode(shortCode) {
		return model.Link{}, ErrInvalidShortCode
	}
	link, err := s.storage.GetStatsByCode(ctx, shortCode)
	if err != nil {
		return model.Link{}, fmt.Errorf("get link stats: %w", err)
	}
	return link, nil
}

func ValidShortCode(code string) bool {
	if len(code) != CodeLength {
		return false
	}
	for _, char := range code {
		if !strings.ContainsRune(Alphabet, char) {
			return false
		}
	}
	return true
}

func validURL(raw string) bool {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return false
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}
