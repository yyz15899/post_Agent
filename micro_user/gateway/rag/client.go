package rag

import (
	"context"
	"log"

	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

var client *milvusclient.Client

// 向量数据库client
func NewMilvusClient(ctx context.Context) (*milvusclient.Client, error) {
	cli, err := milvusclient.New(ctx, &milvusclient.ClientConfig{
		Address: "localhost:19530",
		DBName:  "default",
	})
	if err != nil {
		log.Printf("创建milvus client 失败:%v", err)
		return nil, err
	}
	client = cli
	return cli, nil
}
