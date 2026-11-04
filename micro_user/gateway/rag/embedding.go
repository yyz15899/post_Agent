package rag

import (
	"context"
	"errors"
	"os"

	arkembedder "github.com/cloudwego/eino-ext/components/embedding/ark"
)

// 创建向量模型  (将post文本转换为向量)
func NewEmbedder(ctx context.Context) (*arkembedder.Embedder, error) {
	apiKey := os.Getenv("ARK_API_KEY")
	if apiKey == "" {
		return nil, errors.New("未读取到 ARK_API_KEY，请检查配置加载和变量名")
	}
	APIType := arkembedder.APITypeMultiModal
	embedder, err := arkembedder.NewEmbedder(ctx, &arkembedder.EmbeddingConfig{
		APIKey:  apiKey,
		Model:   os.Getenv("EMBEDDER"),
		APIType: &APIType,
	})
	if err != nil {
		return nil, err
	}
	return embedder, nil
}
