package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zxchelik/ozon_bank_test_task/internal/model"
	"github.com/zxchelik/ozon_bank_test_task/internal/storage"
)

const (
	originalURLConstraint = "links_original_url_key"
	shortCodeConstraint   = "links_short_code_key"
)

type Storage struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Storage {
	return &Storage{pool: pool}
}

func (s *Storage) CreateOrGet(ctx context.Context, originalURL, shortCode string) (model.Link, bool, error) {
	link, err := s.getByOriginalURL(ctx, originalURL)
	if err == nil {
		return link, false, nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return model.Link{}, false, err
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO links (original_url, short_code)
		VALUES ($1, $2)
		RETURNING original_url, short_code, created_at, last_accessed_at, access_count`, originalURL, shortCode)
	link, err = scanLink(row)
	if err == nil {
		return link, true, nil
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return model.Link{}, false, fmt.Errorf("insert link: %w", err)
	}
	switch pgErr.ConstraintName {
	case originalURLConstraint:
		link, getErr := s.getByOriginalURL(ctx, originalURL)
		if getErr != nil {
			return model.Link{}, false, fmt.Errorf("get concurrently created link: %w", getErr)
		}
		return link, false, nil
	case shortCodeConstraint:
		return model.Link{}, false, storage.ErrShortCodeConflict
	default:
		return model.Link{}, false, fmt.Errorf("unexpected unique constraint %q: %w", pgErr.ConstraintName, err)
	}
}

func (s *Storage) ResolveByCode(ctx context.Context, shortCode string) (model.Link, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE links
		SET last_accessed_at = NOW(), access_count = access_count + 1
		WHERE short_code = $1
		RETURNING original_url, short_code, created_at, last_accessed_at, access_count`, shortCode)
	link, err := scanLink(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Link{}, storage.ErrNotFound
	}
	if err != nil {
		return model.Link{}, fmt.Errorf("resolve link: %w", err)
	}
	return link, nil
}

func (s *Storage) GetStatsByCode(ctx context.Context, shortCode string) (model.Link, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT original_url, short_code, created_at, last_accessed_at, access_count
		FROM links
		WHERE short_code = $1`, shortCode)
	link, err := scanLink(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Link{}, storage.ErrNotFound
	}
	if err != nil {
		return model.Link{}, fmt.Errorf("get link stats: %w", err)
	}
	return link, nil
}

func (s *Storage) getByOriginalURL(ctx context.Context, originalURL string) (model.Link, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT original_url, short_code, created_at, last_accessed_at, access_count
		FROM links
		WHERE original_url = $1`, originalURL)
	link, err := scanLink(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Link{}, storage.ErrNotFound
	}
	if err != nil {
		return model.Link{}, fmt.Errorf("get link by original URL: %w", err)
	}
	return link, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanLink(row rowScanner) (model.Link, error) {
	var link model.Link
	err := row.Scan(&link.OriginalURL, &link.ShortCode, &link.CreatedAt, &link.LastAccessedAt, &link.AccessCount)
	return link, err
}
