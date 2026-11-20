package main

import (
	"context"
	"log"
	"micro_user/gateway/dao"
	"micro_user/gateway/httphandler"
	"micro_user/gateway/middleware"
	"micro_user/gateway/rag"
	"micro_user/gateway/service"
	"micro_user/pb"
	"os"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Fatalf("加载配置失败：%v", err)
	}
	secret := os.Getenv("JWT_SECRET") // 获取本地密钥
	if secret == "" {
		log.Printf("jwt 为空")
		return
	}
	con, err := grpc.NewClient("127.0.0.1:8973", grpc.WithTransportCredentials(insecure.NewCredentials()))
	// 数据库初始化
	if err := dao.InitDB(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	if err != nil {
		log.Fatal(err)
	}
	// redis初始化
	err = dao.InitRdb()
	if err != nil {
		log.Fatal(err)
	}
	defer con.Close() // 在下面匿名goroutine执行结束后执行
	// 创建grpc client
	cli := pb.NewUserServiceClient(con)
	// 创建milvus client
	mil_cli, err := rag.NewMilvusClient(context.Background())
	if err != nil {
		log.Printf("网关初始化 NewMilvusClient失败:%v", err)
		return
	}
	defer mil_cli.Close(context.Background()) // 关闭client连接
	embedder, err := rag.NewEmbedder(context.Background())
	if err != nil {
		log.Printf("网关embedder初始化失败:%v", err)
		return
	}
	indexer, err := rag.NewIndexer(context.Background(), "post_docs", embedder)
	if err != nil {
		log.Printf("网关indexer初始化失败:%v", err)
		return
	}
	retriever, err := rag.NewRetriever(context.Background(), "post_docs", embedder)
	if err != nil {
		log.Printf("网关retriever初始化失败:%v", err)
		return
	} // 存入retriever

	var wg sync.WaitGroup
	wg.Add(2)
	workerCtx, stopWorker := context.WithCancel(context.Background())
	workerDone := make(chan struct{})
	go func() {
		// defer close(workerDone) // Ⅹ RunPostSyncWorker退出执行,	开始执行close()
		defer wg.Done()
		service.RunPostSyncWorker(workerCtx, indexer) // indexer中使用的是milvusclient
	}()
	go func() {
		defer wg.Done()
		service.RunPostVoteSyncWorker(workerCtx)
	}()
	go func() {
		wg.Wait()
		close(workerDone)
	}()
	defer func() {
		stopWorker() // 通知workerctx关闭
		<-workerDone // Ⅹ处执行close()后,这里才能接收到,执行完毕后,开始执行上面的defer
	}()

	MonitoringHandler := httphandler.NewMonitorHandler(cli)
	postHandler := httphandler.NewPostHandler(MonitoringHandler.VoteStages)
	agentHandler := httphandler.NewAgentHandler(retriever)
	userHandler := httphandler.NewUserHandler(cli)

	// 注册中间件
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	// r.Use(middleware.AuthInterceptor(cli)) // 全局注册

	// r := gin.Default()		 使用默认中间件
	// 接口路由
	r.POST("/api/signup", userHandler.Signup)
	r.POST("/api/signin", userHandler.Signin)
	r.GET("/api/posts/:id", postHandler.GetPostByID)
	r.GET("/api/posts", postHandler.SearchPosts)

	r.POST("api/posts/:id/vote", postHandler.VoteTiming(), middleware.VoteAuth([]byte(secret)), postHandler.VotePost)

	// 需要JWT的接口(路由组)
	authenticated := r.Group("/api")
	authenticated.Use(
		postHandler.VoteTiming(), // 执行完的next()执行鉴权
		middleware.AuthInterceptor(cli),
	)

	authenticated.GET("/getinfo", userHandler.GetInfo)
	authenticated.POST("/posts", postHandler.CreatePost)
	// authenticated.POST("/posts/:id/vote", postHandler.VoteTiming(), middleware.VoteAuth([]byte(secret)), postHandler.VotePost)
	authenticated.POST("/agent/chat", agentHandler.AgentChat)
	// 监控pending接口
	r.GET("/metrics", MonitoringHandler.CountPendingPostVoteSyncTasks)
	r.Run(":8081")
}
