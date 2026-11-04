package rag

import (
	"context"

	arkembedder "github.com/cloudwego/eino-ext/components/embedding/ark"
	milvusIndexer "github.com/cloudwego/eino-ext/components/indexer/milvus2"
)

func NewIndexer(ctx context.Context, collection string, embedder *arkembedder.Embedder) (*milvusIndexer.Indexer, error) {
	indexer, err := milvusIndexer.NewIndexer(ctx, &milvusIndexer.IndexerConfig{
		Client:     client,
		Collection: collection,
		Embedding:  embedder,
		Vector: &milvusIndexer.VectorConfig{
			VectorField: "vector",
			Dimension:   2048,
			MetricType:  milvusIndexer.COSINE,
		},
	})
	if err != nil {
		return nil, err
	}
	return indexer, nil
}
