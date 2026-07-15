package metrics

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	created  prometheus.Counter
	resolved prometheus.Counter
	errors   *prometheus.CounterVec
	handler  http.Handler
}

func New(backend string) *Metrics {
	registry := prometheus.NewRegistry()
	created := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "shortener",
		Name:      "links_created_total",
		Help:      "Number of newly created links.",
	})
	resolved := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "shortener",
		Name:      "resolve_requests_total",
		Help:      "Number of successful JSON resolves and redirects.",
	})
	httpErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "shortener",
		Name:      "http_errors_total",
		Help:      "Number of HTTP errors by status.",
	}, []string{"status"})
	storageBackend := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "shortener",
		Name:      "storage_backend_info",
		Help:      "Selected storage backend.",
	}, []string{"backend"})

	registry.MustRegister(created, resolved, httpErrors, storageBackend)
	storageBackend.WithLabelValues(backend).Set(1)

	return &Metrics{
		created:  created,
		resolved: resolved,
		errors:   httpErrors,
		handler:  promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
	}
}

func (m *Metrics) IncCreated()  { m.created.Inc() }
func (m *Metrics) IncResolved() { m.resolved.Inc() }

func (m *Metrics) IncError(status int) {
	m.errors.WithLabelValues(strconv.Itoa(status)).Inc()
}

func (m *Metrics) Handler() http.Handler {
	return m.handler
}
