package ai

import (
	aihandler "HuaTug.com/cmd/api/handlers/ai"
	"HuaTug.com/cmd/api/router/authfunc"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
)

// Register registers AI-related routes
func Register(r *server.Hertz) {
	root := r.Group("/")
	{
		_v1 := root.Group("/v1")

		// AI Chat routes (legacy Ollama-based)
		_ai := _v1.Group("/ai", _aiGroupMw()...)
		_ai.POST("/chat", append(_aiChatMw(), aihandler.ChatHandler)...)
		_ai.POST("/chat/stream", append(_aiChatMw(), aihandler.ChatSSE)...)
		_ai.GET("/sessions", append(_aiSessionMw(), aihandler.ListSessions)...)
		_ai.GET("/session", append(_aiSessionMw(), aihandler.GetSession)...)
		_ai.DELETE("/session", append(_aiSessionMw(), aihandler.DeleteSession)...)
		_ai.GET("/tools", aihandler.GetTools)
		_ai.GET("/health", aihandler.HealthCheck)

		// Eino-based AI Agent routes (upgraded with RAG + ReAct Agent)
		_agent := _v1.Group("/agent", _aiGroupMw()...)
		_agent.POST("/chat", append(_aiChatMw(), aihandler.EinoChatHandler)...)
		_agent.POST("/chat/stream", append(_aiChatMw(), aihandler.EinoChatSSE)...)
		_agent.GET("/health", aihandler.EinoHealthCheck)
		_agent.POST("/knowledge/upload", append(_aiChatMw(), aihandler.KnowledgeUploadHandler)...)
		_agent.POST("/knowledge/reindex", append(_aiChatMw(), aihandler.KnowledgeReindexHandler)...)
	}
}

func _aiGroupMw() []app.HandlerFunc {
	return nil
}

func _aiChatMw() []app.HandlerFunc {
	return authfunc.Auth()
}

func _aiSessionMw() []app.HandlerFunc {
	return authfunc.Auth()
}
