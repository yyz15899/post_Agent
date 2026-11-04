package rag

import (
	"context"

	arkembedder "github.com/cloudwego/eino-ext/components/embedding/ark"
	"github.com/cloudwego/eino-ext/components/retriever/milvus2"
	milvusRetriever "github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/cloudwego/eino-ext/components/retriever/milvus2/search_mode"
)

func NewRetriever(ctx context.Context, collecion string, embedder *arkembedder.Embedder) (*milvusRetriever.Retriever, error) {
	retriever, err := milvusRetriever.NewRetriever(ctx, &milvusRetriever.RetrieverConfig{
		Client:      client,
		Collection:  collecion,
		Embedding:   embedder,
		VectorField: "vector",
		OutputFields: []string{
			"id",
			"content",
			"metadata",
		},
		TopK:       2,
		SearchMode: search_mode.NewApproximate(milvus2.COSINE),
	})
	if err != nil {
		return nil, err
	}
	return retriever, nil

}
