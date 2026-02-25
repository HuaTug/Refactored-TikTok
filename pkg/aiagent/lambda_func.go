package aiagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// inputToRagLambda extracts the query string from UserMessage for RAG retrieval.
func inputToRagLambda(ctx context.Context, input *UserMessage, opts ...any) (string, error) {
	return input.Query, nil
}

// inputToChatLambda extracts conversation context from UserMessage for the chat template.
// Note: "documents" is NOT included here; it is provided by the RAG branch (or empty docs lambda).
func inputToChatLambda(ctx context.Context, input *UserMessage, opts ...any) (map[string]any, error) {
	return map[string]any{
		"content": input.Query,
		"history": input.History,
		"date":    time.Now().Format("2006-01-02 15:04:05"),
	}, nil
}

// makeRagLambda creates a lambda that: takes the user query string → calls the retriever → formats results as a string.
// This avoids the type mismatch that occurs when AddRetrieverNode outputs []*schema.Document
// but ChatTemplate expects a string for the {documents} template variable.
func makeRagLambda(r retriever.Retriever) func(ctx context.Context, query string, opts ...any) (map[string]any, error) {
	return func(ctx context.Context, query string, opts ...any) (map[string]any, error) {
		docs, err := r.Retrieve(ctx, query)
		if err != nil {
			hlog.Warnf("[AI Agent] RAG retrieval failed (continuing with empty docs): %v", err)
			return map[string]any{"documents": ""}, nil
		}
		if len(docs) == 0 {
			return map[string]any{"documents": ""}, nil
		}
		// Format retrieved documents as numbered text
		var sb strings.Builder
		for i, doc := range docs {
			sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, doc.Content))
		}
		return map[string]any{"documents": sb.String()}, nil
	}
}

// emptyDocsLambda provides empty documents when RAG is disabled.
func emptyDocsLambda(ctx context.Context, input *UserMessage, opts ...any) (map[string]any, error) {
	return map[string]any{"documents": ""}, nil
}
