package aiagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"HuaTug.com/config"

	"github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/markdown"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/google/uuid"
)

// Supported document file extensions for knowledge base indexing.
var SupportedDocExtensions = map[string]bool{
	".md":       true,
	".markdown": true,
	".txt":      true,
	".text":     true,
	".html":     true,
	".htm":      true,
	".json":     true,
	".yaml":     true,
	".yml":      true,
}

// IsSupportedDocFile checks if a filename has a supported extension for knowledge indexing.
func IsSupportedDocFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return SupportedDocExtensions[ext]
}

// BuildKnowledgeIndexingPipeline constructs a pipeline for ingesting documents
// into the Milvus knowledge base.
//
// Pipeline architecture (referencing OncallAgent's knowledge_index_pipeline):
//
//	START → FileLoader → MarkdownSplitter → MilvusIndexer → END
//
// This pipeline:
// 1. Loads documents from files (Markdown, text, etc.)
// 2. Splits documents into chunks using markdown header-based splitting
// 3. Generates embeddings and stores in Milvus vector database
func BuildKnowledgeIndexingPipeline(ctx context.Context) (compose.Runnable[document.Source, []string], error) {
	const (
		FileLoader       = "FileLoader"
		MarkdownSplitter = "MarkdownSplitter"
		MilvusIndexer    = "MilvusIndexer"
	)

	g := compose.NewGraph[document.Source, []string]()

	// Node: FileLoader - Loads documents from file source
	loader, err := newFileLoader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create file loader: %w", err)
	}
	_ = g.AddLoaderNode(FileLoader, loader)

	// Node: MarkdownSplitter - Splits documents by markdown headers
	splitter, err := newMarkdownSplitter(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create markdown splitter: %w", err)
	}
	_ = g.AddDocumentTransformerNode(MarkdownSplitter, splitter)

	// Node: MilvusIndexer - Stores document chunks as vectors in Milvus
	indexer, err := NewMilvusIndexer(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Milvus indexer: %w", err)
	}
	_ = g.AddIndexerNode(MilvusIndexer, indexer)

	// Wire edges
	_ = g.AddEdge(compose.START, FileLoader)
	_ = g.AddEdge(FileLoader, MarkdownSplitter)
	_ = g.AddEdge(MarkdownSplitter, MilvusIndexer)
	_ = g.AddEdge(MilvusIndexer, compose.END)

	r, err := g.Compile(ctx,
		compose.WithGraphName("KnowledgeIndexing"),
		compose.WithNodeTriggerMode(compose.AnyPredecessor),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to compile knowledge indexing pipeline: %w", err)
	}
	return r, nil
}

// IndexSingleFile indexes a single document file into the knowledge base.
// This is used by both the upload handler and the auto-indexing initialization.
func IndexSingleFile(ctx context.Context, filePath string) ([]string, error) {
	pipeline, err := BuildKnowledgeIndexingPipeline(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build indexing pipeline: %w", err)
	}

	// Create a document source from the file path
	source := &document.Source{
		URI: filePath,
	}

	ids, err := pipeline.Invoke(ctx, *source)
	if err != nil {
		return nil, fmt.Errorf("failed to index file '%s': %w", filePath, err)
	}
	return ids, nil
}

// ===== Auto-Indexing on Startup =====

var (
	autoIndexOnce sync.Once
	autoIndexErr  error
)

// InitKnowledgeBase initializes the knowledge base by automatically indexing
// all documents in the configured docs directory. This should be called during
// service startup. It is safe to call multiple times (only runs once).
//
// The function scans the configured docs_dir (default: ./docs/knowledge/)
// for all supported document files and indexes them into Milvus.
func InitKnowledgeBase(ctx context.Context) error {
	autoIndexOnce.Do(func() {
		autoIndexErr = initKnowledgeBaseInternal(ctx)
	})
	return autoIndexErr
}

func initKnowledgeBaseInternal(ctx context.Context) error {
	cfg := config.ConfigInfo.AIAgent
	if !cfg.Enabled {
		hlog.Info("[Knowledge Base] AI Agent is disabled, skipping knowledge base initialization")
		return nil
	}

	docsDir := cfg.DocsDir
	if docsDir == "" {
		docsDir = DefaultDocsDir
	}

	// Check if docs directory exists
	info, err := os.Stat(docsDir)
	if err != nil {
		if os.IsNotExist(err) {
			hlog.Warnf("[Knowledge Base] Docs directory '%s' does not exist, skipping auto-indexing", docsDir)
			return nil
		}
		return fmt.Errorf("failed to access docs directory '%s': %w", docsDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("docs path '%s' is not a directory", docsDir)
	}

	// Scan for supported document files
	var docFiles []string
	err = filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if IsSupportedDocFile(info.Name()) {
			docFiles = append(docFiles, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan docs directory '%s': %w", docsDir, err)
	}

	if len(docFiles) == 0 {
		hlog.Warnf("[Knowledge Base] No supported documents found in '%s', skipping auto-indexing", docsDir)
		return nil
	}

	hlog.Infof("[Knowledge Base] Found %d documents to index in '%s'", len(docFiles), docsDir)

	// Index each file
	totalChunks := 0
	successCount := 0
	for _, filePath := range docFiles {
		hlog.Infof("[Knowledge Base] Indexing: %s", filePath)
		ids, err := IndexSingleFile(ctx, filePath)
		if err != nil {
			hlog.Errorf("[Knowledge Base] Failed to index '%s': %v", filePath, err)
			continue
		}
		successCount++
		totalChunks += len(ids)
		hlog.Infof("[Knowledge Base] Indexed '%s' → %d chunks", filepath.Base(filePath), len(ids))
	}

	hlog.Infof("[Knowledge Base] Auto-indexing complete: %d/%d files indexed, %d total chunks",
		successCount, len(docFiles), totalChunks)

	return nil
}

// ===== Component Factories =====

// newFileLoader creates a file document loader.
func newFileLoader(ctx context.Context) (document.Loader, error) {
	return file.NewFileLoader(ctx, &file.FileLoaderConfig{})
}

// newMarkdownSplitter creates a markdown header-based document splitter.
// This splitter works well for both Markdown files and plain text files
// (plain text without headers will be treated as a single chunk).
func newMarkdownSplitter(ctx context.Context) (document.Transformer, error) {
	cfg := &markdown.HeaderConfig{
		Headers: map[string]string{
			"#":   "title",
			"##":  "subtitle",
			"###": "section",
		},
		TrimHeaders: false,
		IDGenerator: func(ctx context.Context, originalID string, splitIndex int) string {
			return uuid.New().String()
		},
	}
	return markdown.NewHeaderSplitter(ctx, cfg)
}
