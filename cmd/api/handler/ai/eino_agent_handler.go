package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"HuaTug.com/config"
	"HuaTug.com/pkg/aiagent"
	"HuaTug.com/pkg/errno"
	ollamapkg "HuaTug.com/pkg/ollama"

	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// ========== Eino Agent Handler (upgraded) ==========

// EinoChatHandler handles non-streaming chat requests using the Eino-based
// ReAct Agent with RAG knowledge base integration.
func EinoChatHandler(ctx context.Context, c *app.RequestContext) {
	userID := getUserIDFromContext(c)

	var req ChatRequest
	if err := c.Bind(&req); err != nil {
		sendResponse(c, errno.ParamErr, nil)
		return
	}
	if req.Message == "" {
		sendResponse(c, errno.ParamErr.WithMessage("message cannot be empty"), nil)
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("eino_%d_%d", userID, time.Now().UnixNano())
	}

	// Use a generous timeout for large local models (e.g. qwen3-coder:30b)
	agentCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// Get conversation memory
	memory := aiagent.GetSimpleMemory(sessionID)

	// Build user message for the Eino pipeline
	userMessage := &aiagent.UserMessage{
		ID:      sessionID,
		Query:   req.Message,
		History: memory.GetMessages(),
	}

	// Build and invoke the chat agent pipeline
	runner, err := aiagent.BuildChatAgent(agentCtx)
	if err != nil {
		hlog.Errorf("[Eino Agent] Failed to build chat agent: %v", err)
		hlog.Info("[Eino Agent] Falling back to Ollama handler")
		// Fallback: use Ollama directly within this handler (avoid body re-consumption)
		fallbackOllamaChat(ctx, c, req, sessionID)
		return
	}

	out, err := runner.Invoke(agentCtx, userMessage)
	if err != nil {
		hlog.Errorf("[Eino Agent] Chat invocation failed: %v", err)
		// Reset cached pipeline on Milvus errors so next request rebuilds with LoadCollection
		if strings.Contains(err.Error(), "collection not loaded") || strings.Contains(err.Error(), "milvus") {
			hlog.Info("[Eino Agent] Milvus-related error detected, resetting agent cache for next request")
			aiagent.ResetChatAgent()
		}
		fallbackOllamaChat(ctx, c, req, sessionID)
		return
	}

	// Store conversation history
	memory.SetMessages(schema.UserMessage(req.Message))
	memory.SetMessages(schema.SystemMessage(out.Content))

	// Auto-name session
	title := req.Message
	if len([]rune(title)) > 20 {
		title = string([]rune(title)[:20]) + "..."
	}

	sendResponse(c, nil, map[string]interface{}{
		"reply":      out.Content,
		"session_id": sessionID,
		"title":      title,
		"mode":       "eino_agent",
	})
}

// fallbackOllamaChat handles non-streaming fallback using Ollama directly.
// This avoids the body re-consumption issue when calling ChatHandler.
func fallbackOllamaChat(ctx context.Context, c *app.RequestContext, req ChatRequest, sessionID string) {
	client := getOllamaClient()
	if client == nil || !client.IsAvailable(ctx) {
		hlog.Warn("[Eino Fallback] Ollama also unavailable, returning error")
		sendResponse(c, errno.ServiceErr.WithMessage("AI service temporarily unavailable"), nil)
		return
	}

	// Ollama chat with tool support
	messages := []ollamapkg.ChatMessage{
		{Role: "system", Content: getSystemPrompt()},
		{Role: "user", Content: req.Message},
	}
	tools := getOllamaTools()

	resp, err := client.Chat(ctx, messages, tools)
	if err != nil {
		hlog.Errorf("[Eino Fallback] Ollama chat failed: %v", err)
		sendResponse(c, errno.ServiceErr.WithMessage("AI service failed"), nil)
		return
	}

	// Handle tool calls (multi-round)
	maxToolRounds := 3
	for round := 0; round < maxToolRounds && len(resp.Message.ToolCalls) > 0; round++ {
		hlog.Infof("[Eino Fallback] Ollama requested %d tool calls (round %d)", len(resp.Message.ToolCalls), round+1)
		messages = append(messages, ollamapkg.ChatMessage{
			Role:      "assistant",
			Content:   resp.Message.Content,
			ToolCalls: resp.Message.ToolCalls,
		})
		for _, tc := range resp.Message.ToolCalls {
			hlog.Infof("[Eino Fallback] Executing tool: %s", tc.Function.Name)
			result, toolErr := executeToolCallFromMap(ctx, tc.Function.Name, tc.Function.Arguments)
			if toolErr != nil {
				result = fmt.Sprintf("Error: %v", toolErr)
			}
			messages = append(messages, ollamapkg.ChatMessage{Role: "tool", Content: result})
		}
		resp, err = client.Chat(ctx, messages, nil)
		if err != nil {
			hlog.Errorf("[Eino Fallback] Ollama follow-up failed: %v", err)
			sendResponse(c, errno.ServiceErr.WithMessage("AI service failed processing tools"), nil)
			return
		}
	}

	title := req.Message
	if len([]rune(title)) > 20 {
		title = string([]rune(title)[:20]) + "..."
	}

	sendResponse(c, nil, map[string]interface{}{
		"reply":      resp.Message.Content,
		"session_id": sessionID,
		"title":      title,
		"mode":       "ollama_fallback",
	})
}

// EinoChatSSE handles SSE streaming chat requests using the Eino-based Agent.
func EinoChatSSE(ctx context.Context, c *app.RequestContext) {
	userID := getUserIDFromContext(c)

	var req ChatRequest
	if err := c.Bind(&req); err != nil {
		sendResponse(c, errno.ParamErr, nil)
		return
	}
	if req.Message == "" {
		sendResponse(c, errno.ParamErr.WithMessage("message cannot be empty"), nil)
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("eino_%d_%d", userID, time.Now().UnixNano())
	}

	// Use a generous timeout for large local models (e.g. qwen3-coder:30b)
	agentCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)

	memory := aiagent.GetSimpleMemory(sessionID)
	userMessage := &aiagent.UserMessage{
		ID:      sessionID,
		Query:   req.Message,
		History: memory.GetMessages(),
	}

	// Set SSE headers first (before any fallback, so the client always gets SSE format)
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.Set("Access-Control-Allow-Origin", "*")
	c.SetStatusCode(consts.StatusOK)

	// Create pipe for streaming
	pr, pw := io.Pipe()
	c.SetBodyStream(pr, -1)

	message := req.Message

	// Build the agent
	buildStart := time.Now()

	// Wait briefly for knowledge base indexing to complete if it's still in progress
	// This prevents serving queries with stale/empty RAG results during startup
	if !aiagent.IsKnowledgeBaseReady() {
		hlog.Info("[Eino Agent] Waiting for knowledge base initial indexing to complete...")
		waitCtx, waitCancel := context.WithTimeout(agentCtx, 30*time.Second)
		if aiagent.WaitForKnowledgeBase(waitCtx) {
			hlog.Info("[Eino Agent] Knowledge base ready, proceeding with query")
		} else {
			hlog.Warn("[Eino Agent] Knowledge base not ready after 30s, proceeding anyway")
		}
		waitCancel()
	}

	runner, err := aiagent.BuildChatAgent(agentCtx)
	hlog.Infof("[Eino Agent] BuildChatAgent took %v", time.Since(buildStart))
	useEinoAgent := err == nil
	if err != nil {
		hlog.Errorf("[Eino Agent] Failed to build chat agent for stream: %v", err)
		hlog.Info("[Eino Agent] Will fallback to Ollama SSE within the same stream")
	}

	go func() {
		defer pw.Close()
		defer cancel()

		writeSSE := func(eventData interface{}) {
			data, _ := json.Marshal(eventData)
			pw.Write([]byte(fmt.Sprintf("data: %s\n\n", data))) //nolint:errcheck
		}

		var fullResponse strings.Builder
		mode := "eino_agent"

		if useEinoAgent {
			// Use Eino Agent stream
			streamStart := time.Now()
			hlog.Infof("[Eino Agent] Starting stream for query: %s", message)
			sr, err := runner.Stream(agentCtx, userMessage)
			if err != nil {
				hlog.Errorf("[Eino Agent] Stream failed: %v, falling back to Ollama", err)
				// Reset cached pipeline so it will be rebuilt (with LoadCollection) on next request
				if strings.Contains(err.Error(), "collection not loaded") || strings.Contains(err.Error(), "milvus") {
					hlog.Info("[Eino Agent] Milvus-related error detected, resetting agent cache for next request")
					aiagent.ResetChatAgent()
				}
				useEinoAgent = false
			} else {
				defer sr.Close()
				firstChunk := true
				chunkCount := 0
				for {
					chunk, err := sr.Recv()
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						hlog.Errorf("[Eino Agent] Stream recv error: %v", err)
						writeSSE(map[string]string{"type": "error", "content": err.Error()})
						break
					}
					if firstChunk {
						hlog.Infof("[Eino Agent] First token arrived in %v", time.Since(streamStart))
						firstChunk = false
					}
					chunkCount++
					fullResponse.WriteString(chunk.Content)
					writeSSE(map[string]string{"type": "content", "content": chunk.Content})
				}
				hlog.Infof("[Eino Agent] Stream completed: %d chunks in %v", chunkCount, time.Since(streamStart))
			}
		}

		// Fallback to Ollama SSE if Eino Agent is not available
		if !useEinoAgent {
			mode = "ollama_fallback"
			client := getOllamaClient()
			if client != nil && client.IsAvailable(ctx) {
				messages := buildSimpleOllamaMessages(message)
				tools := getOllamaTools()

				// Tool-calling loop: Ollama may return tool calls instead of content
				maxToolRounds := 3
				for round := 0; round <= maxToolRounds; round++ {
					resp, ollamaErr := client.ChatStream(ctx, messages, tools, func(content string) {
						fullResponse.WriteString(content)
						writeSSE(map[string]string{"type": "content", "content": content})
					})
					if ollamaErr != nil {
						hlog.Errorf("[Eino Fallback] Ollama stream failed: %v", ollamaErr)
						writeSSE(map[string]string{"type": "error", "content": "AI service temporarily unavailable, please try again later."})
						break
					}
					if resp == nil || len(resp.Message.ToolCalls) == 0 {
						break // No tool calls, streaming is done
					}

					// Process tool calls
					hlog.Infof("[Eino Fallback] Ollama requested %d tool calls (round %d)", len(resp.Message.ToolCalls), round+1)
					messages = append(messages, ollamapkg.ChatMessage{
						Role:      "assistant",
						Content:   resp.Message.Content,
						ToolCalls: resp.Message.ToolCalls,
					})
					for _, tc := range resp.Message.ToolCalls {
						hlog.Infof("[Eino Fallback] Executing tool: %s", tc.Function.Name)
						result, err := executeToolCallFromMap(ctx, tc.Function.Name, tc.Function.Arguments)
						if err != nil {
							result = fmt.Sprintf("Error: %v", err)
						}
						messages = append(messages, ollamapkg.ChatMessage{Role: "tool", Content: result})
					}
					// Next round: no tools, force text response
					tools = nil
				}
			} else {
				hlog.Warn("[Eino Fallback] Ollama client not available")
				writeSSE(map[string]string{"type": "error", "content": "AI service temporarily unavailable."})
			}
		}

		// Store conversation history
		completeResponse := fullResponse.String()
		if completeResponse != "" {
			memory.SetMessages(schema.UserMessage(message))
			memory.SetMessages(schema.SystemMessage(completeResponse))
		}

		// Auto-name session
		title := message
		if len([]rune(title)) > 20 {
			title = string([]rune(title)[:20]) + "..."
		}

		writeSSE(map[string]interface{}{
			"type":       "done",
			"session_id": sessionID,
			"title":      title,
			"mode":       mode,
		})
	}()
}

// buildSimpleOllamaMessages constructs a minimal Ollama message list for fallback.
func buildSimpleOllamaMessages(userMsg string) []ollamapkg.ChatMessage {
	return []ollamapkg.ChatMessage{
		{Role: "system", Content: getSystemPrompt()},
		{Role: "user", Content: userMsg},
	}
}

// EinoHealthCheck checks both Eino agent and Ollama connectivity.
func EinoHealthCheck(ctx context.Context, c *app.RequestContext) {
	status := map[string]interface{}{
		"eino_agent_enabled": config.ConfigInfo.AIAgent.Enabled,
		"ollama_enabled":     config.ConfigInfo.Ollama.Enabled,
	}

	// Check Eino agent
	if config.ConfigInfo.AIAgent.Enabled {
		_, err := aiagent.BuildChatAgent(ctx)
		if err != nil {
			status["eino_agent_status"] = "error"
			status["eino_agent_error"] = err.Error()
		} else {
			status["eino_agent_status"] = "ready"
		}
		// Show the effective model name (falls back to ollama.model if not set)
		chatModelName := config.ConfigInfo.AIAgent.ChatModel.Model
		if chatModelName == "" {
			chatModelName = config.ConfigInfo.Ollama.Model
		}
		status["eino_chat_model"] = chatModelName

		embModelName := config.ConfigInfo.AIAgent.Embedding.Model
		if embModelName == "" {
			embModelName = "nomic-embed-text"
		}
		status["eino_embedding_model"] = embModelName
		status["eino_milvus_address"] = config.ConfigInfo.AIAgent.Milvus.Address
		status["eino_backend"] = "ollama (OpenAI-compatible)"
	}

	// Check Ollama
	client := getOllamaClient()
	if client != nil && client.IsAvailable(ctx) {
		status["ollama_status"] = "connected"
	} else {
		status["ollama_status"] = "disconnected"
	}
	status["ollama_model"] = config.ConfigInfo.Ollama.Model

	// Determine active mode
	if config.ConfigInfo.AIAgent.Enabled {
		status["active_mode"] = "eino_agent"
	} else if config.ConfigInfo.Ollama.Enabled {
		status["active_mode"] = "ollama"
	} else {
		status["active_mode"] = "fallback"
	}

	sendResponse(c, nil, status)
}

// KnowledgeUploadHandler handles document uploads to the knowledge base.
// It accepts multipart file uploads, validates the file type, saves to the
// knowledge docs directory, and runs the knowledge indexing pipeline.
//
// Request: multipart/form-data with field "file" containing the document.
// Supported formats: .md, .markdown, .txt, .text, .html, .htm, .json, .yaml, .yml
func KnowledgeUploadHandler(ctx context.Context, c *app.RequestContext) {
	if !config.ConfigInfo.AIAgent.Enabled {
		sendResponse(c, errno.ParamErr.WithMessage("AI Agent is not enabled"), nil)
		return
	}

	// 1. Get the uploaded file from multipart form
	fileHeader, err := c.FormFile("file")
	if err != nil {
		sendResponse(c, errno.ParamErr.WithMessage("No file uploaded. Use multipart form field 'file'."), nil)
		return
	}

	// 2. Validate file type
	filename := fileHeader.Filename
	if !aiagent.IsSupportedDocFile(filename) {
		sendResponse(c, errno.ParamErr.WithMessage(fmt.Sprintf(
			"Unsupported file type: '%s'. Supported: .md, .markdown, .txt, .text, .html, .htm, .json, .yaml, .yml",
			filename,
		)), nil)
		return
	}

	// 3. Validate file size (max 10MB)
	const maxFileSize = 10 * 1024 * 1024
	if fileHeader.Size > maxFileSize {
		sendResponse(c, errno.ParamErr.WithMessage("File too large. Maximum size is 10MB."), nil)
		return
	}

	// 4. Determine save directory
	docsDir := config.ConfigInfo.AIAgent.DocsDir
	if docsDir == "" {
		docsDir = aiagent.DefaultDocsDir
	}
	// Resolve relative path against project root (cwd may be cmd/api/)
	docsDir = config.ResolveProjectPath(docsDir)

	// Ensure docs directory exists
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		hlog.Errorf("[Knowledge Upload] Failed to create docs directory '%s': %v", docsDir, err)
		sendResponse(c, errno.ServiceErr.WithMessage("Failed to create storage directory"), nil)
		return
	}

	// 5. Save file to docs directory
	savePath := filepath.Join(docsDir, filename)

	// Check if file already exists; add timestamp suffix if needed
	if _, err := os.Stat(savePath); err == nil {
		ext := filepath.Ext(filename)
		nameWithoutExt := strings.TrimSuffix(filename, ext)
		savePath = filepath.Join(docsDir, fmt.Sprintf("%s_%d%s", nameWithoutExt, time.Now().Unix(), ext))
	}

	if err := c.SaveUploadedFile(fileHeader, savePath); err != nil {
		hlog.Errorf("[Knowledge Upload] Failed to save file to '%s': %v", savePath, err)
		sendResponse(c, errno.ServiceErr.WithMessage("Failed to save uploaded file"), nil)
		return
	}

	hlog.Infof("[Knowledge Upload] File saved: %s (%d bytes)", savePath, fileHeader.Size)

	// 6. Run the knowledge indexing pipeline on the uploaded file
	ids, indexErr := aiagent.IndexSingleFile(ctx, savePath)
	if indexErr != nil {
		hlog.Errorf("[Knowledge Upload] Indexing failed for '%s': %v", savePath, indexErr)
		// File is saved but indexing failed - still report partial success
		sendResponse(c, nil, map[string]interface{}{
			"status":    "partial",
			"message":   fmt.Sprintf("File saved but indexing failed: %v", indexErr),
			"filename":  filepath.Base(savePath),
			"file_size": fileHeader.Size,
			"chunks":    0,
		})
		return
	}

	hlog.Infof("[Knowledge Upload] Successfully indexed '%s' → %d chunks", filepath.Base(savePath), len(ids))

	sendResponse(c, nil, map[string]interface{}{
		"status":    "success",
		"message":   "Document uploaded and indexed successfully",
		"filename":  filepath.Base(savePath),
		"file_size": fileHeader.Size,
		"chunks":    len(ids),
		"chunk_ids": ids,
	})
}

// KnowledgeReindexHandler triggers re-indexing of all documents in the knowledge directory.
// This is useful after manually adding files to the docs directory.
func KnowledgeReindexHandler(ctx context.Context, c *app.RequestContext) {
	if !config.ConfigInfo.AIAgent.Enabled {
		sendResponse(c, errno.ParamErr.WithMessage("AI Agent is not enabled"), nil)
		return
	}

	// Reset indexed file tracking so all files get re-indexed
	aiagent.ResetIndexedFiles()

	// Run the auto-indexing process (this re-scans and indexes all docs)
	go func() {
		bgCtx := context.Background()
		if err := aiagent.InitKnowledgeBase(bgCtx); err != nil {
			hlog.Errorf("[Knowledge Reindex] Failed: %v", err)
		}
	}()

	sendResponse(c, nil, map[string]interface{}{
		"status":  "accepted",
		"message": "Knowledge base re-indexing started in background. Check logs for progress.",
	})
}
