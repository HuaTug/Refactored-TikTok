package aiagent

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// Cached agent pipeline singleton to avoid rebuilding on every request.
var (
	cachedAgent     compose.Runnable[*UserMessage, *schema.Message]
	cachedAgentErr  error
	cachedAgentOnce sync.Once
)

// ResetChatAgent resets the cached agent, forcing a rebuild on next call.
// Useful after knowledge base re-indexing or config changes.
func ResetChatAgent() {
	cachedAgentOnce = sync.Once{}
	cachedAgent = nil
	cachedAgentErr = nil
	hlog.Info("[AI Agent] Chat agent cache reset, will rebuild on next request")
}

// BuildChatAgent constructs the complete RAG-enhanced ReAct Agent pipeline.
// The pipeline is cached after the first successful build to avoid expensive
// re-initialization (Milvus connection, Embedder, ChatModel) on every request.
//
// Pipeline architecture (referencing OncallAgent's chat_pipeline):
//
//	START
//	  ├── InputToRag → MilvusRetriever ──┐
//	  └── InputToChat ───────────────────┤
//	                                     ▼
//	                              ChatTemplate → ReActAgent → END
//
// The pipeline:
// 1. Splits input into two parallel branches
// 2. One branch performs RAG retrieval from Milvus knowledge base
// 3. The other branch extracts chat context (history, date, etc.)
// 4. Both merge at ChatTemplate which constructs the full prompt
// 5. ReActAgent processes the prompt with tool-calling capabilities
func BuildChatAgent(ctx context.Context) (compose.Runnable[*UserMessage, *schema.Message], error) {
	cachedAgentOnce.Do(func() {
		hlog.Info("[AI Agent] Building chat agent pipeline (first time)...")
		cachedAgent, cachedAgentErr = buildChatAgentInternal(ctx)
		if cachedAgentErr != nil {
			hlog.Errorf("[AI Agent] Failed to build chat agent: %v", cachedAgentErr)
			// Reset so next request will retry
			cachedAgentOnce = sync.Once{}
		} else {
			hlog.Info("[AI Agent] Chat agent pipeline built and cached successfully")
		}
	})
	return cachedAgent, cachedAgentErr
}

// buildChatAgentInternal does the actual pipeline construction.
func buildChatAgentInternal(ctx context.Context) (compose.Runnable[*UserMessage, *schema.Message], error) {
	const (
		InputToRag      = "InputToRag"
		ChatTemplate    = "ChatTemplate"
		ReactAgent      = "ReactAgent"
		MilvusRetriever = "MilvusRetriever"
		InputToChat     = "InputToChat"
	)

	g := compose.NewGraph[*UserMessage, *schema.Message]()

	// Node: ChatTemplate - Prompt template with RAG context
	chatTemplate, err := NewChatTemplate(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat template: %w", err)
	}
	_ = g.AddChatTemplateNode(ChatTemplate, chatTemplate)

	// Node: ReActAgent - LLM with tool-calling capability
	reactAgentLambda, err := newReactAgentLambda(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create ReAct agent: %w", err)
	}
	_ = g.AddLambdaNode(ReactAgent, reactAgentLambda, compose.WithNodeName("ReActAgent"))

	// Node: InputToChat - Extracts chat context (history, date, etc.)
	_ = g.AddLambdaNode(InputToChat,
		compose.InvokableLambdaWithOption(inputToChatLambda),
		compose.WithNodeName("UserMessageToChat"),
	)

	// Try to add Milvus RAG branch (optional - pipeline works without it)
	ret, milvusErr := NewMilvusRetriever(ctx)
	ragEnabled := milvusErr == nil
	if milvusErr != nil {
		hlog.Warnf("[AI Agent] Milvus retriever unavailable, RAG will be disabled (tools still active): %v", milvusErr)
	}

	if ragEnabled {
		// Full pipeline with RAG:
		// START → InputToRag → RagRetriever (query→formatted docs string) ──┐
		// START → InputToChat ─────────────────────────────────────────────┤
		//                                                                  ▼
		//                                                           ChatTemplate → ReactAgent → END
		_ = g.AddLambdaNode(InputToRag,
			compose.InvokableLambdaWithOption(inputToRagLambda),
			compose.WithNodeName("UserMessageToRag"),
		)
		// RagRetriever: calls retriever and formats []*schema.Document → string for {documents} template var
		_ = g.AddLambdaNode(MilvusRetriever,
			compose.InvokableLambdaWithOption(makeRagLambda(ret)),
			compose.WithNodeName("MilvusRetriever"),
		)

		_ = g.AddEdge(compose.START, InputToRag)
		_ = g.AddEdge(compose.START, InputToChat)
		_ = g.AddEdge(InputToRag, MilvusRetriever)
		_ = g.AddEdge(MilvusRetriever, ChatTemplate)
		_ = g.AddEdge(InputToChat, ChatTemplate)
	} else {
		// Simplified pipeline without RAG:
		// START → [InputToChat, EmptyDocs] → ChatTemplate → ReactAgent → END
		const EmptyDocs = "EmptyDocs"
		_ = g.AddLambdaNode(EmptyDocs,
			compose.InvokableLambdaWithOption(emptyDocsLambda),
			compose.WithNodeName("EmptyDocs"),
		)
		_ = g.AddEdge(compose.START, InputToChat)
		_ = g.AddEdge(compose.START, EmptyDocs)
		_ = g.AddEdge(InputToChat, ChatTemplate)
		_ = g.AddEdge(EmptyDocs, ChatTemplate)
	}

	_ = g.AddEdge(ChatTemplate, ReactAgent)
	_ = g.AddEdge(ReactAgent, compose.END)

	// Compile the graph with AllPredecessor trigger mode (wait for all inputs)
	r, err := g.Compile(ctx,
		compose.WithGraphName("TikTokChatAgent"),
		compose.WithNodeTriggerMode(compose.AllPredecessor),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to compile chat agent graph: %w", err)
	}
	return r, nil
}

// newReactAgentLambda creates the ReAct agent with all available tools.
func newReactAgentLambda(ctx context.Context) (*compose.Lambda, error) {
	agentConfig := &react.AgentConfig{
		MaxStep:            4,
		ToolReturnDirectly: map[string]struct{}{},
	}

	// Initialize the chat model
	chatModel, err := NewChatModel(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat model for ReAct agent: %w", err)
	}
	agentConfig.ToolCallingModel = chatModel

	// Register all available tools
	var toolList []tool.BaseTool
	toolList = append(toolList, NewSearchVideosTool())
	toolList = append(toolList, NewGetHotTopicsTool())
	toolList = append(toolList, NewSuggestContentStrategyTool())
	toolList = append(toolList, NewQueryKnowledgeTool())
	toolList = append(toolList, NewGetCurrentTimeTool())
	// Recommendation-specific tools (Agent × Rec integration)
	toolList = append(toolList, NewGetUserRecommendationTool())
	toolList = append(toolList, NewGetUserStateTool())
	toolList = append(toolList, NewGetVideoHotRankingTool())
	agentConfig.ToolsConfig.Tools = toolList

	agent, err := react.NewAgent(ctx, agentConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create ReAct agent: %w", err)
	}

	lambda, err := compose.AnyLambda(agent.Generate, agent.Stream, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to wrap ReAct agent as Lambda: %w", err)
	}
	return lambda, nil
}
