package aiagent

import "github.com/cloudwego/eino/schema"

// UserMessage represents a user's chat message with conversation context.
type UserMessage struct {
	ID      string            `json:"id"`      // Session/conversation ID
	Query   string            `json:"query"`   // User's query text
	History []*schema.Message `json:"history"` // Previous conversation messages
}
