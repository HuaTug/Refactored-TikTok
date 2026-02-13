package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// Client is an Ollama API client supporting streaming and tool calling
type Client struct {
	baseURL     string
	model       string
	temperature float64
	maxTokens   int
	timeout     time.Duration
	httpClient  *http.Client
}

// ChatMessage represents a message in the Ollama chat API
type ChatMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall represents a tool invocation returned by the model
type ToolCall struct {
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction contains tool function details
type ToolCallFunction struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// Tool defines a tool available to the model
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes the function schema
type ToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// ChatRequest is the request payload for Ollama /api/chat
type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Tools    []Tool        `json:"tools,omitempty"`
	Options  *Options      `json:"options,omitempty"`
}

// Options contains model generation options
type Options struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

// ChatResponse is a single response chunk from Ollama /api/chat
type ChatResponse struct {
	Model           string      `json:"model"`
	CreatedAt       string      `json:"created_at"`
	Message         ChatMessage `json:"message"`
	Done            bool        `json:"done"`
	DoneReason      string      `json:"done_reason,omitempty"`
	TotalDuration   int64       `json:"total_duration,omitempty"`
	EvalCount       int         `json:"eval_count,omitempty"`
	PromptEvalCount int         `json:"prompt_eval_count,omitempty"`
}

// StreamCallback is called for each streamed content chunk
type StreamCallback func(content string)

// NewClient creates a new Ollama client
func NewClient(baseURL, model string, temperature float64, maxTokens, timeoutSec int) *Client {
	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	timeout := time.Duration(timeoutSec) * time.Second

	return &Client{
		baseURL:     baseURL,
		model:       model,
		temperature: temperature,
		maxTokens:   maxTokens,
		timeout:     timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Chat sends a non-streaming chat completion request
func (c *Client) Chat(ctx context.Context, messages []ChatMessage, tools []Tool) (*ChatResponse, error) {
	reqBody := ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
		Tools:    tools,
		Options: &Options{
			Temperature: c.temperature,
			NumPredict:  c.maxTokens,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &chatResp, nil
}

// ChatStream sends a streaming chat completion request, calling the callback for each content chunk
func (c *Client) ChatStream(ctx context.Context, messages []ChatMessage, tools []Tool, callback StreamCallback) (*ChatResponse, error) {
	reqBody := ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
		Tools:    tools,
		Options: &Options{
			Temperature: c.temperature,
			NumPredict:  c.maxTokens,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var lastResp *ChatResponse
	scanner := bufio.NewScanner(resp.Body)
	// Increase max buffer size for large responses
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var chunk ChatResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			hlog.Warnf("[Ollama] Failed to parse stream chunk: %v, raw: %s", err, string(line))
			continue
		}

		lastResp = &chunk

		// If the model returned tool calls, we don't stream content
		if len(chunk.Message.ToolCalls) > 0 {
			return &chunk, nil
		}

		// Stream content to callback
		if chunk.Message.Content != "" && callback != nil {
			callback(chunk.Message.Content)
		}

		if chunk.Done {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return lastResp, fmt.Errorf("stream read error: %w", err)
	}

	return lastResp, nil
}

// IsAvailable checks if the Ollama service is reachable
func (c *Client) IsAvailable(ctx context.Context) bool {
	url := fmt.Sprintf("%s/api/tags", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		hlog.Warnf("[Ollama] Service not available at %s: %v", c.baseURL, err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// GetModel returns the configured model name
func (c *Client) GetModel() string {
	return c.model
}

// ConvertToolsToOllama converts tool definitions to Ollama's tool format
func ConvertToolsToOllama(tools []map[string]interface{}) []Tool {
	var ollamaTools []Tool
	for _, t := range tools {
		name, _ := t["name"].(string)
		desc, _ := t["description"].(string)
		params := t["parameters"]

		ollamaTools = append(ollamaTools, Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		})
	}
	return ollamaTools
}
