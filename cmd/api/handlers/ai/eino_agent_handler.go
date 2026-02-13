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

	// Get conversation memory
	memory := aiagent.GetSimpleMemory(sessionID)

	// Build user message for the Eino pipeline
	userMessage := &aiagent.UserMessage{
		ID:      sessionID,
		Query:   req.Message,
		History: memory.GetMessages(),
	}

	// Build and invoke the chat agent pipeline
	runner, err := aiagent.BuildChatAgent(ctx)
	if err != nil {
		hlog.Errorf("[Eino Agent] Failed to build chat agent: %v", err)
		// Fallback to Ollama if Eino agent fails
		hlog.Info("[Eino Agent] Falling back to Ollama handler")
		ChatHandler(ctx, c)
		return
	}

	out, err := runner.Invoke(ctx, userMessage)
	if err != nil {
		hlog.Errorf("[Eino Agent] Chat invocation failed: %v", err)
		// Fallback to Ollama
		ChatHandler(ctx, c)
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

	memory := aiagent.GetSimpleMemory(sessionID)
	userMessage := &aiagent.UserMessage{
		ID:      sessionID,
		Query:   req.Message,
		History: memory.GetMessages(),
	}

	// Build the agent
	runner, err := aiagent.BuildChatAgent(ctx)
	if err != nil {
		hlog.Errorf("[Eino Agent] Failed to build chat agent for stream: %v", err)
		// Fallback to Ollama SSE
		ChatSSE(ctx, c)
		return
	}

	// Set SSE headers
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.Set("Access-Control-Allow-Origin", "*")
	c.SetStatusCode(consts.StatusOK)

	// Create pipe for streaming
	pr, pw := io.Pipe()
	c.SetBodyStream(pr, -1)

	message := req.Message

	go func() {
		defer pw.Close()

		writeSSE := func(eventData interface{}) {
			data, _ := json.Marshal(eventData)
			pw.Write([]byte(fmt.Sprintf("data: %s\n\n", data))) //nolint:errcheck
		}

		// Use Eino stream
		sr, err := runner.Stream(ctx, userMessage)
		if err != nil {
			hlog.Errorf("[Eino Agent] Stream failed: %v", err)
			writeSSE(map[string]string{"type": "error", "content": "AI agent stream failed"})
			return
		}
		defer sr.Close()

		var fullResponse strings.Builder

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
			fullResponse.WriteString(chunk.Content)
			writeSSE(map[string]string{"type": "content", "content": chunk.Content})
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
			"mode":       "eino_agent",
		})
	}()
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
