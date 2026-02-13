package aiagent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// BuildChatAgent constructs the complete RAG-enhanced ReAct Agent pipeline.
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
	const (
		InputToRag      = "InputToRag"
		ChatTemplate    = "ChatTemplate"
		ReactAgent      = "ReactAgent"
		MilvusRetriever = "MilvusRetriever"
		InputToChat     = "InputToChat"
	)

	g := compose.NewGraph[*UserMessage, *schema.Message]()

	// Node: InputToRag - Extracts query string for RAG retrieval
	_ = g.AddLambdaNode(InputToRag,
		compose.InvokableLambdaWithOption(inputToRagLambda),
		compose.WithNodeName("UserMessageToRag"),
	)

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

	// Node: MilvusRetriever - RAG knowledge retrieval from vector store
	retriever, err := NewMilvusRetriever(ctx)
	if err != nil {
		hlog.Warnf("[AI Agent] Milvus retriever unavailable, RAG will be disabled: %v", err)
		// If retriever is unavailable, we can still proceed without RAG
		// by using a no-op retriever or skipping the retrieval branch
		// For now, return error to signal the issue
		return nil, fmt.Errorf("failed to create Milvus retriever: %w", err)
	}
	// Output key "documents" matches the {documents} placeholder in the prompt template
	_ = g.AddRetrieverNode(MilvusRetriever, retriever, compose.WithOutputKey("documents"))

	// Node: InputToChat - Extracts chat context (history, date, etc.)
	_ = g.AddLambdaNode(InputToChat,
		compose.InvokableLambdaWithOption(inputToChatLambda),
		compose.WithNodeName("UserMessageToChat"),
	)

	// Wire the edges
	_ = g.AddEdge(compose.START, InputToRag)
	_ = g.AddEdge(compose.START, InputToChat)
	_ = g.AddEdge(InputToRag, MilvusRetriever)
	_ = g.AddEdge(MilvusRetriever, ChatTemplate)
	_ = g.AddEdge(InputToChat, ChatTemplate)
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
		MaxStep:            25,
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
