package storage

import (
	"context"
	"errors"

	"github.com/zxchelik/ozon_bank_test_task/internal/model"
)

var (
	ErrNotFound          = errors.New("link not found")
	ErrShortCodeConflict = errors.New("short code already exists")
)

type LinkStorage interface {
	CreateOrGet(ctx context.Context, originalURL, shortCode string) (model.Link, bool, error)
	ResolveByCode(ctx context.Context, shortCode string) (model.Link, error)
	GetStatsByCode(ctx context.Context, shortCode string) (model.Link, error)
}
