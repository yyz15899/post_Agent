package httphandler

// 耗时计算
import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// VoteStageMetrics is shared by PostHandler and the /metrics registry.
type VoteStageMetrics struct {
	duration *prometheus.HistogramVec
}

// Construct and register once during startup, never once per request.
func NewVoteStageMetrics(registry prometheus.Registerer) *VoteStageMetrics {
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "post_vote_stage_duration_seconds",
		Help: "Duration of vote handler stages, including completed calls that return errors.",
		Buckets: []float64{
			0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.0075,
			0.01, 0.015, 0.02, 0.03, 0.05, 0.075, 0.1, 0.15,
			0.2, 0.3, 0.5, 0.75, 1, 1.5, 2, 3, 5,
		},
	}, []string{"stage"})
	registry.MustRegister(duration)
	// Expose zero-count series before the first request for reliable rate queries.
	for _, stage := range []string{"auth_rpc", "post_lookup", "vote_tx", "handler_total"} {
		duration.WithLabelValues(stage)
	}
	return &VoteStageMetrics{duration: duration}
}

// stage must be one of the fixed names above; never pass user or post IDs.
func (m *VoteStageMetrics) Observe(stage string, started time.Time) {
	m.duration.WithLabelValues(stage).Observe(time.Since(started).Seconds())
}
