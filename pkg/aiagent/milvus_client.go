package aiagent

import (
	"context"
	"fmt"

	"HuaTug.com/config"

	"github.com/cloudwego/hertz/pkg/common/hlog"
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

	// If the collection exists but was created without EnableDynamicField,
	// the milvus-sdk-go will reject OutputFields in search results with:
	//   "extra output fields found and result does not dynamic field"
	// Drop and recreate the collection with the correct schema.
	//
	// Also drop and recreate if the collection description doesn't match the
	// current schema version. This cleans up duplicate chunks that accumulated
	// from server restarts before the dedup-on-index fix was added.
	const collectionDescription = "TikTok platform knowledge base v2"
	if collectionExists {
		col, err := agentClient.DescribeCollection(ctx, MilvusCollectionName)
		needsRecreate := false
		if err == nil && !col.Schema.EnableDynamicField {
			hlog.Warnf("[Milvus] Collection '%s' missing EnableDynamicField, dropping and recreating...", MilvusCollectionName)
			needsRecreate = true
		}
		if err == nil && col.Schema.Description != collectionDescription {
			hlog.Warnf("[Milvus] Collection '%s' schema version mismatch (have=%q, want=%q), dropping and recreating to clean duplicates...",
				MilvusCollectionName, col.Schema.Description, collectionDescription)
			needsRecreate = true
		}
		if needsRecreate {
			if dropErr := agentClient.DropCollection(ctx, MilvusCollectionName); dropErr != nil {
				hlog.Errorf("[Milvus] Failed to drop collection '%s': %v", MilvusCollectionName, dropErr)
			} else {
				collectionExists = false
				// Clear indexed files tracker so all docs get re-indexed
				ResetIndexedFiles()
			}
		}
	}

	if !collectionExists {
		schema := &entity.Schema{
			CollectionName:     MilvusCollectionName,
			Description:        collectionDescription,
			Fields:             milvusFields,
			EnableDynamicField: true,
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

	// 5. Ensure the collection is loaded into memory (required before search)
	if err := agentClient.LoadCollection(ctx, MilvusCollectionName, false); err != nil {
		// Log but don't fail - collection may be empty (no segments to load)
		fmt.Printf("[Milvus] LoadCollection warning (may be empty): %v\n", err)
	}

	defaultClient.Close()
	return agentClient, nil
}

// deleteChunksBySource deletes all document chunks from Milvus that originated
// from a specific source file. This is called before re-indexing a file to
// prevent duplicate chunks from accumulating across server restarts.
func deleteChunksBySource(ctx context.Context, sourceFile string) error {
	client, err := NewMilvusClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Milvus client for deletion: %w", err)
	}
	defer client.Close()

	// Milvus JSON field query: metadata["source"] == "filename.md"
	expr := fmt.Sprintf(`metadata["source"] == "%s"`, sourceFile)
	if err := client.Delete(ctx, MilvusCollectionName, "", expr); err != nil {
		return fmt.Errorf("failed to delete chunks for source '%s': %w", sourceFile, err)
	}
	hlog.Infof("[Knowledge Base] Deleted old chunks for source: %s", sourceFile)
	return nil
}

// flushMilvusCollection flushes the knowledge base collection so that
// GetCollectionStatistics returns accurate row counts. Without an explicit
// flush, newly inserted data sits in growing (unsealed) segments and the
// stats report row_count=0, causing SafeRetriever to skip searches.
func flushMilvusCollection(ctx context.Context) error {
	client, err := NewMilvusClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Milvus client for flush: %w", err)
	}
	defer client.Close()
	if err := client.Flush(ctx, MilvusCollectionName, false); err != nil {
		return fmt.Errorf("failed to flush collection '%s': %w", MilvusCollectionName, err)
	}
	// Reload collection so newly flushed (sealed) segments are available for search.
	// Without this, queries may miss recently indexed data.
	if err := client.LoadCollection(ctx, MilvusCollectionName, false); err != nil {
		hlog.Warnf("[Knowledge Base] LoadCollection after flush warning: %v", err)
	}
	return nil
}
