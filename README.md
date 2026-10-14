# go-observability-demo

Учебный пример на Go: один файл `main.go`, одна внешняя зависимость (Prometheus client) и несколько endpoint'ов для экспериментов с наблюдаемостью.

## Что демонстрирует

- структурированные JSON-логи через `log/slog`;
- метрики запросов для Prometheus;
- контролируемое воспроизведение медленного ответа и ошибки.

## main.go

```go
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
```

## Запуск

Создай каталог и инициализируй модуль:

```bash
mkdir go-observability-demo
cd go-observability-demo

go mod init github.com/wolverineo250kr/go-observability-demo
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp

go run .
```

Проверь работу endpoint'ов:

```bash
curl http://localhost:8080/orders
curl http://localhost:8080/debug/slow
curl http://localhost:8080/debug/error
curl http://localhost:8080/metrics
```

В `/metrics` появятся, среди прочего:

- `http_requests_total` — количество запросов с разбивкой по методу, маршруту и статусу;
- `http_request_duration_seconds` — гистограмма длительности запросов.

Это стандартный подход к инструментированию Go-приложения для Prometheus.

## Замечания

Это учебный пример, а не готовый production middleware. В реальном сервисе стоит также учесть:

- поддержку дополнительных интерфейсов `ResponseWriter`;
- graceful shutdown;
- таймауты сервера;
- контекст запроса.

Для метрик важно не использовать произвольный URL или ID пользователя в качестве label: это может привести к высокой кардинальности.
