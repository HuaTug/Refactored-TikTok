package aiagent

import (
	"context"
	"fmt"

	"HuaTug.com/config"

	oaiemb "github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
)

// NewEmbedder creates an embedding service using the local Ollama instance.
// Ollama exposes an OpenAI-compatible embedding endpoint at /v1/embeddings,
// so we use the eino OpenAI embedding component to connect to it.
func NewEmbedder(ctx context.Context) (embedding.Embedder, error) {
	cfg := config.ConfigInfo.AIAgent

	// Determine embedding model: prefer ai_agent.embedding.model, fallback to nomic-embed-text
	modelName := cfg.Embedding.Model
	if modelName == "" {
		modelName = "nomic-embed-text"
	}

	// Use Ollama's OpenAI-compatible endpoint
	baseURL := ollamaOpenAIBaseURL()

	// Ollama doesn't need a real API key
	apiKey := cfg.Embedding.APIKey
	if apiKey == "" || apiKey == "your-dashscope-api-key-here" {
		apiKey = "ollama"
	}

	eb, err := oaiemb.NewEmbedder(ctx, &oaiemb.EmbeddingConfig{
		Model:   modelName,
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Ollama embedder (model=%s, base_url=%s): %w", modelName, baseURL, err)
	}
	return eb, nil
}
