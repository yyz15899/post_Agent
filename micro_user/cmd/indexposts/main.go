package main

import (
	"context"
	"fmt"
	"log"
	"micro_user/gateway/dao"
	"micro_user/gateway/rag"
	"micro_user/gateway/service"
	"time"

	"github.com/joho/godotenv"
)

// 手动执行入库(milvus)
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
	//入库
	doc_id, err := service.IndexPost(ctx, indexer, 10)
	if err != nil {
		log.Printf("调用入库失败:%v", err)
		return
	}
	fmt.Printf("返回文档id:%v，数量:%d", doc_id, len(doc_id))

	// 检索数据
	retriever, err := rag.NewRetriever(ctx, "post_docs", embedder)
	if err != nil {
		log.Printf("初始化retruever失败:%v", err)
		return
	}
	docs, err := retriever.Retrieve(ctx, "aI时代")
	if err != nil {
		return
	}
	if len(docs) == 0 {
		log.Println("没有检索到post")
		return
	}
	for _, doc := range docs {
		doc_content := doc.Content
		doc_id := doc.ID
		post_id := doc.MetaData["post_id"]
		post_title := doc.MetaData["title"]
		fmt.Printf("content:%v doc_id:%v post_id:%v post_title:%v", doc_content, doc_id, post_id, post_title)
	}
}
