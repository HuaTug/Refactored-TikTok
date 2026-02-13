package aiagent

import (
	"context"
	"time"
)

// inputToRagLambda extracts the query string from UserMessage for RAG retrieval.
func inputToRagLambda(ctx context.Context, input *UserMessage, opts ...any) (string, error) {
	return input.Query, nil
}

// inputToChatLambda extracts conversation context from UserMessage for the chat template.
func inputToChatLambda(ctx context.Context, input *UserMessage, opts ...any) (map[string]any, error) {
	return map[string]any{
		"content": input.Query,
		"history": input.History,
		"date":    time.Now().Format("2006-01-02 15:04:05"),
	}, nil
}
