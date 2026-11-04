package main

import (
	"context"
	"log"
	"micro_user/pb"
	"micro_user/usersvr/config"
	"micro_user/usersvr/dao"
	"micro_user/usersvr/handler"
	"micro_user/usersvr/middleware"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Fatalf("加载配置失败：%v", err)
	}
	l, err := net.Listen("tcp", ":8973")
	if err != nil {
		log.Fatal(err)
	}
	// 加载密钥
	if err := config.InitConfig(); err != nil {
		log.Fatalf("配置初始化失败,err:%v", err)
	}
	// db注册
	if err := dao.InitDB(); err != nil {
		log.Fatalf("数据库初始化失败,err:%v", err)
	}
	// TLS
	// creds, err := credentials.NewServerTLSFromFile("certs/server.crt", "certs/server.key")
	// if err != nil {
	// 	log.Printf("加载 TLS 证书失败: %v", err)
	// 	return
	// }
	// 注册服务
	s := grpc.NewServer(
		// grpc.Creds(creds),
		grpc.ChainUnaryInterceptor(middleware.Logginginterceptor,
			middleware.AuthInterceptor,
		),
	) // 创建服务,注册拦截器
	pb.RegisterUserServiceServer(s, &handler.Serve{}) // 注册
	// 优雅退出
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt) // 监听两个操作系统信号
	defer stop()

	go func() {
		if err := s.Serve(l); err != nil {
			log.Printf("server connect failed,err:%v\n", err)
		}
	}()

	<-ctx.Done() // 上下文取消
	done := make(chan struct{})
	go func() {
		s.GracefulStop() // 优雅停止
		close(done)
	}()
	select {
	case <-done:
		log.Println("优雅退出完成")
	case <-time.After(5 * time.Second):
		s.Stop()
		log.Println("等待超时,强制退出")

	}

}
