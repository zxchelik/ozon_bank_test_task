package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zxchelik/ozon_bank_test_task/internal/metrics"
	"github.com/zxchelik/ozon_bank_test_task/internal/model"
	"github.com/zxchelik/ozon_bank_test_task/internal/observability"
	"github.com/zxchelik/ozon_bank_test_task/internal/service"
	"github.com/zxchelik/ozon_bank_test_task/internal/storage"
)

type LinkService interface {
	Create(ctx context.Context, originalURL string) (model.Link, bool, error)
	Resolve(ctx context.Context, shortCode string) (model.Link, error)
	Stats(ctx context.Context, shortCode string) (model.Link, error)
}

type Readiness interface {
	Check(ctx context.Context) error
}

type Handler struct {
	service          LinkService
	readiness        Readiness
	metrics          *metrics.Metrics
	logger           *slog.Logger
	baseURL          string
	readinessTimeout time.Duration
}

func NewRouter(linkService LinkService, readiness Readiness, metricSet *metrics.Metrics, logger *slog.Logger, baseURL, storageType string, readinessTimeout time.Duration) http.Handler {
	h := &Handler{
		service: linkService, readiness: readiness, metrics: metricSet,
		logger: logger, baseURL: baseURL, readinessTimeout: readinessTimeout,
	}
	router := chi.NewRouter()
	router.Use(observability.RequestID)
	router.Use(observability.AccessLog(logger, storageType))
	router.Post("/links", h.create)
	router.Get("/links/{code}", h.resolve)
	router.Get("/links/{code}/stats", h.stats)
	router.Get("/r/{code}", h.redirect)
	router.Get("/healthz", h.health)
	router.Get("/readyz", h.ready)
	router.Get("/metrics", h.prometheusMetrics)
	return router
}

type createRequest struct {
	URL string `json:"url"`
}
type createResponse struct {
	ShortURL  string `json:"short_url"`
	ShortCode string `json:"short_code"`
}
type resolveResponse struct {
	URL string `json:"url"`
}
type statsResponse struct {
	ShortCode      string     `json:"short_code"`
	CreatedAt      time.Time  `json:"created_at"`
	LastAccessedAt *time.Time `json:"last_accessed_at"`
	AccessCount    int64      `json:"access_count"`
}
type errorResponse struct {
	Error string `json:"error"`
}
type statusResponse struct {
	Status string `json:"status"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var request createRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "invalid JSON body", err, "")
		return
	}
	if err := ensureEOF(decoder); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "invalid JSON body", err, "")
		return
	}
	link, created, err := h.service.Create(r.Context(), request.URL)
	if err != nil {
		h.handleServiceError(w, r, err, "")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
		h.metrics.IncCreated()
	}
	h.writeJSON(w, status, createResponse{ShortURL: h.baseURL + "/r/" + link.ShortCode, ShortCode: link.ShortCode})
}

func (h *Handler) resolve(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	link, err := h.service.Resolve(r.Context(), code)
	if err != nil {
		h.handleServiceError(w, r, err, code)
		return
	}
	h.metrics.IncResolved()
	h.writeJSON(w, http.StatusOK, resolveResponse{URL: link.OriginalURL})
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	link, err := h.service.Stats(r.Context(), code)
	if err != nil {
		h.handleServiceError(w, r, err, code)
		return
	}
	h.writeJSON(w, http.StatusOK, statsResponse{
		ShortCode: link.ShortCode, CreatedAt: link.CreatedAt,
		LastAccessedAt: link.LastAccessedAt, AccessCount: link.AccessCount,
	})
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	link, err := h.service.Resolve(r.Context(), code)
	if err != nil {
		h.handleServiceError(w, r, err, code)
		return
	}
	h.metrics.IncResolved()
	http.Redirect(w, r, link.OriginalURL, http.StatusFound)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	h.writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.readinessTimeout)
	defer cancel()
	if err := h.readiness.Check(ctx); err != nil {
		h.writeError(w, r, http.StatusServiceUnavailable, "service is not ready", err, "")
		return
	}
	h.writeJSON(w, http.StatusOK, statusResponse{Status: "ready"})
}

func (h *Handler) prometheusMetrics(w http.ResponseWriter, r *http.Request) {
	h.metrics.Handler().ServeHTTP(w, r)
}

func (h *Handler) handleServiceError(w http.ResponseWriter, r *http.Request, err error, code string) {
	switch {
	case errors.Is(err, service.ErrInvalidURL), errors.Is(err, service.ErrInvalidShortCode):
		h.writeError(w, r, http.StatusBadRequest, err.Error(), err, code)
	case errors.Is(err, storage.ErrNotFound):
		h.writeError(w, r, http.StatusNotFound, "link not found", err, code)
	default:
		h.writeError(w, r, http.StatusInternalServerError, "internal server error", err, code)
	}
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, status int, message string, err error, code string) {
	h.metrics.IncError(status)
	h.logger.ErrorContext(r.Context(), "request failed",
		"request_id", observability.RequestIDFromContext(r.Context()),
		"status", status, "short_code", code, "error", err,
	)
	h.writeJSON(w, status, errorResponse{Error: message})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
