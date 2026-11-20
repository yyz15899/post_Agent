package httphandler

import (
	"context"
	"log"
	"math"
	"micro_user/gateway/service"
	"micro_user/pb"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MonitoringHandler struct {
	Cli            pb.UserServiceClient
	pending        prometheus.Gauge
	pendingSuccess prometheus.Gauge
	metrics        http.Handler
	VoteStages     *VoteStageMetrics
}

func NewMonitorHandler(client pb.UserServiceClient) *MonitoringHandler {
	// Register once when constructing this Handler, not on every HTTP request.
	registry := prometheus.NewRegistry()
	pending := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "post_vote_sync_pending",
		Help: "Number of unfinished post vote sync tasks.",
	})
	pendingSuccess := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "post_vote_sync_pending_collect_success",
		Help: "Whether the latest pending query succeeded (1) or failed (0).",
	})
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		pending,
		pendingSuccess,
	)
	voteStages := NewVoteStageMetrics(registry)
	return &MonitoringHandler{
		Cli:            client,
		pending:        pending,
		pendingSuccess: pendingSuccess,
		metrics:        promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
		VoteStages:     voteStages,
	}
}

func (M *MonitoringHandler) CountPendingPostVoteSyncTasks(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	pending, err := service.CountPendingPostVoteSyncTasks(ctx)
	if err != nil {
		log.Printf("pending collection failed: %v", err)
		// Unknown is not zero. Keep process/runtime metrics available on DB failure.
		M.pending.Set(math.NaN())
		M.pendingSuccess.Set(0)
	} else {
		M.pending.Set(float64(pending))
		M.pendingSuccess.Set(1)
	}

	// The official handler writes the appropriate Prometheus response format.
	M.metrics.ServeHTTP(c.Writer, c.Request)
}
