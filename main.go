
package main

import (
	"log"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}

	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}

	return r.ResponseWriter.Write(data)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(log.Writer(), nil))

	registry := prometheus.NewRegistry()

	requests := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "route", "status"},
	)

	duration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"route"},
	)

	registry.MustRegister(requests, duration)

	mux := http.NewServeMux()

	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte(`{"orders": 3}`))
	})

	mux.HandleFunc("/debug/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte("slow response"))
	})

	mux.HandleFunc("/debug/error", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "simulated error", http.StatusInternalServerError)
	})

	mux.Handle("/metrics", promhttp.HandlerFor(
		registry,
		promhttp.HandlerOpts{},
	))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}

		mux.ServeHTTP(recorder, r)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}

		elapsed := time.Since(start).Seconds()
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}

		requests.WithLabelValues(
			r.Method,
			route,
			strconv.Itoa(status),
		).Inc()

		duration.WithLabelValues(route).Observe(elapsed)

		logger.Info("http request",
			"method", r.Method,
			"route", route,
			"status", status,
			"duration_ms", elapsed*1000,
		)
	})

	logger.Info("server starting", "addr", ":8080")

	if err := http.ListenAndServe(":8080", handler); err != nil {
		logger.Error("server stopped", "error", err)
		log.Fatal(err)
	}
}
