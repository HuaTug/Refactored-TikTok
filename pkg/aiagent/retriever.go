package aiagent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/retriever/milvus"
	"github.com/cloudwego/eino/components/retriever"
)

// NewMilvusRetriever creates a Milvus-based vector retriever for RAG queries.
func NewMilvusRetriever(ctx context.Context) (retriever.Retriever, error) {
	client, err := NewMilvusClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Milvus client for retriever: %w", err)
	}
	eb, err := NewEmbedder(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder for retriever: %w", err)
	}
	r, err := milvus.NewRetriever(ctx, &milvus.RetrieverConfig{
		Client:      client,
		Collection:  MilvusCollectionName,
		VectorField: "vector",
		OutputFields: []string{
			"id",
			"content",
			"metadata",
		},
		TopK:      3,
		Embedding: eb,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Milvus retriever: %w", err)
	}
	return r, nil
}
