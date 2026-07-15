package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zxchelik/ozon_bank_test_task/internal/metrics"
	"github.com/zxchelik/ozon_bank_test_task/internal/model"
	"github.com/zxchelik/ozon_bank_test_task/internal/service"
	"github.com/zxchelik/ozon_bank_test_task/internal/storage"
)

func TestCreateAndRequestID(t *testing.T) {
	fake := &fakeService{create: func(context.Context, string) (model.Link, bool, error) {
		return model.Link{ShortCode: "abcdefghij"}, true, nil
	}}
	router := testRouter(fake, readinessFunc(func(context.Context) error { return nil }))
	request := httptest.NewRequest(http.MethodPost, "/links", strings.NewReader(`{"url":"https://example.com"}`))
	request.Header.Set("X-Request-ID", "known-request")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", response.Code)
	}
	if response.Header().Get("X-Request-ID") != "known-request" {
		t.Fatalf("request id = %q", response.Header().Get("X-Request-ID"))
	}
	if !strings.Contains(response.Body.String(), `"short_url":"http://short.test/r/abcdefghij"`) {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func TestResolveStatsAndRedirect(t *testing.T) {
	now := time.Date(2026, 7, 14, 7, 0, 0, 0, time.UTC)
	resolveCalls := 0
	statsCalls := 0
	fake := &fakeService{
		resolve: func(_ context.Context, code string) (model.Link, error) {
			resolveCalls++
			return model.Link{OriginalURL: "https://example.com", ShortCode: code}, nil
		},
		stats: func(_ context.Context, code string) (model.Link, error) {
			statsCalls++
			return model.Link{ShortCode: code, CreatedAt: now, AccessCount: 2}, nil
		},
	}
	router := testRouter(fake, readinessFunc(func(context.Context) error { return nil }))

	resolve := httptest.NewRecorder()
	router.ServeHTTP(resolve, httptest.NewRequest(http.MethodGet, "/links/abcdefghij", nil))
	if resolve.Code != 200 || !strings.Contains(resolve.Body.String(), `"url":"https://example.com"`) {
		t.Fatalf("resolve = %d %s", resolve.Code, resolve.Body.String())
	}

	stats := httptest.NewRecorder()
	router.ServeHTTP(stats, httptest.NewRequest(http.MethodGet, "/links/abcdefghij/stats", nil))
	if stats.Code != 200 || !strings.Contains(stats.Body.String(), `"access_count":2`) || !strings.Contains(stats.Body.String(), `"last_accessed_at":null`) {
		t.Fatalf("stats = %d %s", stats.Code, stats.Body.String())
	}

	redirect := httptest.NewRecorder()
	router.ServeHTTP(redirect, httptest.NewRequest(http.MethodGet, "/r/abcdefghij", nil))
	if redirect.Code != http.StatusFound || redirect.Header().Get("Location") != "https://example.com" {
		t.Fatalf("redirect = %d location=%q", redirect.Code, redirect.Header().Get("Location"))
	}
	if resolveCalls != 2 || statsCalls != 1 {
		t.Fatalf("resolve/stats calls = %d/%d", resolveCalls, statsCalls)
	}
}

func TestProbesMetricsAndErrors(t *testing.T) {
	fake := &fakeService{resolve: func(context.Context, string) (model.Link, error) { return model.Link{}, storage.ErrNotFound }}
	router := testRouter(fake, readinessFunc(func(context.Context) error { return errors.New("database unavailable") }))

	tests := []struct {
		path    string
		status  int
		content string
	}{
		{"/healthz", 200, `"status":"ok"`},
		{"/readyz", 503, `"error":"service is not ready"`},
		{"/links/abcdefghij", 404, `"error":"link not found"`},
		{"/metrics", 200, `shortener_storage_backend_info{backend="memory"} 1`},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.content) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			if response.Header().Get("X-Request-ID") == "" {
				t.Fatal("missing X-Request-ID")
			}
		})
	}
}

func TestBadRequests(t *testing.T) {
	fake := &fakeService{create: func(context.Context, string) (model.Link, bool, error) {
		return model.Link{}, false, service.ErrInvalidURL
	}}
	router := testRouter(fake, readinessFunc(func(context.Context) error { return nil }))
	for _, body := range []string{``, `{"url":`, `{"url":"https://example.com"} {}`, `{"unexpected":true}`} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/links", bytes.NewBufferString(body)))
		if response.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d", body, response.Code)
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/links", strings.NewReader(`{"url":"bad"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("service validation status = %d", response.Code)
	}
}

func testRouter(svc LinkService, ready Readiness) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(svc, ready, metrics.New("memory"), logger, "http://short.test", "memory", time.Second)
}

type readinessFunc func(context.Context) error

func (f readinessFunc) Check(ctx context.Context) error { return f(ctx) }

type fakeService struct {
	create  func(context.Context, string) (model.Link, bool, error)
	resolve func(context.Context, string) (model.Link, error)
	stats   func(context.Context, string) (model.Link, error)
}

func (f *fakeService) Create(ctx context.Context, value string) (model.Link, bool, error) {
	if f.create == nil {
		return model.Link{}, false, errors.New("unexpected Create call")
	}
	return f.create(ctx, value)
}
func (f *fakeService) Resolve(ctx context.Context, code string) (model.Link, error) {
	if f.resolve == nil {
		return model.Link{}, errors.New("unexpected Resolve call")
	}
	return f.resolve(ctx, code)
}
func (f *fakeService) Stats(ctx context.Context, code string) (model.Link, error) {
	if f.stats == nil {
		return model.Link{}, errors.New("unexpected Stats call")
	}
	return f.stats(ctx, code)
}
