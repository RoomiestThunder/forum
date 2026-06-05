package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	RequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status"})

	RequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "path", "status"})

	ActiveWebSockets = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "websocket_active_connections",
		Help: "Number of active WebSocket connections",
	})

	CacheHits = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "cache_hits_total",
		Help: "Total number of cache hits/misses",
	}, []string{"result"}) // "hit" or "miss"

	UploadSizeBytes = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "upload_size_bytes",
		Help:    "File upload sizes in bytes",
		Buckets: []float64{1024, 10240, 102400, 1048576, 5242880},
	})
)

func Init() {
	prometheus.MustRegister(
		RequestDuration,
		RequestsTotal,
		ActiveWebSockets,
		CacheHits,
		UploadSizeBytes,
	)
}

func Handler() http.Handler {
	return promhttp.Handler()
}

// responseRecorder captures the status code for metrics.
type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Middleware records request duration and count.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(rec.status)
		RequestDuration.WithLabelValues(r.Method, r.URL.Path, status).Observe(duration)
		RequestsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
	})
}
