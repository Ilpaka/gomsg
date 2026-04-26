package metrics

import (
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// M holds Prometheus collectors for the messenger backend. Pass nil checks at call sites.
type M struct {
	reg *prometheus.Registry

	HTTPRequests   *prometheus.CounterVec
	HTTPDuration   *prometheus.HistogramVec
	HTTPInflight   prometheus.Gauge
	WSActive       prometheus.Gauge
	WSInbound      *prometheus.CounterVec
	WSOutbound     *prometheus.CounterVec
	WSErrors       *prometheus.CounterVec
	AuthRegister   prometheus.Counter
	AuthLogin      prometheus.Counter
	AuthRefresh    prometheus.Counter
	AuthFailures   *prometheus.CounterVec
	MessagesCreate prometheus.Counter
	MessagesUpdate prometheus.Counter
	MessagesDelete prometheus.Counter
	MessagesReadRc prometheus.Counter
	OutboxCreated  prometheus.Counter
	RelayIter      prometheus.Counter
	RelayPubOK     prometheus.Counter
	RelayPubFail   prometheus.Counter
	KafkaHandled   prometheus.Counter
	KafkaFail      prometheus.Counter
}

// New builds a dedicated registry and registers standard + application collectors.
func New() *M {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	m := &M{reg: reg}

	m.HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests handled.",
	}, []string{"method", "route", "status_class"})
	m.HTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route", "status_class"})
	m.HTTPInflight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http_requests_inflight",
		Help: "Number of HTTP requests currently being handled.",
	})
	m.WSActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ws_active_connections",
		Help: "Number of active WebSocket connections.",
	})
	m.WSInbound = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ws_events_inbound_total",
		Help: "Inbound WebSocket JSON events by envelope event name (low-cardinality).",
	}, []string{"event"})
	m.WSOutbound = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ws_events_outbound_total",
		Help: "Outbound WebSocket frames by envelope event name (low-cardinality).",
	}, []string{"event"})
	m.WSErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ws_handler_errors_total",
		Help: "WS handler errors by coarse reason.",
	}, []string{"reason"})
	m.AuthRegister = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "auth_register_attempts_total",
		Help: "Auth register attempts (each HTTP request).",
	})
	m.AuthLogin = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "auth_login_attempts_total",
		Help: "Auth login attempts.",
	})
	m.AuthRefresh = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "auth_refresh_attempts_total",
		Help: "Auth refresh attempts.",
	})
	m.AuthFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "auth_failures_total",
		Help: "Auth failures by coarse reason (low-cardinality).",
	}, []string{"reason"})
	m.MessagesCreate = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "messages_created_total",
		Help: "Messages successfully created.",
	})
	m.MessagesUpdate = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "messages_updated_total",
		Help: "Messages successfully updated.",
	})
	m.MessagesDelete = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "messages_deleted_total",
		Help: "Messages successfully soft-deleted.",
	})
	m.MessagesReadRc = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "messages_read_receipts_total",
		Help: "Read receipt updates successfully applied.",
	})
	m.OutboxCreated = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "outbox_events_created_total",
		Help: "Outbox rows inserted with domain writes.",
	})
	m.RelayIter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "outbox_relay_iterations_total",
		Help: "Outbox relay polling iterations.",
	})
	m.RelayPubOK = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "outbox_relay_publish_success_total",
		Help: "Outbox events successfully published (Kafka or local fanout).",
	})
	m.RelayPubFail = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "outbox_relay_publish_failures_total",
		Help: "Outbox publish failures (will retry on next tick).",
	})
	m.KafkaHandled = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "kafka_consumer_events_handled_total",
		Help: "Kafka consumer messages processed for WS fanout.",
	})
	m.KafkaFail = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "kafka_consumer_handler_failures_total",
		Help: "Kafka consumer handler failures (decode/fanout).",
	})

	reg.MustRegister(
		m.HTTPRequests, m.HTTPDuration, m.HTTPInflight,
		m.WSActive, m.WSInbound, m.WSOutbound, m.WSErrors,
		m.AuthRegister, m.AuthLogin, m.AuthRefresh, m.AuthFailures,
		m.MessagesCreate, m.MessagesUpdate, m.MessagesDelete, m.MessagesReadRc,
		m.OutboxCreated, m.RelayIter, m.RelayPubOK, m.RelayPubFail,
		m.KafkaHandled, m.KafkaFail,
	)
	return m
}

// Handler exposes /metrics scrape endpoint (use dedicated route, no auth).
func (m *M) Handler() http.Handler {
	if m == nil || m.reg == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})
	}
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}

// Registry exposes the Prometheus registry (for tests and advanced integrations).
func (m *M) Registry() *prometheus.Registry {
	if m == nil {
		return nil
	}
	return m.reg
}

func statusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	case code >= 200:
		return "2xx"
	default:
		return "other"
	}
}

func routePattern(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	p := strings.TrimSpace(r.Pattern)
	if p != "" {
		return p
	}
	// Fallback when not using Go 1.22 pattern mux (should not happen here).
	return "unmatched"
}

var stripPrefixes = []string{"/metrics", "/health"}

func stripHighCardinalityPath(path string) string {
	// Reserved: if any custom non-pattern routes appear, map to fixed tokens.
	path = strings.TrimSpace(path)
	for _, p := range stripPrefixes {
		if path == p {
			return p
		}
	}
	return path
}

// HTTPMiddleware records request counts, duration, and inflight. Use outside Recovery
// so panics do not leak inflight gauge; pair with defer-safe design.
func (m *M) HTTPMiddleware(next http.Handler) http.Handler {
	if m == nil || m.reg == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.HTTPInflight.Inc()
		defer m.HTTPInflight.Dec()

		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()

		next.ServeHTTP(rw, r)

		route := routePattern(r)
		if route == "unmatched" {
			route = stripHighCardinalityPath(r.URL.Path)
		}
		sc := statusClass(rw.status)
		lm := r.Method
		m.HTTPRequests.WithLabelValues(lm, route, sc).Inc()
		m.HTTPDuration.WithLabelValues(lm, route, sc).Observe(time.Since(start).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
