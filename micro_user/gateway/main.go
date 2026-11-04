package main

import (
	"context"
	"log"
	"micro_user/gateway/dao"
	"micro_user/gateway/httphandler"
	"micro_user/gateway/rag"
	"micro_user/gateway/service"
	"micro_user/pb"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Fatalf("加载配置失败：%v", err)
	}
	con, err := grpc.NewClient("127.0.0.1:8973", grpc.WithTransportCredentials(insecure.NewCredentials()))
	//
	if err := dao.InitDB(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	if err != nil {
		log.Fatal(err)
	}
	defer con.Close() // 在下面匿名goroutine执行结束后执行
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

	workerCtx, stopWorker := context.WithCancel(context.Background())
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)                       // Ⅹ RunPostSyncWorker退出执行,	开始执行close()
		service.RunPostSyncWorker(workerCtx, indexer) // indexer中使用的是milvusclient
	}()
	defer func() {
		stopWorker() // 通知workerctx关闭
		<-workerDone // Ⅹ处执行close()后,这里才能接收到,执行完毕后,开始执行上面的defer (P38)
	}()
	postHandler := httphandler.NewPostHandler(cli)
	agentHandler := httphandler.NewAgentHandler(cli, retriever)
	userHandler := httphandler.NewUserHandler(cli)

	r := gin.Default()

	r.POST("/api/signup", userHandler.Signup)

	r.POST("/api/signin", userHandler.Signin)
	r.GET("/api/getinfo", userHandler.GetInfo)

	r.POST("/api/posts", postHandler.CreatePost)
	r.GET("/api/posts/:id", postHandler.GetPostByID)

	r.GET("/api/posts", postHandler.SearchPosts)

	r.POST("/api/agent/chat", agentHandler.AgentChat)
	r.Run(":8081")
}
