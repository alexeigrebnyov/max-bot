package api

import (
	"net/http"
	"sync/atomic"
)

// Metrics — счётчики для мониторинга (запросы, ошибки, срабатывания rate limit).
type Metrics struct {
	requests   atomic.Int64
	errors     atomic.Int64
	rateLimits atomic.Int64
}

// NewMetrics создаёт новый счётчик метрик.
func NewMetrics() *Metrics {
	return &Metrics{}
}

// IncRequests увеличивает счётчик запросов на 1.
func (m *Metrics) IncRequests() { m.requests.Add(1) }

// IncErrors увеличивает счётчик ошибок на 1.
func (m *Metrics) IncErrors() { m.errors.Add(1) }

// IncRateLimit увеличивает счётчик срабатываний rate limit на 1.
func (m *Metrics) IncRateLimit() { m.rateLimits.Add(1) }

// Snapshot возвращает текущие значения для отдачи в /metrics.
func (m *Metrics) Snapshot() map[string]int64 {
	return map[string]int64{
		"requests":    m.requests.Load(),
		"errors":      m.errors.Load(),
		"rate_limits": m.rateLimits.Load(),
	}
}

// metricsMiddleware считает запросы и ошибки (ответ с кодом >= 400).
func (m *Metrics) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.IncRequests()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.status >= 400 {
			m.IncErrors()
		}
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
