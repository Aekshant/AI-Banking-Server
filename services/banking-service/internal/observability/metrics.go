package observability

import (
	"net/http"
	"runtime"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds every metric the service exposes at /metrics.
var Registry = prometheus.NewRegistry()

var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests by method, route template and status code.",
	}, []string{"method", "route", "status"})

	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by method and route template.",
		Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
	}, []string{"method", "route"})

	httpInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "HTTP requests currently being served.",
	})

	// TransfersTotal counts transfer requests by outcome: settled, replayed,
	// insufficient_funds, account_not_found, idempotency_conflict, invalid, error.
	TransfersTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "transfers_total",
		Help: "Transfer requests by outcome.",
	}, []string{"outcome"})

	// TransferAmount sums the rupees moved by settled transfers.
	TransferAmount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transfer_amount_rupees_total",
		Help: "Total rupees moved by settled transfers.",
	})

	// LoanEvaluations counts loan decisions by reason (ELIGIBLE, LOW_CREDIT_SCORE, ...).
	LoanEvaluations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "loan_evaluations_total",
		Help: "Loan evaluations by decision reason.",
	}, []string{"reason"})

	// RateLimitDecisions counts limiter outcomes: allowed, rejected, or error
	// (Redis failed and the request was allowed anyway).
	RateLimitDecisions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ratelimit_decisions_total",
		Help: "Rate limiter decisions by rule and decision.",
	}, []string{"rule", "decision"})
)

// Version is set at build time with -ldflags "-X ...observability.Version=v1.2.3".
var Version = "dev"

func init() {
	buildInfo := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "banking_service_build_info",
		Help:        "Build information; always 1.",
		ConstLabels: prometheus.Labels{"version": Version, "go_version": runtime.Version()},
	})
	buildInfo.Set(1)

	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		buildInfo,
		httpRequests, httpDuration, httpInFlight,
		TransfersTotal, TransferAmount, LoanEvaluations, RateLimitDecisions,
	)
}

// MetricsHandler serves /metrics in OpenMetrics format, which carries exemplars.
func MetricsHandler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{EnableOpenMetrics: true, Registry: Registry})
}

// RegisterDBPool exposes connection pool statistics.
func RegisterDBPool(pool *pgxpool.Pool) {
	Registry.MustRegister(&poolCollector{pool: pool})
}

type poolCollector struct{ pool *pgxpool.Pool }

var (
	poolConns       = prometheus.NewDesc("db_pool_connections", "Connections in the pool by state.", []string{"state"}, nil)
	poolMax         = prometheus.NewDesc("db_pool_max_connections", "Maximum pool size.", nil, nil)
	poolAcquires    = prometheus.NewDesc("db_pool_acquires_total", "Successful connection acquisitions.", nil, nil)
	poolEmptyWaits  = prometheus.NewDesc("db_pool_empty_acquires_total", "Acquisitions that had to wait because the pool was empty.", nil, nil)
	poolAcquireTime = prometheus.NewDesc("db_pool_acquire_duration_seconds_total", "Total time spent acquiring connections.", nil, nil)
)

func (p *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{poolConns, poolMax, poolAcquires, poolEmptyWaits, poolAcquireTime} {
		ch <- d
	}
}

func (p *poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := p.pool.Stat()
	ch <- prometheus.MustNewConstMetric(poolConns, prometheus.GaugeValue, float64(s.AcquiredConns()), "acquired")
	ch <- prometheus.MustNewConstMetric(poolConns, prometheus.GaugeValue, float64(s.IdleConns()), "idle")
	ch <- prometheus.MustNewConstMetric(poolConns, prometheus.GaugeValue, float64(s.ConstructingConns()), "constructing")
	ch <- prometheus.MustNewConstMetric(poolMax, prometheus.GaugeValue, float64(s.MaxConns()))
	ch <- prometheus.MustNewConstMetric(poolAcquires, prometheus.CounterValue, float64(s.AcquireCount()))
	ch <- prometheus.MustNewConstMetric(poolEmptyWaits, prometheus.CounterValue, float64(s.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(poolAcquireTime, prometheus.CounterValue, s.AcquireDuration().Seconds())
}
