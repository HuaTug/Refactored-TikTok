package aiagent

import (
	"context"
	"fmt"
	"strings"

	"HuaTug.com/config"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

// ollamaOpenAIBaseURL returns the OpenAI-compatible API base URL for the local Ollama instance.
// Ollama exposes an OpenAI-compatible endpoint at /v1 (e.g. http://localhost:11434/v1).
func ollamaOpenAIBaseURL() string {
	base := config.ConfigInfo.Ollama.BaseURL
	if base == "" {
		base = "http://localhost:11434"
	}
	return strings.TrimRight(base, "/") + "/v1"
}

// NewChatModel creates a ToolCallingChatModel via the local Ollama service
// using the OpenAI-compatible endpoint. No external API key is needed.
func NewChatModel(ctx context.Context) (model.ToolCallingChatModel, error) {
	cfg := config.ConfigInfo.AIAgent
	ollamaCfg := config.ConfigInfo.Ollama

	// Determine the model name: prefer ai_agent.chat_model.model, fallback to ollama.model
	modelName := cfg.ChatModel.Model
	if modelName == "" {
		modelName = ollamaCfg.Model
	}
	if modelName == "" {
		return nil, fmt.Errorf("no model configured: set ai_agent.chat_model.model or ollama.model in config")
	}

	// Determine the base URL: prefer ai_agent.chat_model.base_url, fallback to Ollama's OpenAI endpoint
	baseURL := cfg.ChatModel.BaseURL
	if baseURL == "" {
		baseURL = ollamaOpenAIBaseURL()
	}

	// API key: Ollama doesn't require a real key, use "ollama" as placeholder
	apiKey := cfg.ChatModel.APIKey
	if apiKey == "" || apiKey == "your-api-key-here" {
		apiKey = "ollama"
	}

	cm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:   modelName,
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create chat model (model=%s, base_url=%s): %w", modelName, baseURL, err)
	}
	return cm, nil
}

// NewThinkModel creates a reasoning LLM instance for planning tasks.
// When using Ollama, this typically points to the same model as the chat model,
// but can be configured separately if you have a dedicated reasoning model.
func NewThinkModel(ctx context.Context) (model.ToolCallingChatModel, error) {
	cfg := config.ConfigInfo.AIAgent
	ollamaCfg := config.ConfigInfo.Ollama

	// Determine model: prefer ai_agent.think_model.model, fallback to chat model, then ollama.model
	modelName := cfg.ThinkModel.Model
	if modelName == "" {
		modelName = cfg.ChatModel.Model
	}
	if modelName == "" {
		modelName = ollamaCfg.Model
	}
	if modelName == "" {
		return nil, fmt.Errorf("no think model configured")
	}

	baseURL := cfg.ThinkModel.BaseURL
	if baseURL == "" {
		baseURL = ollamaOpenAIBaseURL()
	}

	apiKey := cfg.ThinkModel.APIKey
	if apiKey == "" || apiKey == "your-api-key-here" {
		apiKey = "ollama"
	}

	cm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:   modelName,
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create think model (model=%s, base_url=%s): %w", modelName, baseURL, err)
	}
	return cm, nil
}
