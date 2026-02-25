package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	cli "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

func init() {
	// Ensure strings import is used by SafeRetriever logging.
	_ = strings.ReplaceAll
}

// DirectMilvusRetriever performs vector search directly via the milvus-sdk-go
// client, working around a bug in v2.4.2 where requesting OutputFields in
// Search causes "extra output fields found and result does not dynamic field"
// when the search returns 0 results.
//
// Strategy: call Search WITHOUT OutputFields (returns only IDs + scores), then
// fetch the matched documents via Query-by-expression in a second call.
type DirectMilvusRetriever struct {
	client     cli.Client
	collection string
	topK       int
	metricType entity.MetricType
	embedding  embedding.Embedder
}

func (r *DirectMilvusRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	// 1. Embed the query
	vectors, err := r.embedding.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding error: %w", err)
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("expected 1 embedding vector, got %d", len(vectors))
	}

	// 2. Convert float64 → FloatVector (float32)
	fv := make(entity.FloatVector, len(vectors[0]))
	for i, v := range vectors[0] {
		fv[i] = float32(v)
	}

	// 3. Search WITHOUT OutputFields — avoids the milvus-sdk-go v2.4.2 bug
	//    where empty results + OutputFields triggers:
	//    "extra output fields [...] found and result does not dynamic field"
	sp, _ := entity.NewIndexAUTOINDEXSearchParam(1)
	results, err := r.client.Search(
		ctx, r.collection, nil, "",
		nil, // no OutputFields
		[]entity.Vector{fv}, "vector", r.metricType, r.topK, sp,
	)
	if err != nil {
		return nil, fmt.Errorf("vector search error: %w", err)
	}

	// 4. Collect primary-key IDs from search results
	var ids []string
	var scores []float32
	for _, res := range results {
		if res.Err != nil {
			continue
		}
		for i := 0; i < res.IDs.Len(); i++ {
			id, err := res.IDs.GetAsString(i)
			if err != nil {
				continue
			}
			ids = append(ids, id)
		}
		scores = append(scores, res.Scores...)
	}
	if len(ids) == 0 {
		return []*schema.Document{}, nil
	}

	// 5. Query by IDs to fetch content and metadata
	//    Expression: id in ["id1","id2",...]
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = fmt.Sprintf("%q", id)
	}
	expr := fmt.Sprintf("id in [%s]", strings.Join(quoted, ","))

	rs, err := r.client.Query(ctx, r.collection, nil, expr,
		[]string{"id", "content", "metadata"})
	if err != nil {
		return nil, fmt.Errorf("query-by-id error: %w", err)
	}

	// 6. Build a lookup map from Query results
	type docData struct {
		content  string
		metadata map[string]any
	}
	lookup := make(map[string]*docData, rs.Len())

	idCol := rs.GetColumn("id")
	contentCol := rs.GetColumn("content")
	metadataCol := rs.GetColumn("metadata")

	for i := 0; i < rs.Len(); i++ {
		dd := &docData{metadata: make(map[string]any)}
		if idCol != nil {
			val, _ := idCol.GetAsString(i)
			if contentCol != nil {
				dd.content, _ = contentCol.GetAsString(i)
			}
			if metadataCol != nil {
				raw, err2 := metadataCol.Get(i)
				if err2 == nil {
					if b, ok := raw.([]byte); ok {
						_ = json.Unmarshal(b, &dd.metadata)
					}
				}
			}
			lookup[val] = dd
		}
	}

	// 7. Assemble documents in search-rank order
	docs := make([]*schema.Document, 0, len(ids))
	for idx, id := range ids {
		doc := &schema.Document{
			ID:       id,
			MetaData: make(map[string]any),
		}
		if idx < len(scores) {
			doc.MetaData["_score"] = scores[idx]
		}
		if dd, ok := lookup[id]; ok {
			doc.Content = dd.content
			for k, v := range dd.metadata {
				doc.MetaData[k] = v
			}
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

func (r *DirectMilvusRetriever) GetType() string {
	return "DirectMilvusRetriever"
}

func (r *DirectMilvusRetriever) IsCallbacksEnabled() bool {
	return false
}

// SafeRetriever wraps a retriever.Retriever and returns empty results on error
// instead of propagating the error and crashing the pipeline.
type SafeRetriever struct {
	inner retriever.Retriever
}

func (s *SafeRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	docs, err := s.inner.Retrieve(ctx, query, opts...)
	if err != nil {
		hlog.Warnf("[AI Agent] Milvus retriever error (returning empty docs): %v", err)
		return []*schema.Document{}, nil
	}
	if len(docs) == 0 {
		hlog.Debugf("[AI Agent] RAG search returned 0 documents for query: %s", query)
	} else {
		hlog.Infof("[AI Agent] RAG search returned %d documents for query: %s", len(docs), query)
		// Log document previews and scores for debugging retrieval quality
		for i, doc := range docs {
			preview := doc.Content
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			preview = strings.ReplaceAll(preview, "\n", " ")
			score, _ := doc.MetaData["_score"]
			hlog.Infof("[AI Agent] RAG doc[%d] score=%.4f preview: %s", i, score, preview)
		}
	}
	return docs, nil
}

func (s *SafeRetriever) GetType() string {
	return "SafeMilvusRetriever"
}

func (s *SafeRetriever) IsCallbacksEnabled() bool {
	return false
}

// NewMilvusRetriever creates a Milvus-based vector retriever for RAG queries.
// Uses DirectMilvusRetriever which avoids the milvus-sdk-go v2.4.2 bug where
// Search with OutputFields fails on empty results. Wrapped in SafeRetriever
// for extra resilience.
func NewMilvusRetriever(ctx context.Context) (retriever.Retriever, error) {
	client, err := NewMilvusClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Milvus client for retriever: %w", err)
	}
	eb, err := NewEmbedder(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder for retriever: %w", err)
	}
	inner := &DirectMilvusRetriever{
		client:     client,
		collection: MilvusCollectionName,
		topK:       3,
		metricType: entity.COSINE,
		embedding:  eb,
	}
	return &SafeRetriever{inner: inner}, nil
}
