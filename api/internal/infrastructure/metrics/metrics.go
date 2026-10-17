// Package metrics exports Prometheus metrics for the API and the worker.
package metrics

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "hooklane"

// NewRegistry returns a registry with Go runtime and process collectors.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return reg
}

// HTTP records API request counts and latencies by route pattern, so label
// cardinality stays bounded no matter which IDs clients request.
type HTTP struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func NewHTTP(reg prometheus.Registerer) *HTTP {
	m := &HTTP{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "http_requests_total",
			Help: "API requests by method, route pattern and status code.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Name: "http_request_duration_seconds",
			Help:    "API request latency by method and route pattern.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
	}
	reg.MustRegister(m.requests, m.duration)
	return m
}

func (m *HTTP) ObserveRequest(method, route string, status int, duration time.Duration) {
	m.requests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	m.duration.WithLabelValues(method, route).Observe(duration.Seconds())
}

// Worker implements application.WorkerObserver.
type Worker struct {
	claimed      prometheus.Counter
	claimErrors  prometheus.Counter
	inFlight     prometheus.Gauge
	attempts     *prometheus.CounterVec
	duration     *prometheus.HistogramVec
	leaseLost    prometheus.Counter
	recordErrors prometheus.Counter
	pruned       prometheus.Counter
}

func NewWorker(reg prometheus.Registerer) *Worker {
	m := &Worker{
		claimed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "worker_claimed_deliveries_total",
			Help: "Deliveries claimed by this worker.",
		}),
		claimErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "worker_claim_errors_total",
			Help: "Failed claim queries.",
		}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Name: "worker_deliveries_in_flight",
			Help: "Deliveries currently being sent by this worker.",
		}),
		attempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "delivery_attempts_total",
			Help: "Recorded delivery attempts by resulting delivery status and response class.",
		}, []string{"result", "response"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Name: "delivery_attempt_duration_seconds",
			Help:    "Outbound webhook request time by resulting delivery status.",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 15, 30},
		}, []string{"result"}),
		leaseLost: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "worker_lease_lost_total",
			Help: "Attempts discarded because the lease had expired.",
		}),
		recordErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "worker_record_errors_total",
			Help: "Attempts that could not be recorded.",
		}),
		pruned: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "retention_deleted_events_total",
			Help: "Events deleted by retention, with their deliveries and attempts.",
		}),
	}
	reg.MustRegister(m.claimed, m.claimErrors, m.inFlight, m.attempts, m.duration, m.leaseLost, m.recordErrors, m.pruned)
	return m
}

func (m *Worker) Claimed(n int)      { m.claimed.Add(float64(n)) }
func (m *Worker) ClaimFailed()       { m.claimErrors.Inc() }
func (m *Worker) InFlight(delta int) { m.inFlight.Add(float64(delta)) }
func (m *Worker) LeaseLost()         { m.leaseLost.Inc() }
func (m *Worker) RecordFailed()      { m.recordErrors.Inc() }

// EventsPruned counts events removed by retention.
func (m *Worker) EventsPruned(n int64) { m.pruned.Add(float64(n)) }

// AttemptRecorded labels a pending result as "retry_scheduled": after an
// attempt, a pending delivery is always waiting for its next try.
func (m *Worker) AttemptRecorded(status domain.DeliveryStatus, statusCode int, duration time.Duration) {
	result := string(status)
	if status == domain.DeliveryPending {
		result = "retry_scheduled"
	}
	m.attempts.WithLabelValues(result, responseClass(statusCode)).Inc()
	m.duration.WithLabelValues(result).Observe(duration.Seconds())
}

func responseClass(code int) string {
	if code < 100 || code > 599 {
		return "transport_error"
	}
	return strconv.Itoa(code/100) + "xx"
}

var _ application.WorkerObserver = (*Worker)(nil)

// BacklogFunc reads instance-wide queue depth.
type BacklogFunc func(ctx context.Context) (application.QueueBacklog, error)

// backlogCollector queries the database on every scrape so the gauges are
// correct across any number of workers.
type backlogCollector struct {
	read       BacklogFunc
	due        *prometheus.Desc
	scheduled  *prometheus.Desc
	held       *prometheus.Desc
	processing *prometheus.Desc
	oldest     *prometheus.Desc
	up         *prometheus.Desc
}

// RegisterBacklog exports queue depth gauges computed from the database.
func RegisterBacklog(reg prometheus.Registerer, read BacklogFunc) {
	desc := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, "queue", name), help, nil, nil)
	}
	reg.MustRegister(&backlogCollector{
		read:       read,
		due:        desc("due_deliveries", "Pending deliveries whose next attempt is due now."),
		scheduled:  desc("scheduled_deliveries", "Pending deliveries waiting for a future retry."),
		held:       desc("held_deliveries", "Pending deliveries of disabled endpoints; not claimed until re-enabled."),
		processing: desc("processing_deliveries", "Deliveries claimed by a worker."),
		oldest:     desc("oldest_due_seconds", "Age of the oldest due pending delivery; grows when workers fall behind."),
		up:         desc("backlog_query_up", "1 if the last backlog query succeeded."),
	})
}

func (c *backlogCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.due
	ch <- c.scheduled
	ch <- c.held
	ch <- c.processing
	ch <- c.oldest
	ch <- c.up
}

func (c *backlogCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	backlog, err := c.read(ctx)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 1)
	ch <- prometheus.MustNewConstMetric(c.due, prometheus.GaugeValue, float64(backlog.Due))
	ch <- prometheus.MustNewConstMetric(c.scheduled, prometheus.GaugeValue, float64(backlog.Scheduled))
	ch <- prometheus.MustNewConstMetric(c.held, prometheus.GaugeValue, float64(backlog.Held))
	ch <- prometheus.MustNewConstMetric(c.processing, prometheus.GaugeValue, float64(backlog.Processing))
	ch <- prometheus.MustNewConstMetric(c.oldest, prometheus.GaugeValue, backlog.OldestDueSeconds)
}

// Handler serves the registry in the Prometheus text format.
func Handler(reg *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}
