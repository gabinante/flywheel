package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "flywheel_http_requests_total",
		Help: "Total HTTP requests by method, path pattern, and status code.",
	}, []string{"method", "path", "status"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "flywheel_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	DispatchActiveWorkers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "flywheel_dispatch_active_workers",
		Help: "Number of currently active dispatch workers.",
	})

	DispatchMaxWorkers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "flywheel_dispatch_max_workers",
		Help: "Maximum number of dispatch workers allowed.",
	})

	QueueLeaseActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "flywheel_queue_leases_active",
		Help: "Number of active queue leases.",
	})

	QueueLeaseExpired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "flywheel_queue_leases_expired_total",
		Help: "Total number of expired queue leases.",
	})

	EventBusPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "flywheel_events_published_total",
		Help: "Total events published by topic.",
	}, []string{"topic"})

	TicketTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "flywheel_ticket_transitions_total",
		Help: "Total ticket state transitions.",
	}, []string{"from", "to"})

	CostTotalSpend = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "flywheel_cost_spend_total",
		Help: "Total cost spend in dollars by provider.",
	}, []string{"provider", "model"})
)
