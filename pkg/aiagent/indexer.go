package aiagent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino-ext/components/indexer/milvus"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// indexerRow is the row struct for Milvus InsertRows.
// Vector MUST be []float32 to match FieldTypeFloatVector in the collection schema.
// The default eino-ext DocumentConverter uses []byte (BinaryVector), which fails
// when the collection field is FloatVector.
type indexerRow struct {
	ID       string    `json:"id" milvus:"name:id"`
	Content  string    `json:"content" milvus:"name:content"`
	Vector   []float32 `json:"vector" milvus:"name:vector"`
	Metadata []byte    `json:"metadata" milvus:"name:metadata"`
}

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
		MetricType: milvus.COSINE,
		// Custom DocumentConverter that stores vectors as []float32 (FloatVector)
		// instead of the default []byte (BinaryVector).
		DocumentConverter: func(ctx context.Context, docs []*schema.Document, vectors [][]float64) ([]interface{}, error) {
			rows := make([]interface{}, 0, len(docs))
			for i, doc := range docs {
				metadata, err := json.Marshal(doc.MetaData)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal metadata: %w", err)
				}
				// Convert float64 embedding to float32 for FloatVector
				fv := make([]float32, len(vectors[i]))
				for j, v := range vectors[i] {
					fv[j] = float32(v)
				}
				rows = append(rows, &indexerRow{
					ID:       doc.ID,
					Content:  doc.Content,
					Vector:   fv,
					Metadata: metadata,
				})
			}
			return rows, nil
		},
		Embedding: eb,
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
