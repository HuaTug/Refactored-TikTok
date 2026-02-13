package aiagent

import (
	"context"
	"fmt"

	"HuaTug.com/config"

	cli "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// milvusFields defines the Milvus collection schema for the knowledge base.
var milvusFields = []*entity.Field{
	{
		Name:     "id",
		DataType: entity.FieldTypeVarChar,
		TypeParams: map[string]string{
			"max_length": "256",
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

// NewMilvusClient creates and initializes a Milvus client with auto-provisioning
// of the database and collection if they don't exist.
func NewMilvusClient(ctx context.Context) (cli.Client, error) {
	cfg := config.ConfigInfo.AIAgent
	addr := cfg.Milvus.Address
	if addr == "" {
		addr = "localhost:19530"
	}

	// 1. Connect to the default database first
	defaultClient, err := cli.NewClient(ctx, cli.Config{
		Address: addr,
		DBName:  "default",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Milvus default database: %w", err)
	}

	// 2. Ensure the agent database exists
	databases, err := defaultClient.ListDatabases(ctx)
	if err != nil {
		defaultClient.Close()
		return nil, fmt.Errorf("failed to list Milvus databases: %w", err)
	}
	dbExists := false
	for _, db := range databases {
		if db.Name == MilvusDBName {
			dbExists = true
			break
		}
	}
	if !dbExists {
		if err := defaultClient.CreateDatabase(ctx, MilvusDBName); err != nil {
			defaultClient.Close()
			return nil, fmt.Errorf("failed to create Milvus database '%s': %w", MilvusDBName, err)
		}
	}

	// 3. Connect to the agent database
	agentClient, err := cli.NewClient(ctx, cli.Config{
		Address: addr,
		DBName:  MilvusDBName,
	})
	if err != nil {
		defaultClient.Close()
		return nil, fmt.Errorf("failed to connect to Milvus agent database: %w", err)
	}

	// 4. Ensure the knowledge collection exists
	collections, err := agentClient.ListCollections(ctx)
	if err != nil {
		defaultClient.Close()
		agentClient.Close()
		return nil, fmt.Errorf("failed to list Milvus collections: %w", err)
	}
	collectionExists := false
	for _, col := range collections {
		if col.Name == MilvusCollectionName {
			collectionExists = true
			break
		}
	}

	if !collectionExists {
		schema := &entity.Schema{
			CollectionName: MilvusCollectionName,
			Description:    "TikTok platform knowledge base collection",
			Fields:         milvusFields,
		}
		if err := agentClient.CreateCollection(ctx, schema, entity.DefaultShardNumber); err != nil {
			defaultClient.Close()
			agentClient.Close()
			return nil, fmt.Errorf("failed to create Milvus collection '%s': %w", MilvusCollectionName, err)
		}

		// Create indexes for efficient search
		idIndex, _ := entity.NewIndexAUTOINDEX(entity.L2)
		_ = agentClient.CreateIndex(ctx, MilvusCollectionName, "id", idIndex, false)

		contentIndex, _ := entity.NewIndexAUTOINDEX(entity.L2)
		_ = agentClient.CreateIndex(ctx, MilvusCollectionName, "content", contentIndex, false)

		vectorIndex, _ := entity.NewIndexAUTOINDEX(entity.COSINE)
		_ = agentClient.CreateIndex(ctx, MilvusCollectionName, "vector", vectorIndex, false)
	}

	defaultClient.Close()
	return agentClient, nil
}
