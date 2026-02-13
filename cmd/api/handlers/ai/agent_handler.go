package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/config"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/ollama"
	"HuaTug.com/pkg/recommendation"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// ========== Data Structures ==========

// ChatMessage represents a single message in a conversation
type ChatMessage struct {
	Role      string     `json:"role"` // "user", "assistant", "system", "tool"
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"`
}

// ToolCall represents a tool invocation by the AI
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction represents the function details in a tool call
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatSession represents a single chat session
type ChatSession struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Messages  []ChatMessage `json:"messages"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// ChatRequest is the request body for chat endpoint
type ChatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

// ToolDefinition describes a tool available to the AI
type ToolDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// ========== In-Memory Session Store ==========

var (
	sessionStore = make(map[int64]map[string]*ChatSession) // userID -> sessionID -> session
	sessionMu    sync.RWMutex
)

// Ollama client singleton
var (
	ollamaClient     *ollama.Client
	ollamaClientOnce sync.Once
)

// getOllamaClient returns the singleton Ollama client
func getOllamaClient() *ollama.Client {
	ollamaClientOnce.Do(func() {
		cfg := config.ConfigInfo.Ollama
		if !cfg.Enabled {
			hlog.Info("[AI Agent] Ollama integration is disabled, using fallback mode")
			return
		}
		ollamaClient = ollama.NewClient(
			cfg.BaseURL,
			cfg.Model,
			cfg.Temperature,
			cfg.MaxTokens,
			cfg.Timeout,
		)
		hlog.Infof("[AI Agent] Ollama client initialized: %s (model: %s)", cfg.BaseURL, cfg.Model)
	})
	return ollamaClient
}

func getUserSessions(userID int64) map[string]*ChatSession {
	sessionMu.RLock()
	defer sessionMu.RUnlock()
	if sessions, ok := sessionStore[userID]; ok {
		return sessions
	}
	return nil
}

func getOrCreateUserSessions(userID int64) map[string]*ChatSession {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if _, ok := sessionStore[userID]; !ok {
		sessionStore[userID] = make(map[string]*ChatSession)
	}
	return sessionStore[userID]
}

// ========== Available Tools ==========

func getAvailableTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "search_videos",
			Description: "Search for videos by keyword. Use this tool when the user asks about finding videos or content on the platform.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"keyword": map[string]interface{}{
						"type":        "string",
						"description": "The search keyword",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of results (default 5)",
					},
				},
				"required": []string{"keyword"},
			},
		},
		{
			Name:        "get_hot_topics",
			Description: "Get current hot/trending topics and popular videos. Use this when the user asks about trends or popular content.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Number of hot topics to return (default 10)",
					},
				},
			},
		},
		{
			Name:        "suggest_content_strategy",
			Description: "Provide content creation suggestions including title, description, tags, and best posting time based on current trends.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"topic": map[string]interface{}{
						"type":        "string",
						"description": "The content topic or theme",
					},
					"content_type": map[string]interface{}{
						"type":        "string",
						"description": "Type of content: video, image, or panoramic",
					},
				},
				"required": []string{"topic"},
			},
		},
	}
}

// getOllamaTools converts tool definitions to Ollama's tool format
func getOllamaTools() []ollama.Tool {
	var tools []ollama.Tool
	for _, td := range getAvailableTools() {
		tools = append(tools, ollama.Tool{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        td.Name,
				Description: td.Description,
				Parameters:  td.Parameters,
			},
		})
	}
	return tools
}

// ========== Tool Execution ==========

func executeToolCall(ctx context.Context, toolName string, arguments string) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid tool arguments: %v", err)
	}

	switch toolName {
	case "search_videos":
		return executeSearchVideos(ctx, args)
	case "get_hot_topics":
		return executeGetHotTopics(ctx, args)
	case "suggest_content_strategy":
		return executeSuggestContentStrategy(ctx, args)
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

// executeToolCallFromMap handles tool arguments as map (from Ollama)
func executeToolCallFromMap(ctx context.Context, toolName string, arguments map[string]interface{}) (string, error) {
	switch toolName {
	case "search_videos":
		return executeSearchVideos(ctx, arguments)
	case "get_hot_topics":
		return executeGetHotTopics(ctx, arguments)
	case "suggest_content_strategy":
		return executeSuggestContentStrategy(ctx, arguments)
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

func executeSearchVideos(ctx context.Context, args map[string]interface{}) (string, error) {
	keyword, _ := args["keyword"].(string)
	limit := 5
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	resp, err := rpc.VideoSearch(ctx, &videos.VideoSearchRequestV2{
		Keyword:  keyword,
		PageNum:  1,
		PageSize: int64(limit),
	})
	if err != nil {
		hlog.Warnf("[AI Agent] Video search failed: %v", err)
		return fmt.Sprintf("Video search for '%s' returned no results at this time.", keyword), nil
	}

	result, _ := json.Marshal(resp)
	return fmt.Sprintf("Search results for '%s':\n%s", keyword, string(result)), nil
}

func executeGetHotTopics(ctx context.Context, args map[string]interface{}) (string, error) {
	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	// Use the local recommendation service directly (not RPC)
	service := recommendation.GetHotScoreService()
	if service != nil {
		videoIds, err := service.GetTopHotVideos(ctx, "24h", limit)
		if err == nil && len(videoIds) > 0 {
			result, _ := json.Marshal(map[string]interface{}{
				"hot_videos": videoIds,
				"count":      len(videoIds),
				"time_range": "24h",
			})
			return fmt.Sprintf("Current hot topics and trending videos:\n%s", string(result)), nil
		}
	}

	// Fallback: use VideoPopular RPC
	resp, err := rpc.VideoPopular(ctx, &videos.VideoPopularRequestV2{
		PageNum:  1,
		PageSize: int64(limit),
	})
	if err != nil {
		return "Currently unable to fetch hot topics. The platform has diverse content trending across categories.", nil
	}

	result, _ := json.Marshal(resp)
	return fmt.Sprintf("Current popular videos:\n%s", string(result)), nil
}

func executeSuggestContentStrategy(ctx context.Context, args map[string]interface{}) (string, error) {
	topic, _ := args["topic"].(string)
	contentType, _ := args["content_type"].(string)
	if contentType == "" {
		contentType = "video"
	}

	suggestion := fmt.Sprintf(`Content Strategy for topic "%s" (%s):

📝 Title Suggestions:
1. "你不知道的%s秘密 | 深度解析"
2. "%s完全攻略：从入门到精通"
3. "3分钟带你了解%s的最新趋势"

📋 Description Template:
"深入探讨%s相关话题，分享最新见解和实用技巧。关注我获取更多优质内容！#%s #知识分享 #干货"

🏷️ Recommended Tags:
- %s
- 知识分享
- 干货分享
- 热门话题
- 涨知识

⏰ Best Posting Time:
- Weekdays: 12:00-13:00 (lunch break), 18:00-20:00 (commute & evening)
- Weekends: 10:00-12:00 (morning), 20:00-22:00 (peak engagement)

💡 Tips:
- Keep the opening hook within the first 3 seconds
- Add captions/subtitles for better engagement
- Use trending background music
- End with a call-to-action (like, follow, comment)`,
		topic, contentType, topic, topic, topic, topic, topic, topic)

	return suggestion, nil
}

// ========== Ollama-Powered AI Response Generation ==========

// getSystemPrompt returns the configured or default system prompt
func getSystemPrompt() string {
	if config.ConfigInfo.Ollama.SystemPrompt != "" {
		return config.ConfigInfo.Ollama.SystemPrompt
	}
	return "你是ZhiShi短视频平台的AI智能助手「小知」。你可以和用户聊任何话题，也可以帮助搜索视频、查看热门趋势、提供创作建议。请用中文回答，语气友好、自然。"
}

// buildOllamaMessages converts session messages to Ollama format
func buildOllamaMessages(session *ChatSession) []ollama.ChatMessage {
	var msgs []ollama.ChatMessage
	for _, m := range session.Messages {
		msgs = append(msgs, ollama.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	return msgs
}

// generateAIResponseWithOllama uses Ollama LLM for response generation with tool calling
func generateAIResponseWithOllama(ctx context.Context, session *ChatSession, userMessage string) string {
	client := getOllamaClient()
	if client == nil {
		hlog.Warn("[AI Agent] Ollama client not available, using fallback")
		return generateAIResponseFallback(ctx, session, userMessage)
	}

	// Check if Ollama service is reachable
	if !client.IsAvailable(ctx) {
		hlog.Warn("[AI Agent] Ollama service is not reachable, using fallback")
		return generateAIResponseFallback(ctx, session, userMessage)
	}

	hlog.Infof("[AI Agent] Using Ollama model: %s for message: %s", client.GetModel(), userMessage)

	// Build messages for Ollama
	messages := buildOllamaMessages(session)
	tools := getOllamaTools()

	// First call: let the model decide if it needs tools
	resp, err := client.Chat(ctx, messages, tools)
	if err != nil {
		hlog.Errorf("[AI Agent] Ollama chat failed: %v", err)
		return generateAIResponseFallback(ctx, session, userMessage)
	}

	hlog.Infof("[AI Agent] Ollama response received, tool_calls=%d, content_len=%d", len(resp.Message.ToolCalls), len(resp.Message.Content))

	// Handle tool calls (may need multiple rounds)
	maxToolRounds := 3
	for round := 0; round < maxToolRounds && len(resp.Message.ToolCalls) > 0; round++ {
		hlog.Infof("[AI Agent] Ollama requested %d tool calls (round %d)", len(resp.Message.ToolCalls), round+1)

		// Add assistant message with tool calls
		messages = append(messages, ollama.ChatMessage{
			Role:      "assistant",
			Content:   resp.Message.Content,
			ToolCalls: resp.Message.ToolCalls,
		})

		// Execute each tool call and add results
		for _, tc := range resp.Message.ToolCalls {
			toolName := tc.Function.Name
			toolArgs := tc.Function.Arguments

			hlog.Infof("[AI Agent] Executing tool: %s, args: %v", toolName, toolArgs)
			result, err := executeToolCallFromMap(ctx, toolName, toolArgs)
			if err != nil {
				result = fmt.Sprintf("Error executing tool %s: %v", toolName, err)
				hlog.Warnf("[AI Agent] Tool execution error: %v", err)
			}

			// Store tool result in session
			session.Messages = append(session.Messages, ChatMessage{
				Role:     "tool",
				Content:  result,
				ToolName: toolName,
			})

			// Add tool response to Ollama messages
			messages = append(messages, ollama.ChatMessage{
				Role:    "tool",
				Content: result,
			})
		}

		// Call Ollama again with tool results (no tools this time to force a text response)
		resp, err = client.Chat(ctx, messages, nil)
		if err != nil {
			hlog.Errorf("[AI Agent] Ollama follow-up call failed: %v", err)
			return "抱歉，处理工具结果时出现了问题。请稍后重试。"
		}
	}

	if resp.Message.Content == "" {
		return "抱歉，我暂时无法生成回复。请稍后再试。"
	}

	// Strip <think>...</think> tags from thinking models (e.g. qwen3-coder)
	finalContent := ollama.StripThinkTags(resp.Message.Content)
	if finalContent == "" {
		return "抱歉，模型返回了空内容。请稍后再试。"
	}

	return finalContent
}

// thinkTagFilter is a streaming filter that strips <think>...</think> blocks in real time
type thinkTagFilter struct {
	inThinkBlock bool
	buffer       string
	output       strings.Builder
	onChunk      func(content string)
}

func newThinkTagFilter(onChunk func(content string)) *thinkTagFilter {
	return &thinkTagFilter{onChunk: onChunk}
}

func (f *thinkTagFilter) Write(content string) {
	f.buffer += content
	for {
		if f.inThinkBlock {
			closeIdx := strings.Index(f.buffer, "</think>")
			if closeIdx >= 0 {
				f.buffer = f.buffer[closeIdx+len("</think>"):]
				f.inThinkBlock = false
				continue
			}
			// Still inside think block, keep only tail for partial match
			if len(f.buffer) > 20 {
				f.buffer = f.buffer[len(f.buffer)-20:]
			}
			return
		}

		openIdx := strings.Index(f.buffer, "<think>")
		if openIdx >= 0 {
			if openIdx > 0 {
				chunk := f.buffer[:openIdx]
				f.output.WriteString(chunk)
				f.onChunk(chunk)
			}
			f.buffer = f.buffer[openIdx+len("<think>"):]
			f.inThinkBlock = true
			continue
		}

		// Check for potential partial "<think>" at end of buffer
		partial := "<think>"
		for i := 1; i < len(partial) && i <= len(f.buffer); i++ {
			if strings.HasSuffix(f.buffer, partial[:i]) {
				safe := f.buffer[:len(f.buffer)-i]
				if safe != "" {
					f.output.WriteString(safe)
					f.onChunk(safe)
				}
				f.buffer = f.buffer[len(f.buffer)-i:]
				return
			}
		}

		// No think tags found, output everything
		if f.buffer != "" {
			f.output.WriteString(f.buffer)
			f.onChunk(f.buffer)
		}
		f.buffer = ""
		return
	}
}

func (f *thinkTagFilter) Flush() {
	if f.buffer != "" && !f.inThinkBlock {
		f.output.WriteString(f.buffer)
		f.onChunk(f.buffer)
		f.buffer = ""
	}
}

func (f *thinkTagFilter) String() string {
	return f.output.String()
}

// generateAIResponseWithOllamaStream uses Ollama LLM for streaming response.
// Key design: directly stream from Ollama -> filter <think> tags -> SSE to frontend.
// Tool calls are detected via a non-streaming probe first, because Ollama cannot
// stream and return tool_calls at the same time.
func generateAIResponseWithOllamaStream(ctx context.Context, session *ChatSession, userMessage string, onChunk func(content string)) (string, error) {
	client := getOllamaClient()
	if client == nil {
		return "", fmt.Errorf("ollama client not available")
	}

	if !client.IsAvailable(ctx) {
		return "", fmt.Errorf("ollama service not reachable")
	}

	hlog.Infof("[AI Agent] Starting Ollama stream for message: %s", userMessage)

	messages := buildOllamaMessages(session)
	tools := getOllamaTools()

	// Probe: non-streaming call with tools to detect if model wants tool calls
	resp, err := client.Chat(ctx, messages, tools)
	if err != nil {
		return "", fmt.Errorf("ollama chat failed: %w", err)
	}

	hlog.Infof("[AI Agent] Stream probe: tool_calls=%d, content_len=%d", len(resp.Message.ToolCalls), len(resp.Message.Content))

	// If tools were called, execute them then stream the final answer
	if len(resp.Message.ToolCalls) > 0 {
		messages = append(messages, ollama.ChatMessage{
			Role:      "assistant",
			Content:   resp.Message.Content,
			ToolCalls: resp.Message.ToolCalls,
		})

		for _, tc := range resp.Message.ToolCalls {
			hlog.Infof("[AI Agent] Stream executing tool: %s", tc.Function.Name)
			result, execErr := executeToolCallFromMap(ctx, tc.Function.Name, tc.Function.Arguments)
			if execErr != nil {
				result = fmt.Sprintf("Error: %v", execErr)
			}

			session.Messages = append(session.Messages, ChatMessage{
				Role:     "tool",
				Content:  result,
				ToolName: tc.Function.Name,
			})
			messages = append(messages, ollama.ChatMessage{
				Role:    "tool",
				Content: result,
			})
		}

		// Stream the final answer (no tools, to avoid another round of tool calls)
		filter := newThinkTagFilter(onChunk)
		_, err = client.ChatStream(ctx, messages, nil, func(content string) {
			filter.Write(content)
		})
		filter.Flush()
		if err != nil {
			return filter.String(), err
		}
		return filter.String(), nil
	}

	// No tool calls — the probe already generated a full response.
	// Instead of calling the model again, directly stream from a new call WITHOUT tools
	// so the model generates a fresh streaming response.
	filter := newThinkTagFilter(onChunk)
	_, err = client.ChatStream(ctx, messages, nil, func(content string) {
		filter.Write(content)
	})
	filter.Flush()

	if err != nil {
		return filter.String(), err
	}
	return filter.String(), nil
}

// ========== Fallback AI Response (when Ollama is unavailable) ==========

// generateAIResponseFallback is the original rule-based response generator
func generateAIResponseFallback(ctx context.Context, session *ChatSession, userMessage string) string {
	// Analyze user intent and decide whether to call tools
	toolCalls := analyzeIntent(userMessage)

	if len(toolCalls) > 0 {
		var parts []string
		for _, tc := range toolCalls {
			result, err := executeToolCall(ctx, tc.Function.Name, tc.Function.Arguments)
			if err != nil {
				parts = append(parts, fmt.Sprintf("⚠️ 工具调用出错: %s", err.Error()))
				continue
			}

			session.Messages = append(session.Messages, ChatMessage{
				Role:     "tool",
				Content:  result,
				ToolName: tc.Function.Name,
			})

			switch tc.Function.Name {
			case "search_videos":
				parts = append(parts, fmt.Sprintf("📹 **视频搜索结果**\n\n%s", summarizeResult(result)))
			case "get_hot_topics":
				parts = append(parts, fmt.Sprintf("🔥 **当前热门话题**\n\n%s", summarizeResult(result)))
			case "suggest_content_strategy":
				parts = append(parts, fmt.Sprintf("💡 **创作建议**\n\n%s", result))
			}
		}

		if len(parts) == 0 {
			return "抱歉，我暂时无法获取相关信息。请稍后再试。"
		}

		response := strings.Join(parts, "\n\n---\n\n")
		response += "\n\n如果你还需要更多帮助，随时告诉我！😊"
		return response
	}

	// Direct conversation without tools
	return generateDirectResponse(userMessage)
}

func summarizeResult(result string) string {
	if len(result) > 800 {
		return result[:800] + "\n..."
	}
	return result
}

// analyzeIntent determines which tools to call based on user message (fallback mode)
func analyzeIntent(message string) []ToolCall {
	msg := strings.ToLower(message)
	var calls []ToolCall

	// Search intent
	searchKeywords := []string{"搜索", "搜一下", "找", "查找", "有没有", "search", "find", "关于", "推荐视频", "看看"}
	for _, kw := range searchKeywords {
		if strings.Contains(msg, kw) {
			keyword := extractSearchKeyword(message, kw)
			if keyword != "" {
				args, _ := json.Marshal(map[string]interface{}{"keyword": keyword, "limit": 5})
				calls = append(calls, ToolCall{
					ID: "call_search_1", Type: "function",
					Function: ToolCallFunction{Name: "search_videos", Arguments: string(args)},
				})
			}
			break
		}
	}

	// Hot topics intent
	hotKeywords := []string{"热门", "热搜", "趋势", "热点", "流行", "trending", "hot", "popular", "火", "排行"}
	for _, kw := range hotKeywords {
		if strings.Contains(msg, kw) {
			args, _ := json.Marshal(map[string]interface{}{"limit": 10})
			calls = append(calls, ToolCall{
				ID: "call_hot_1", Type: "function",
				Function: ToolCallFunction{Name: "get_hot_topics", Arguments: string(args)},
			})
			break
		}
	}

	// Content strategy intent
	strategyKeywords := []string{"建议", "怎么拍", "标题", "标签", "发布时间", "创作", "怎么做", "如何发", "什么时候发"}
	for _, kw := range strategyKeywords {
		if strings.Contains(msg, kw) {
			topic := extractTopic(message)
			if topic == "" {
				topic = "通用内容"
			}
			args, _ := json.Marshal(map[string]interface{}{"topic": topic, "content_type": "video"})
			calls = append(calls, ToolCall{
				ID: "call_strategy_1", Type: "function",
				Function: ToolCallFunction{Name: "suggest_content_strategy", Arguments: string(args)},
			})
			break
		}
	}

	return calls
}

func extractSearchKeyword(message, triggerWord string) string {
	idx := strings.Index(strings.ToLower(message), strings.ToLower(triggerWord))
	if idx == -1 {
		return message
	}
	after := strings.TrimSpace(message[idx+len([]byte(triggerWord)):])
	if after != "" {
		after = strings.TrimRight(after, "的吗吧呢啊哦？?！!")
		if after != "" {
			return after
		}
	}
	before := strings.TrimSpace(message[:idx])
	if before != "" {
		return before
	}
	return message
}

func extractTopic(message string) string {
	topic := message
	removeWords := []string{"请", "帮我", "给我", "我想", "怎么", "如何", "建议", "什么", "应该",
		"可以", "能", "一下", "关于", "的", "吗", "？", "?"}
	for _, w := range removeWords {
		topic = strings.ReplaceAll(topic, w, "")
	}
	topic = strings.TrimSpace(topic)
	if len([]rune(topic)) > 20 {
		return string([]rune(topic)[:20])
	}
	return topic
}

func generateDirectResponse(message string) string {
	msg := strings.ToLower(message)

	if containsAny(msg, []string{"你好", "hello", "hi", "嗨", "hey"}) {
		return "你好！👋 我是你的AI智能助手，我可以帮你：\n\n" +
			"🔍 **搜索视频** - 告诉我你想找什么内容\n" +
			"🔥 **查看热门** - 了解当前平台热门话题和趋势\n" +
			"💡 **创作建议** - 为你提供标题、描述、标签和最佳发布时间建议\n" +
			"❓ **回答问题** - 关于平台使用的任何问题\n\n" +
			"有什么我可以帮你的吗？"
	}

	if containsAny(msg, []string{"帮助", "help", "能做什么", "功能"}) {
		return "我可以帮你做以下事情：\n\n" +
			"1️⃣ **视频搜索** - 输入「搜索 + 关键词」\n" +
			"2️⃣ **热门趋势** - 输入「查看热门」\n" +
			"3️⃣ **创作建议** - 输入「给我关于XX的创作建议」\n" +
			"4️⃣ **标题生成** - 输入「帮我想个关于XX的标题」\n" +
			"5️⃣ **发布策略** - 输入「什么时候发视频最好」\n\n" +
			"试试看吧！💪"
	}

	if containsAny(msg, []string{"谢谢", "thank", "感谢", "多谢"}) {
		return "不客气！😊 很高兴能帮到你。如果还有其他需要，随时找我！"
	}

	return fmt.Sprintf("我收到了你的消息。\n\n" +
		"作为AI助手，我最擅长以下几个方面：\n" +
		"- 🔍 搜索视频内容（试试说「搜索+关键词」）\n" +
		"- 🔥 查看热门趋势（试试说「热门话题」）\n" +
		"- 💡 提供创作建议（试试说「给我创作建议」）\n\n" +
		"请告诉我你具体需要什么帮助？")
}

func containsAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// ========== HTTP Handlers ==========

func sendResponse(c *app.RequestContext, err error, data interface{}) {
	if err != nil {
		Err := errno.ConvertErr(err)
		c.JSON(consts.StatusOK, map[string]interface{}{
			"code":    Err.ErrCode,
			"message": Err.ErrMsg,
			"data":    data,
		})
		return
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"code":    errno.SuccessCode,
		"message": "Success",
		"data":    data,
	})
}

func getUserIDFromContext(c *app.RequestContext) int64 {
	if uid, exists := c.Get("user_id"); exists {
		if id, ok := uid.(int64); ok {
			return id
		}
		if id, ok := uid.(float64); ok {
			return int64(id)
		}
	}
	return 1
}

// ChatHandler handles non-streaming chat requests
func ChatHandler(ctx context.Context, c *app.RequestContext) {
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

	// Get or create session
	sessions := getOrCreateUserSessions(userID)
	sessionMu.Lock()
	session, exists := sessions[req.SessionID]
	if !exists {
		session = &ChatSession{
			ID:    req.SessionID,
			Title: "新会话",
			Messages: []ChatMessage{
				{Role: "system", Content: getSystemPrompt()},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		sessions[req.SessionID] = session
	}

	// Add user message
	session.Messages = append(session.Messages, ChatMessage{
		Role:    "user",
		Content: req.Message,
	})

	// Auto-name session from first user message
	userMsgCount := 0
	for _, m := range session.Messages {
		if m.Role == "user" {
			userMsgCount++
		}
	}
	if userMsgCount == 1 {
		title := []rune(req.Message)
		if len(title) > 20 {
			session.Title = string(title[:20]) + "..."
		} else {
			session.Title = req.Message
		}
	}
	session.UpdatedAt = time.Now()
	sessionMu.Unlock()

	// Generate AI response (Ollama with fallback)
	response := generateAIResponseWithOllama(ctx, session, req.Message)

	// Store assistant message
	sessionMu.Lock()
	session.Messages = append(session.Messages, ChatMessage{
		Role:    "assistant",
		Content: response,
	})
	sessionMu.Unlock()

	sendResponse(c, nil, map[string]interface{}{
		"reply":      response,
		"session_id": session.ID,
		"title":      session.Title,
	})
}

// ChatSSE handles SSE streaming chat requests.
// Uses io.Pipe + SetBodyStream(-1) for true chunked streaming in Hertz.
func ChatSSE(ctx context.Context, c *app.RequestContext) {
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

	// Get or create session
	sessions := getOrCreateUserSessions(userID)
	sessionMu.Lock()
	session, exists := sessions[req.SessionID]
	if !exists {
		session = &ChatSession{
			ID:    req.SessionID,
			Title: "新会话",
			Messages: []ChatMessage{
				{Role: "system", Content: getSystemPrompt()},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		sessions[req.SessionID] = session
	}

	session.Messages = append(session.Messages, ChatMessage{
		Role:    "user",
		Content: req.Message,
	})

	// Auto-name
	userMsgCount := 0
	for _, m := range session.Messages {
		if m.Role == "user" {
			userMsgCount++
		}
	}
	if userMsgCount == 1 {
		title := []rune(req.Message)
		if len(title) > 20 {
			session.Title = string(title[:20]) + "..."
		} else {
			session.Title = req.Message
		}
	}
	session.UpdatedAt = time.Now()
	sessionMu.Unlock()

	// Set SSE headers
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.Set("Access-Control-Allow-Origin", "*")
	c.SetStatusCode(consts.StatusOK)

	// Create a pipe for streaming: pw.Write() -> pr is read by Hertz -> sent to client
	pr, pw := io.Pipe()

	// SetBodyStream with -1 means chunked transfer encoding (true streaming)
	c.SetBodyStream(pr, -1)

	// Capture message for goroutine
	message := req.Message

	// Launch a goroutine that writes SSE events to the pipe writer
	go func() {
		defer pw.Close()

		// Helper: write an SSE event
		writeSSE := func(eventData interface{}) {
			data, _ := json.Marshal(eventData)
			pw.Write([]byte(fmt.Sprintf("data: %s\n\n", data))) //nolint:errcheck
		}

		var fullResponse string

		// Try Ollama streaming
		ollamaErr := func() error {
			var err error
			fullResponse, err = generateAIResponseWithOllamaStream(ctx, session, message, func(content string) {
				writeSSE(map[string]string{"type": "content", "content": content})
			})
			return err
		}()

		if ollamaErr != nil {
			hlog.Warnf("[AI Agent] Ollama streaming failed: %v, falling back", ollamaErr)

			// Fallback: generate response and stream it in small chunks
			fullResponse = generateAIResponseFallback(ctx, session, message)
			runes := []rune(fullResponse)
			for i := 0; i < len(runes); {
				chunkSize := 3 + (i % 4)
				if i+chunkSize > len(runes) {
					chunkSize = len(runes) - i
				}
				chunk := string(runes[i : i+chunkSize])
				writeSSE(map[string]string{"type": "content", "content": chunk})
				i += chunkSize
				time.Sleep(15 * time.Millisecond)
			}
		}

		// Store assistant message
		sessionMu.Lock()
		session.Messages = append(session.Messages, ChatMessage{
			Role:    "assistant",
			Content: fullResponse,
		})
		sessionMu.Unlock()

		// Send done event
		writeSSE(map[string]interface{}{
			"type": "done", "session_id": session.ID, "title": session.Title,
		})
	}()
}

// ListSessions lists all chat sessions for a user
func ListSessions(ctx context.Context, c *app.RequestContext) {
	userID := getUserIDFromContext(c)
	sessions := getUserSessions(userID)

	var result []map[string]interface{}
	if sessions != nil {
		for _, s := range sessions {
			result = append(result, map[string]interface{}{
				"id":            s.ID,
				"title":         s.Title,
				"created_at":    s.CreatedAt.Format(time.RFC3339),
				"updated_at":    s.UpdatedAt.Format(time.RFC3339),
				"message_count": len(s.Messages),
			})
		}
	}

	sendResponse(c, nil, map[string]interface{}{"sessions": result})
}

// GetSession gets a specific chat session with messages
func GetSession(ctx context.Context, c *app.RequestContext) {
	userID := getUserIDFromContext(c)
	sessionID := c.Query("session_id")
	if sessionID == "" {
		sendResponse(c, errno.ParamErr.WithMessage("session_id is required"), nil)
		return
	}

	sessions := getUserSessions(userID)
	if sessions == nil {
		sendResponse(c, errno.ParamErr.WithMessage("session not found"), nil)
		return
	}

	sessionMu.RLock()
	session, exists := sessions[sessionID]
	sessionMu.RUnlock()

	if !exists {
		sendResponse(c, errno.ParamErr.WithMessage("session not found"), nil)
		return
	}

	var filteredMessages []map[string]interface{}
	for _, msg := range session.Messages {
		if msg.Role == "system" {
			continue
		}
		filteredMessages = append(filteredMessages, map[string]interface{}{
			"role": msg.Role, "content": msg.Content,
		})
	}

	sendResponse(c, nil, map[string]interface{}{
		"id": session.ID, "title": session.Title,
		"messages": filteredMessages,
	})
}

// DeleteSession deletes a chat session
func DeleteSession(ctx context.Context, c *app.RequestContext) {
	userID := getUserIDFromContext(c)
	sessionID := c.Query("session_id")
	if sessionID == "" {
		sendResponse(c, errno.ParamErr.WithMessage("session_id is required"), nil)
		return
	}

	sessionMu.Lock()
	if sessions, ok := sessionStore[userID]; ok {
		delete(sessions, sessionID)
	}
	sessionMu.Unlock()

	sendResponse(c, nil, map[string]interface{}{"deleted": true})
}

// GetTools returns available AI tools
func GetTools(ctx context.Context, c *app.RequestContext) {
	sendResponse(c, nil, map[string]interface{}{"tools": getAvailableTools()})
}

// HealthCheck checks Ollama connectivity
func HealthCheck(ctx context.Context, c *app.RequestContext) {
	client := getOllamaClient()
	status := map[string]interface{}{
		"ollama_enabled": config.ConfigInfo.Ollama.Enabled,
		"model":          config.ConfigInfo.Ollama.Model,
		"base_url":       config.ConfigInfo.Ollama.BaseURL,
	}

	if client != nil && client.IsAvailable(ctx) {
		status["ollama_status"] = "connected"
		status["mode"] = "ollama"
	} else {
		status["ollama_status"] = "disconnected"
		status["mode"] = "fallback"
	}

	sendResponse(c, nil, status)
}
