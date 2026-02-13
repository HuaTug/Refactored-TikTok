package aiagent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/indexer/milvus"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// NewMilvusIndexer creates a Milvus-based document indexer for knowledge ingestion.
func NewMilvusIndexer(ctx context.Context) (*milvus.Indexer, error) {
	client, err := NewMilvusClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Milvus client for indexer: %w", err)
	}
	eb, err := NewEmbedder(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder for indexer: %w", err)
	}
	indexer, err := milvus.NewIndexer(ctx, &milvus.IndexerConfig{
		Client:     client,
		Collection: MilvusCollectionName,
		Fields:     indexerFields,
		Embedding:  eb,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Milvus indexer: %w", err)
	}
	return indexer, nil
}

// indexerFields defines the schema fields for the Milvus indexer.
var indexerFields = []*entity.Field{
	{
		Name:     "id",
		DataType: entity.FieldTypeVarChar,
		TypeParams: map[string]string{
			"max_length": "255",
		},
		PrimaryKey: true,
	},
	{
		Name:     "vector",
		DataType: entity.FieldTypeFloatVector,
		TypeParams: map[string]string{
			"dim": "768",
		},
	},
	{
		Name:     "content",
		DataType: entity.FieldTypeVarChar,
		TypeParams: map[string]string{
			"max_length": "8192",
		},
	},
	{
		Name:     "metadata",
		DataType: entity.FieldTypeJSON,
	},
}
