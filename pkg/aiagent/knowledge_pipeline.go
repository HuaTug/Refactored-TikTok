package aiagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"HuaTug.com/config"

	"github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/markdown"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/fsnotify/fsnotify"
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
// It first deletes any previously indexed chunks from the same source file
// to prevent duplicates from accumulating across server restarts.
func IndexSingleFile(ctx context.Context, filePath string) ([]string, error) {
	sourceFile := filepath.Base(filePath)

	// Delete old chunks for this file before re-indexing to prevent duplicates.
	// This is critical because indexedFiles is an in-memory map that resets on
	// each server restart, causing all files to be re-indexed.
	if err := deleteChunksBySource(ctx, sourceFile); err != nil {
		hlog.Warnf("[Knowledge Base] Failed to delete old chunks for '%s' (continuing): %v", sourceFile, err)
	}

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

// ===== Auto-Indexing with File Change Tracking =====

var (
	// indexedFiles tracks which files have been indexed and their last modification time.
	// Key: absolute file path, Value: modification time at the time of last indexing.
	indexedFiles   = make(map[string]time.Time)
	indexedFilesMu sync.Mutex

	// watcherStarted ensures we only start one file watcher goroutine.
	watcherOnce sync.Once

	// knowledgeReady signals that the initial knowledge base indexing is complete.
	// AI queries can check this to avoid serving stale results during startup.
	knowledgeReady     bool
	knowledgeReadyOnce sync.Once
	knowledgeReadyCh   = make(chan struct{})
)

// IsKnowledgeBaseReady returns true if the initial knowledge base indexing has completed.
func IsKnowledgeBaseReady() bool {
	return knowledgeReady
}

// WaitForKnowledgeBase blocks until the knowledge base is ready or the context is cancelled.
// Returns true if ready, false if context was cancelled.
func WaitForKnowledgeBase(ctx context.Context) bool {
	if knowledgeReady {
		return true
	}
	select {
	case <-knowledgeReadyCh:
		return true
	case <-ctx.Done():
		return false
	}
}

// markKnowledgeReady signals that initial indexing is complete.
func markKnowledgeReady() {
	knowledgeReadyOnce.Do(func() {
		knowledgeReady = true
		close(knowledgeReadyCh)
		hlog.Info("[Knowledge Base] Initial indexing complete, knowledge base is ready for queries")
	})
}

// ResetIndexedFiles clears the file tracking state, forcing all documents to be
// re-indexed on the next InitKnowledgeBase call.
func ResetIndexedFiles() {
	indexedFilesMu.Lock()
	indexedFiles = make(map[string]time.Time)
	indexedFilesMu.Unlock()
	hlog.Info("[Knowledge Base] Indexed files tracking reset, all files will be re-indexed")
}

// InitKnowledgeBase indexes all new or updated documents in the knowledge directory.
// It is safe to call multiple times — only files that are new or changed since
// the last indexing will be processed. It also starts a file watcher (once) to
// automatically re-index on file changes.
func InitKnowledgeBase(ctx context.Context) error {
	cfg := config.ConfigInfo.AIAgent
	if !cfg.Enabled {
		hlog.Info("[Knowledge Base] AI Agent is disabled, skipping knowledge base initialization")
		return nil
	}

	docsDir := cfg.DocsDir
	if docsDir == "" {
		docsDir = DefaultDocsDir
	}

	// Resolve relative docsDir against the project root.
	// When running `make api` (cd cmd/api && ./api), the working directory is
	// cmd/api/, so "./docs/knowledge/" would resolve to cmd/api/docs/knowledge/
	// which doesn't exist. We derive the project root from the config file
	// location (always at <project_root>/config/config.yml).
	docsDir = config.ResolveProjectPath(docsDir)

	hlog.Infof("[Knowledge Base] Using docs directory: %s", docsDir)

	// Index all new/changed files
	if err := indexNewOrChangedFiles(ctx, docsDir); err != nil {
		return err
	}

	// Mark knowledge base as ready for queries
	markKnowledgeReady()

	// Start the file watcher (only once, stays running in background)
	watcherOnce.Do(func() {
		go startFileWatcher(docsDir)
	})

	return nil
}

// indexNewOrChangedFiles scans the docs directory and indexes files that are
// new or have been modified since their last indexing.
func indexNewOrChangedFiles(ctx context.Context, docsDir string) error {
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
	var filesToIndex []string
	err = filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !IsSupportedDocFile(info.Name()) {
			return nil
		}
		absPath, _ := filepath.Abs(path)

		indexedFilesMu.Lock()
		lastIndexed, exists := indexedFiles[absPath]
		indexedFilesMu.Unlock()

		if !exists || info.ModTime().After(lastIndexed) {
			filesToIndex = append(filesToIndex, absPath)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan docs directory '%s': %w", docsDir, err)
	}

	if len(filesToIndex) == 0 {
		hlog.Info("[Knowledge Base] All documents are up to date, nothing to index")
		return nil
	}

	hlog.Infof("[Knowledge Base] Found %d new/updated documents to index", len(filesToIndex))

	// Index each file
	totalChunks := 0
	successCount := 0
	for _, filePath := range filesToIndex {
		hlog.Infof("[Knowledge Base] Indexing: %s", filePath)
		ids, err := IndexSingleFile(ctx, filePath)
		if err != nil {
			hlog.Errorf("[Knowledge Base] Failed to index '%s': %v", filePath, err)
			continue
		}
		successCount++
		totalChunks += len(ids)

		// Record successful indexing
		indexedFilesMu.Lock()
		indexedFiles[filePath] = time.Now()
		indexedFilesMu.Unlock()

		hlog.Infof("[Knowledge Base] Indexed '%s' → %d chunks", filepath.Base(filePath), len(ids))
	}

	if successCount > 0 {
		// Flush Milvus so GetCollectionStatistics reflects the newly inserted data.
		// Without this flush, the SafeRetriever sees row_count=0 and skips search.
		if err := flushMilvusCollection(ctx); err != nil {
			hlog.Warnf("[Knowledge Base] Flush warning: %v", err)
		}

		// Reset the chat agent cache so it picks up new knowledge
		ResetChatAgent()
		hlog.Infof("[Knowledge Base] Indexing complete: %d/%d files indexed, %d total chunks",
			successCount, len(filesToIndex), totalChunks)
	}

	return nil
}

// startFileWatcher monitors the knowledge docs directory for file changes
// and automatically re-indexes new or modified documents.
func startFileWatcher(docsDir string) {
	absDir, err := filepath.Abs(docsDir)
	if err != nil {
		hlog.Errorf("[Knowledge Watcher] Failed to resolve docs directory: %v", err)
		return
	}

	// Ensure directory exists
	if err := os.MkdirAll(absDir, 0755); err != nil {
		hlog.Errorf("[Knowledge Watcher] Failed to create docs directory '%s': %v", absDir, err)
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		hlog.Errorf("[Knowledge Watcher] Failed to create file watcher: %v", err)
		return
	}
	// Note: intentionally not closing watcher — it runs for the lifetime of the process

	if err := watcher.Add(absDir); err != nil {
		hlog.Errorf("[Knowledge Watcher] Failed to watch directory '%s': %v", absDir, err)
		watcher.Close()
		return
	}

	hlog.Infof("[Knowledge Watcher] Watching '%s' for document changes...", absDir)

	// Debounce timer to batch rapid file changes (e.g. editor save = truncate + write)
	var debounceTimer *time.Timer
	pendingFiles := make(map[string]struct{})
	var pendingMu sync.Mutex

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Only care about create/write events on supported files
			if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
				continue
			}
			if !IsSupportedDocFile(filepath.Base(event.Name)) {
				continue
			}

			pendingMu.Lock()
			pendingFiles[event.Name] = struct{}{}
			// Reset debounce timer (wait 2s after last change before indexing)
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(2*time.Second, func() {
				pendingMu.Lock()
				files := make([]string, 0, len(pendingFiles))
				for f := range pendingFiles {
					files = append(files, f)
				}
				pendingFiles = make(map[string]struct{})
				pendingMu.Unlock()

				if len(files) == 0 {
					return
				}

				hlog.Infof("[Knowledge Watcher] Detected %d file change(s), re-indexing...", len(files))
				ctx := context.Background()
				for _, f := range files {
					absPath, _ := filepath.Abs(f)
					ids, err := IndexSingleFile(ctx, absPath)
					if err != nil {
						hlog.Errorf("[Knowledge Watcher] Failed to index '%s': %v", filepath.Base(f), err)
						continue
					}
					indexedFilesMu.Lock()
					indexedFiles[absPath] = time.Now()
					indexedFilesMu.Unlock()
					hlog.Infof("[Knowledge Watcher] Re-indexed '%s' → %d chunks", filepath.Base(f), len(ids))
				}
				// Flush so stats reflect new data
				if err := flushMilvusCollection(ctx); err != nil {
					hlog.Warnf("[Knowledge Watcher] Flush warning: %v", err)
				}
				// Reset chat agent so it picks up new knowledge
				ResetChatAgent()
			})
			pendingMu.Unlock()

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			hlog.Errorf("[Knowledge Watcher] File watcher error: %v", err)
		}
	}
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
	splitter, err := markdown.NewHeaderSplitter(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Wrap the splitter to inject source metadata into each chunk.
	// This enables deleteChunksBySource to remove old chunks before re-indexing.
	return &sourceInjectingTransformer{inner: splitter}, nil
}

// sourceInjectingTransformer wraps a document.Transformer to inject the source
// filename into each chunk's metadata. This metadata is stored in Milvus JSON
// field and used by deleteChunksBySource for deduplication.
type sourceInjectingTransformer struct {
	inner document.Transformer
}

func (t *sourceInjectingTransformer) Transform(ctx context.Context, docs []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	// Extract source filename from the first document's metadata (set by FileLoader)
	var source string
	if len(docs) > 0 && docs[0].MetaData != nil {
		// eino FileLoader stores the source path in "_source" metadata
		if src, ok := docs[0].MetaData["_source"].(string); ok {
			source = filepath.Base(src)
		}
	}

	result, err := t.inner.Transform(ctx, docs, opts...)
	if err != nil {
		return nil, err
	}

	// Inject source into each chunk's metadata
	for _, doc := range result {
		if doc.MetaData == nil {
			doc.MetaData = make(map[string]any)
		}
		if source != "" {
			doc.MetaData["source"] = source
		}
	}
	return result, nil
}
