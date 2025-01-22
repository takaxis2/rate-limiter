package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/takaxis2/rate-limiter/internals/storage"
)

type Metrics struct {
	qm       *storage.QueueManager
	queueKey string
	// Prometheus metrics
	queueLength   *prometheus.GaugeVec
	waitTime      *prometheus.HistogramVec
	processTime   *prometheus.HistogramVec
	requestStatus *prometheus.CounterVec
}

func NewMetrics(qm *storage.QueueManager, queueKey string) *Metrics {
	m := &Metrics{
		qm:       qm,
		queueKey: queueKey,

		queueLength: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "rate_limiter_queue_length",
				Help: "Current length of the rate limiter queue",
			},
			[]string{"domain"},
		),

		waitTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "rate_limiter_wait_time_seconds",
				Help:    "Time spent waiting in queue",
				Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
			},
			[]string{"domain"},
		),

		processTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "rate_limiter_process_time_seconds",
				Help:    "Time spent processing request",
				Buckets: prometheus.ExponentialBuckets(0.01, 2, 10),
			},
			[]string{"domain"},
		),

		requestStatus: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "rate_limiter_request_total",
				Help: "Total number of requests by status",
			},
			[]string{"domain", "status"},
		),
	}

	// Prometheus에 메트릭 등록
	prometheus.MustRegister(
		m.queueLength,
		m.waitTime,
		m.processTime,
		m.requestStatus,
	)

	return m
}

func (m *Metrics) StartMetricsCollection(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 모든 도메인의 큐 길이 업데이트
			length, err := m.qm.GetTotalClients(ctx)
			if err != nil {
				if err == redis.Nil {
					length = -1
				} else {
					continue
				}
			}

			// 큐 길이 메트릭 업데이트
			m.queueLength.WithLabelValues(m.queueKey).Set(float64(length))
		}
	}
}
