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
	registry := prometheus.NewRegistry() // 创建空注册表(后续从中收集数据)
	// 创建仪表(可升降或直接赋值的数值指标)
	pending := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "post_vote_sync_pending", // 指标名
		Help: "Number of unfinished post vote sync tasks.",
	})
	pendingSuccess := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "post_vote_sync_pending_collect_success",
		Help: "Whether the latest pending query succeeded (1) or failed (0).",
	})
	//
	registry.MustRegister( // 将创建的仪表(数值指标)放进注册表
		collectors.NewGoCollector(),
		// 创建 Go 运行时采集器，提供 goroutine 数量、内存分配等指标。
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		// 创建进程采集器，提供当前进程的 CPU 时间、内存等指标。这里传空配置，使用默认选项。
		pending,
		pendingSuccess,
	)
	voteStages := NewVoteStageMetrics(registry)
	return &MonitoringHandler{
		Cli:            client,
		pending:        pending,
		pendingSuccess: pendingSuccess,
		metrics:        promhttp.HandlerFor(registry, promhttp.HandlerOpts{}), // 从指定注册表收集指标并生成prometheus能读的响应
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
		M.pending.Set(math.NaN()) // Set赋值给响应(pending)指标  这里是NaN即nil
		M.pendingSuccess.Set(0)
	} else {
		M.pending.Set(float64(pending))
		M.pendingSuccess.Set(1)
	}

	// The official handler writes the appropriate Prometheus response format.
	M.metrics.ServeHTTP(c.Writer, c.Request)
}
