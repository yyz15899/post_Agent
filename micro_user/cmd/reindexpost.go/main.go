package main

import (
	"context"
	"flag"
	"log"
	"micro_user/gateway/dao"
	"micro_user/gateway/rag"
	"micro_user/gateway/service"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Printf("加载env失败")
		return
	}
	err = dao.InitDB()
	if err != nil {
		log.Printf("初始化db失败")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// milvus客户端
	client, err := rag.NewMilvusClient(ctx)
	if err != nil {
		log.Printf("创建client失败")
		return
	}
	defer client.Close(ctx)

	// 向量模型
	embedder, err := rag.NewEmbedder(ctx)
	if err != nil {
		log.Printf("创建向量模型失败")
		return
	}
	// 索引器
	indexer, err := rag.NewIndexer(ctx, "post_docs", embedder)
	if err != nil {
		log.Printf("创建索引器失败")
		return
	}
	//
	postID := flag.Int64("id", 0, "需要补写的postid")
	if *postID <= 0 {
		log.Printf("postid 格式错误")
		return
	}
	err = service.ReindexPost(ctx, indexer, *postID)
	if err != nil {
		log.Printf("补写失败,err:%v", err)
		return
	}
}
