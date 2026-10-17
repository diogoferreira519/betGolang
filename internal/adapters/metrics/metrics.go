package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Registry struct {
	status     *prometheus.CounterVec
	duplicates prometheus.Counter
	retries    prometheus.Counter
	dlq        prometheus.Counter
	conflicts  prometheus.Counter
	outboxLag  prometheus.Gauge
	processing prometheus.Histogram
	reconDiv   prometheus.Counter
}

func New() *Registry {
	r := &Registry{
		status: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "wager_results_total", Help: "Wager results by status"}, []string{"status"}),
		duplicates: prometheus.NewCounter(prometheus.CounterOpts{Name: "wager_duplicates_total", Help: "Duplicates"}),
		retries: prometheus.NewCounter(prometheus.CounterOpts{Name: "sqs_retries_total", Help: "Retries"}),
		dlq: prometheus.NewCounter(prometheus.CounterOpts{Name: "sqs_dlq_total", Help: "DLQ"}),
		conflicts: prometheus.NewCounter(prometheus.CounterOpts{Name: "concurrency_conflicts_total", Help: "Conflicts"}),
		outboxLag: prometheus.NewGauge(prometheus.GaugeOpts{Name: "outbox_lag_seconds", Help: "Outbox lag"}),
		processing: prometheus.NewHistogram(prometheus.HistogramOpts{Name: "wager_processing_seconds", Help: "Latency", Buckets: prometheus.DefBuckets}),
		reconDiv: prometheus.NewCounter(prometheus.CounterOpts{Name: "reconciliation_divergences_total", Help: "Recon divergences"}),
	}
	prometheus.MustRegister(r.status, r.duplicates, r.retries, r.dlq, r.conflicts, r.outboxLag, r.processing, r.reconDiv)
	return r
}

func (r *Registry) IncStatus(s string)           { r.status.WithLabelValues(s).Inc() }
func (r *Registry) IncDuplicate()                { r.duplicates.Inc() }
func (r *Registry) IncRetry()                    { r.retries.Inc() }
func (r *Registry) IncDLQ()                      { r.dlq.Inc() }
func (r *Registry) IncConflict()                 { r.conflicts.Inc() }
func (r *Registry) SetOutboxLag(s float64)       { r.outboxLag.Set(s) }
func (r *Registry) ObserveProcessing(d time.Duration) { r.processing.Observe(d.Seconds()) }
func (r *Registry) IncReconciliationDivergence() { r.reconDiv.Inc() }
func (r *Registry) Handler() http.Handler        { return promhttp.Handler() }
